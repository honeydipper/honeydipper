// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/agenthistory"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

type fakeCacheCaller struct {
	data  map[string][]byte
	lists map[string][]string
}

func (f *fakeCacheCaller) Call(feature string, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	if f.data == nil {
		f.data = map[string][]byte{}
	}
	if feature != "cache" {
		return []byte{}, nil
	}
	payload, _ := params.(map[string]any)
	key, _ := payload["key"].(string)

	switch method {
	case "load":
		return f.data[key], nil
	case "save":
		val, _ := payload["value"].(string)
		f.data[key] = []byte(val)

		return []byte{}, nil
	case "lrange":
		if f.lists == nil {
			f.lists = map[string][]string{}
		}
		list, ok := f.lists[key]
		if !ok || len(list) == 0 {
			return []byte("[]"), nil
		}

		ret := "["
		for i, item := range list {
			if i > 0 {
				ret += ", "
			}
			ret += item
		}
		ret += "]"

		return []byte(ret), nil
	case "rpush":
		if f.lists == nil {
			f.lists = map[string][]string{}
		}
		val, _ := payload["value"].(string)
		f.lists[key] = append(f.lists[key], val)

		return []byte{}, nil
	case "del":
		delete(f.data, key)
		delete(f.lists, key)

		return []byte{}, nil
	default:
		return []byte{}, nil
	}
}

func (f *fakeCacheCaller) CallNoWait(feature string, method string, params interface{}, labelsKV ...string) error {
	_, _ = f.Call(feature, method, params, labelsKV...)

	return nil
}

func (f *fakeCacheCaller) CallRaw(feature string, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return []byte{}, nil
}

func (f *fakeCacheCaller) CallRawNoWait(feature string, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}

func (f *fakeCacheCaller) GetName() string {
	return "fake-cache"
}

func TestCreateActivations(t *testing.T) {
	atomic.StoreInt64(&agentMatchCounter, 0)

	agent = &Service{ready: make(chan struct{})}
	close(agent.ready)
	orig := persistActivationFn
	origResolve := resolveContextFn
	persistActivationFn = func(caller dipper.RPCCaller, msg *dipper.Message, agentName string) (*activationPersistResult, error) {
		return &activationPersistResult{SessionID: "s1", TurnID: "t1"}, nil
	}
	resolveCalled := false
	resolveContextFn = func(caller dipper.RPCCaller, sessionID string, turnID string) error {
		assert.Equal(t, "s1", sessionID)
		assert.Equal(t, "t1", turnID)
		resolveCalled = true

		return nil
	}
	defer func() { persistActivationFn = orig }()
	defer func() { resolveContextFn = origResolve }()

	msg := &dipper.Message{Payload: map[string]interface{}{
		"agent":  "support-bot",
		"prompt": "help user",
	}}

	assert.NotPanics(t, func() { createActivations(nil, msg) })
	assert.Equal(t, int64(1), atomic.LoadInt64(&agentMatchCounter))
	assert.True(t, resolveCalled)
}

func TestPersistAgentActivation_ReusesSessionOnSameEvent(t *testing.T) {
	caller := &fakeCacheCaller{data: map[string][]byte{}}
	msg := &dipper.Message{
		Labels: map[string]string{"eventID": "evt-1", "sourceSessionID": "wf-1"},
		Payload: map[string]any{
			"agent": "support-bot",
		},
	}

	first, err := persistAgentActivation(caller, msg, "support-bot")
	assert.NoError(t, err)
	assert.NotEmpty(t, first.SessionID)
	assert.NotEmpty(t, first.TurnID)

	second, err := persistAgentActivation(caller, msg, "support-bot")
	assert.NoError(t, err)
	assert.Equal(t, first.SessionID, second.SessionID)
	assert.NotEqual(t, first.TurnID, second.TurnID)
}

func TestPersistAgentActivation_UsesCtxConversationID(t *testing.T) {
	caller := &fakeCacheCaller{data: map[string][]byte{}}
	msg1 := &dipper.Message{
		Labels: map[string]string{"eventID": "evt-1"},
		Payload: map[string]any{
			"agent": "support-bot",
			"ctx": map[string]any{
				"conversation_id": "thread-123",
			},
		},
	}
	msg2 := &dipper.Message{
		Labels: map[string]string{"eventID": "evt-2"},
		Payload: map[string]any{
			"agent": "support-bot",
			"ctx": map[string]any{
				"conversation_id": "thread-123",
			},
		},
	}

	first, err := persistAgentActivation(caller, msg1, "support-bot")
	assert.NoError(t, err)
	second, err := persistAgentActivation(caller, msg2, "support-bot")
	assert.NoError(t, err)

	assert.Equal(t, first.SessionID, second.SessionID)
	assert.NotEqual(t, first.TurnID, second.TurnID)
}

func TestResolveTurnContext(t *testing.T) {
	origAgent := agent
	defer func() { agent = origAgent }()
	agent = &Service{config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
		"support-bot": {SystemPrompt: "you are helpful"},
	}}}}

	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}

	sess := &agentSession{
		ID:             "sess-1",
		Agent:          "support-bot",
		ConversationID: "thread-123",
		State:          "resolving_context",
		CurrentTurnID:  "turn-1",
		CreatedAt:      "2026-01-01T00:00:00Z",
		UpdatedAt:      "2026-01-01T00:00:00Z",
	}
	turn := &agentTurn{
		ID:        "turn-1",
		SessionID: "sess-1",
		Agent:     "support-bot",
		State:     "created",
		Event: map[string]any{
			"text": "hello",
		},
		Ctx: map[string]any{
			"conversation_id": "thread-123",
		},
		Workflow: map[string]any{
			"status": "success",
		},
		Tools:     []any{"workflow"},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	assert.NoError(t, saveJSON(caller, agentSessionPrefix+sess.ID, sess, agentSessionTTL))
	assert.NoError(t, saveJSON(caller, agentTurnPrefix+turn.ID, turn, agentTurnTTL))

	caller.lists[agentHistoryKey(sess.Agent, sess.ConversationID)] = []string{
		`{"role":"system","content":"init"}`,
		`{"role":"assistant","content":"hi"}`,
	}

	err := resolveTurnContext(caller, sess.ID, turn.ID)
	assert.NoError(t, err)

	storedSession := &agentSession{}
	loaded, err := loadJSON(caller, agentSessionPrefix+sess.ID, storedSession)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "selecting_provider", storedSession.State)

	storedTurn := &agentTurn{}
	loaded, err = loadJSON(caller, agentTurnPrefix+turn.ID, storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "context_resolved", storedTurn.State)

	resolved := storedTurn.ResolvedContext
	if assert.NotNil(t, resolved) {
		assert.Equal(t, "thread-123", resolved.ConversationID)
	}
	history := resolved.History
	assert.Len(t, history, 2)
	assert.Equal(t, agenthistory.RoleSystem, history[0].Role)
	if assert.NotNil(t, history[0].Content) {
		assert.Equal(t, "init", history[0].Content.Text)
	}

	event, ok := resolved.Event.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "hello", event["text"])

	workflow, ok := resolved.Workflow.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "success", workflow["status"])

	tools, ok := resolved.Tools.([]interface{})
	assert.True(t, ok)
	assert.Len(t, tools, 1)
	assert.Equal(t, "workflow", tools[0])
}

func TestLoadAgentHistory_Empty(t *testing.T) {
	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}

	history, err := loadAgentHistory(caller, "agent:history:support-bot:thread-empty")
	assert.NoError(t, err)
	assert.Len(t, history, 0)
}

func TestLoadAgentHistory_InvalidJSON(t *testing.T) {
	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	caller.lists["agent:history:support-bot:thread-bad"] = []string{"not-json"}

	_, err := loadAgentHistory(caller, "agent:history:support-bot:thread-bad")
	assert.Error(t, err)
}

func TestResolveTurnContext_SeedsHistoryFromAgentSystemPrompt(t *testing.T) {
	origAgent := agent
	defer func() { agent = origAgent }()
	agent = &Service{config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
		"support-bot": {SystemPrompt: "system prompt from config"},
	}}}}

	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	sess := &agentSession{
		ID:             "sess-seed",
		Agent:          "support-bot",
		ConversationID: "thread-seed",
		State:          "resolving_context",
		CurrentTurnID:  "turn-seed",
		CreatedAt:      "2026-01-01T00:00:00Z",
		UpdatedAt:      "2026-01-01T00:00:00Z",
	}
	turn := &agentTurn{
		ID:        "turn-seed",
		SessionID: "sess-seed",
		Agent:     "support-bot",
		State:     "created",
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
	assert.NoError(t, saveJSON(caller, agentSessionPrefix+sess.ID, sess, agentSessionTTL))
	assert.NoError(t, saveJSON(caller, agentTurnPrefix+turn.ID, turn, agentTurnTTL))

	err := resolveTurnContext(caller, sess.ID, turn.ID)
	assert.NoError(t, err)

	storedTurn := &agentTurn{}
	loaded, err := loadJSON(caller, agentTurnPrefix+turn.ID, storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	if assert.NotNil(t, storedTurn.ResolvedContext) {
		assert.Len(t, storedTurn.ResolvedContext.History, 1)
		assert.Equal(t, agenthistory.RoleSystem, storedTurn.ResolvedContext.History[0].Role)
		if assert.NotNil(t, storedTurn.ResolvedContext.History[0].Content) {
			assert.Equal(t, "system prompt from config", storedTurn.ResolvedContext.History[0].Content.Text)
		}
	}

	list := caller.lists[agentHistoryKey("support-bot", "thread-seed")]
	assert.Len(t, list, 1)
	assert.Contains(t, list[0], "system prompt from config")
}

func TestResolveTurnContext_EmptyHistoryWithoutPrompt(t *testing.T) {
	origAgent := agent
	defer func() { agent = origAgent }()
	agent = &Service{config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
		"support-bot": {},
	}}}}

	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	sess := &agentSession{
		ID:             "sess-empty",
		Agent:          "support-bot",
		ConversationID: "thread-empty",
		State:          "resolving_context",
		CurrentTurnID:  "turn-empty",
		CreatedAt:      "2026-01-01T00:00:00Z",
		UpdatedAt:      "2026-01-01T00:00:00Z",
	}
	turn := &agentTurn{
		ID:        "turn-empty",
		SessionID: "sess-empty",
		Agent:     "support-bot",
		State:     "created",
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
	assert.NoError(t, saveJSON(caller, agentSessionPrefix+sess.ID, sess, agentSessionTTL))
	assert.NoError(t, saveJSON(caller, agentTurnPrefix+turn.ID, turn, agentTurnTTL))

	err := resolveTurnContext(caller, sess.ID, turn.ID)
	assert.NoError(t, err)

	storedTurn := &agentTurn{}
	loaded, err := loadJSON(caller, agentTurnPrefix+turn.ID, storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	if assert.NotNil(t, storedTurn.ResolvedContext) {
		assert.Len(t, storedTurn.ResolvedContext.History, 0)
	}

	_, ok := caller.lists[agentHistoryKey("support-bot", "thread-empty")]
	assert.False(t, ok)
}

func TestResolveTurnContext_MissingEntities(t *testing.T) {
	caller := &fakeCacheCaller{data: map[string][]byte{}}

	err := resolveTurnContext(caller, "missing-session", "missing-turn")
	assert.Error(t, err)
}

func TestPersistedTurnJSONContainsResolvedContext(t *testing.T) {
	turn := &agentTurn{ResolvedContext: &resolvedTurnContext{ConversationID: "thread-123"}}
	b, err := json.Marshal(turn)
	assert.NoError(t, err)
	assert.Contains(t, string(b), "resolved_context")
}
