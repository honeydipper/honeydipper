// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/agenthistory"
	"github.com/honeydipper/honeydipper/v4/pkg/agentruntime"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

type (
	agentSession            = agentruntime.Session
	agentTurn               = agentruntime.Turn
	resolvedTurnContext     = agentruntime.ResolvedTurnContext
	activationPersistResult = agentruntime.ActivationPersistResult
)

const (
	agentSessionTTL = agentruntime.SessionTTL
	agentTurnTTL    = agentruntime.TurnTTL
)

var errAgentProviderMissing = agentruntime.ErrProviderMissing

func agentSessionKey(id string) string {
	return agentruntime.SessionKey(id)
}

func agentTurnKey(id string) string {
	return agentruntime.TurnKey(id)
}

func agentHistoryKey(agentName string, conversationID string) string {
	return agentruntime.HistoryKey(agentName, conversationID)
}

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
	origEnqueue := enqueueProviderFn
	defer func() { agent = origAgent }()
	defer func() { enqueueProviderFn = origEnqueue }()
	agent = &Service{config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
		"support-bot": {
			SystemPrompt: "you are helpful",
			Provider:     "openai-main",
			Tools:        []any{"agent-tool-1", "agent-tool-2"},
		},
	}}}}

	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	var cmd *dipper.Message
	enqueueProviderFn = func(msg *dipper.Message) error {
		cmd = msg

		return nil
	}

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
			"provider":        "ctx-provider",
		},
		Workflow: map[string]any{
			"status": "success",
		},
		Tools:     []any{"workflow"},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	assert.NoError(t, agentruntime.SaveJSON(caller, agentSessionKey(sess.ID), sess, agentSessionTTL))
	assert.NoError(t, agentruntime.SaveJSON(caller, agentTurnKey(turn.ID), turn, agentTurnTTL))

	caller.lists[agentHistoryKey(sess.Agent, sess.ConversationID)] = []string{
		`{"role":"system","content":"init"}`,
		`{"role":"assistant","content":"hi"}`,
	}

	err := resolveTurnContext(caller, sess.ID, turn.ID)
	assert.NoError(t, err)

	storedSession := &agentSession{}
	loaded, err := agentruntime.LoadJSON(caller, agentSessionKey(sess.ID), storedSession)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "waiting_provider", storedSession.State)

	storedTurn := &agentTurn{}
	loaded, err = agentruntime.LoadJSON(caller, agentTurnKey(turn.ID), storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "waiting_provider", storedTurn.State)
	assert.Equal(t, "ctx-provider", storedTurn.Provider)
	storedTools, ok := storedTurn.Tools.([]interface{})
	assert.True(t, ok)
	assert.Len(t, storedTools, 2)
	assert.Equal(t, "agent-tool-1", storedTools[0])
	assert.Nil(t, storedTurn.ProviderStart)
	if assert.NotNil(t, cmd) {
		assert.Equal(t, dipper.ChannelEventbus, cmd.Channel)
		assert.Equal(t, dipper.EventbusAgentCommand, cmd.Subject)
		assert.Equal(t, sess.ID, cmd.Labels["sessionID"])
		assert.Equal(t, turn.ID, cmd.Labels["turnID"])
		payload := cmd.Payload.(map[string]any)
		params := payload["data"].(map[string]any)
		assert.Equal(t, "thread-123", params["convID"])
		assert.Equal(t, "agent", params["user"])
		assert.Equal(t, "", params["prompt"])
	}

	resolved := storedTurn.ResolvedContext
	if assert.NotNil(t, resolved) {
		assert.Equal(t, "thread-123", resolved.ConversationID)
		assert.Equal(t, "ctx-provider", resolved.Provider)
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
	assert.Len(t, tools, 2)
	assert.Equal(t, "agent-tool-1", tools[0])
	assert.Equal(t, "agent-tool-2", tools[1])
}

func TestLoadAgentHistory_Empty(t *testing.T) {
	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}

	history, err := agentruntime.LoadHistory(caller, "agent:history:support-bot:thread-empty")
	assert.NoError(t, err)
	assert.Len(t, history, 0)
}

func TestLoadAgentHistory_InvalidJSON(t *testing.T) {
	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	caller.lists["agent:history:support-bot:thread-bad"] = []string{"not-json"}

	_, err := agentruntime.LoadHistory(caller, "agent:history:support-bot:thread-bad")
	assert.Error(t, err)
}

func TestResolveTurnContext_SeedsHistoryFromAgentSystemPrompt(t *testing.T) {
	origAgent := agent
	origEnqueue := enqueueProviderFn
	defer func() { agent = origAgent }()
	defer func() { enqueueProviderFn = origEnqueue }()
	agent = &Service{config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
		"support-bot": {SystemPrompt: "system prompt from config", Provider: "openai-main"},
	}}}}
	enqueueProviderFn = func(msg *dipper.Message) error { return nil }

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
	assert.NoError(t, agentruntime.SaveJSON(caller, agentSessionKey(sess.ID), sess, agentSessionTTL))
	assert.NoError(t, agentruntime.SaveJSON(caller, agentTurnKey(turn.ID), turn, agentTurnTTL))

	err := resolveTurnContext(caller, sess.ID, turn.ID)
	assert.NoError(t, err)

	storedTurn := &agentTurn{}
	loaded, err := agentruntime.LoadJSON(caller, agentTurnKey(turn.ID), storedTurn)
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

func TestResolveTurnContext_UsesAgentDefaultProvider(t *testing.T) {
	origAgent := agent
	origEnqueue := enqueueProviderFn
	defer func() { agent = origAgent }()
	defer func() { enqueueProviderFn = origEnqueue }()
	agent = &Service{config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
		"support-bot": {Provider: "agent-default-provider", Providers: []string{"fallback-provider"}},
	}}}}
	enqueueProviderFn = func(msg *dipper.Message) error { return nil }

	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	sess := &agentSession{
		ID:             "sess-provider-default",
		Agent:          "support-bot",
		ConversationID: "thread-provider-default",
		State:          "resolving_context",
		CurrentTurnID:  "turn-provider-default",
		CreatedAt:      "2026-01-01T00:00:00Z",
		UpdatedAt:      "2026-01-01T00:00:00Z",
	}
	turn := &agentTurn{
		ID:        "turn-provider-default",
		SessionID: "sess-provider-default",
		Agent:     "support-bot",
		State:     "created",
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
	assert.NoError(t, agentruntime.SaveJSON(caller, agentSessionKey(sess.ID), sess, agentSessionTTL))
	assert.NoError(t, agentruntime.SaveJSON(caller, agentTurnKey(turn.ID), turn, agentTurnTTL))

	err := resolveTurnContext(caller, sess.ID, turn.ID)
	assert.NoError(t, err)

	storedTurn := &agentTurn{}
	loaded, err := agentruntime.LoadJSON(caller, agentTurnKey(turn.ID), storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "waiting_provider", storedTurn.State)
	assert.Equal(t, "agent-default-provider", storedTurn.Provider)
}

func TestResolveTurnContext_UsesFirstProviderWhenDefaultMissing(t *testing.T) {
	origAgent := agent
	origEnqueue := enqueueProviderFn
	defer func() { agent = origAgent }()
	defer func() { enqueueProviderFn = origEnqueue }()
	agent = &Service{config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
		"support-bot": {Providers: []string{"", "list-provider-1", "list-provider-2"}},
	}}}}
	enqueueProviderFn = func(msg *dipper.Message) error { return nil }

	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	sess := &agentSession{
		ID:             "sess-provider-list",
		Agent:          "support-bot",
		ConversationID: "thread-provider-list",
		State:          "resolving_context",
		CurrentTurnID:  "turn-provider-list",
		CreatedAt:      "2026-01-01T00:00:00Z",
		UpdatedAt:      "2026-01-01T00:00:00Z",
	}
	turn := &agentTurn{
		ID:        "turn-provider-list",
		SessionID: "sess-provider-list",
		Agent:     "support-bot",
		State:     "created",
		Ctx: map[string]any{
			"conversation_id": "thread-provider-list",
		},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
	assert.NoError(t, agentruntime.SaveJSON(caller, agentSessionKey(sess.ID), sess, agentSessionTTL))
	assert.NoError(t, agentruntime.SaveJSON(caller, agentTurnKey(turn.ID), turn, agentTurnTTL))

	err := resolveTurnContext(caller, sess.ID, turn.ID)
	assert.NoError(t, err)

	storedTurn := &agentTurn{}
	loaded, err := agentruntime.LoadJSON(caller, agentTurnKey(turn.ID), storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "waiting_provider", storedTurn.State)
	assert.Equal(t, "list-provider-1", storedTurn.Provider)
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
	assert.NoError(t, agentruntime.SaveJSON(caller, agentSessionKey(sess.ID), sess, agentSessionTTL))
	assert.NoError(t, agentruntime.SaveJSON(caller, agentTurnKey(turn.ID), turn, agentTurnTTL))

	err := resolveTurnContext(caller, sess.ID, turn.ID)
	assert.ErrorIs(t, err, errAgentProviderMissing)

	storedTurn := &agentTurn{}
	loaded, err := agentruntime.LoadJSON(caller, agentTurnKey(turn.ID), storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "failed", storedTurn.State)
	assert.Equal(t, errAgentProviderMissing.Error(), storedTurn.FailureReason)

	storedSession := &agentSession{}
	loaded, err = agentruntime.LoadJSON(caller, agentSessionKey(sess.ID), storedSession)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "failed", storedSession.State)

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

func TestResolveTurnContext_ProviderChatError(t *testing.T) {
	origAgent := agent
	origEnqueue := enqueueProviderFn
	defer func() { agent = origAgent }()
	defer func() { enqueueProviderFn = origEnqueue }()
	agent = &Service{config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
		"support-bot": {Provider: "openai-main"},
	}}}}
	enqueueProviderFn = func(msg *dipper.Message) error { return errors.New("rpc down") }

	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	sess := &agentSession{
		ID:             "sess-provider-error",
		Agent:          "support-bot",
		ConversationID: "thread-provider-error",
		State:          "resolving_context",
		CurrentTurnID:  "turn-provider-error",
		CreatedAt:      "2026-01-01T00:00:00Z",
		UpdatedAt:      "2026-01-01T00:00:00Z",
	}
	turn := &agentTurn{
		ID:        "turn-provider-error",
		SessionID: "sess-provider-error",
		Agent:     "support-bot",
		State:     "created",
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
	assert.NoError(t, agentruntime.SaveJSON(caller, agentSessionKey(sess.ID), sess, agentSessionTTL))
	assert.NoError(t, agentruntime.SaveJSON(caller, agentTurnKey(turn.ID), turn, agentTurnTTL))

	err := resolveTurnContext(caller, sess.ID, turn.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "start provider command")

	storedTurn := &agentTurn{}
	loaded, err := agentruntime.LoadJSON(caller, agentTurnKey(turn.ID), storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "failed", storedTurn.State)
	assert.Contains(t, storedTurn.FailureReason, "start provider command")
}

func TestContinueProviderTurn(t *testing.T) {
	origAgent := agent
	origStateCaller := agentStateCallerFn
	origEnqueue := enqueueProviderFn
	defer func() { agent = origAgent }()
	defer func() { agentStateCallerFn = origStateCaller }()
	defer func() { enqueueProviderFn = origEnqueue }()
	agent = &Service{ready: make(chan struct{}), config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{}}}}
	close(agent.ready)
	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	agentStateCallerFn = func() dipper.RPCCaller { return caller }
	var cmd *dipper.Message
	enqueueProviderFn = func(msg *dipper.Message) error {
		cmd = msg

		return nil
	}

	sess := &agentSession{ID: "sess-cp", Agent: "support-bot", ConversationID: "thread-cp", State: "waiting_provider", CurrentTurnID: "turn-cp", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	turn := &agentTurn{ID: "turn-cp", SessionID: "sess-cp", Agent: "support-bot", Provider: "openai-main", State: "waiting_provider", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	assert.NoError(t, agentruntime.SaveJSON(caller, agentSessionKey(sess.ID), sess, agentSessionTTL))
	assert.NoError(t, agentruntime.SaveJSON(caller, agentTurnKey(turn.ID), turn, agentTurnTTL))

	msg := &dipper.Message{Channel: dipper.ChannelEventbus, Subject: dipper.EventbusAgentReturn, Labels: map[string]string{"sessionID": "sess-cp", "turnID": "turn-cp"}, Payload: map[string]any{"counter": "1", "convID": "thread-cp"}}
	continueProviderTurn(nil, msg)

	storedTurn := &agentTurn{}
	loaded, err := agentruntime.LoadJSON(caller, agentTurnKey(turn.ID), storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "waiting_provider_chunk", storedTurn.State)
	start, ok := storedTurn.ProviderStart.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "1", start["counter"])
	if assert.NotNil(t, cmd) {
		assert.Equal(t, dipper.EventbusAgentCommand, cmd.Subject)
		payload := cmd.Payload.(map[string]any)
		function := payload["function"].(config.Function)
		assert.Equal(t, "openai-main", function.Driver)
		assert.Equal(t, "chatContinue", function.RawAction)
		data := payload["data"].(map[string]any)
		assert.Equal(t, "thread-cp", data["convID"])
		assert.Equal(t, "1", data["counter"])
	}

	storedSession := &agentSession{}
	loaded, err = agentruntime.LoadJSON(caller, agentSessionKey(sess.ID), storedSession)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "waiting_provider_chunk", storedSession.State)
}

func TestContinueProviderTurn_Done(t *testing.T) {
	origAgent := agent
	origStateCaller := agentStateCallerFn
	origEnqueue := enqueueProviderFn
	defer func() { agent = origAgent }()
	defer func() { agentStateCallerFn = origStateCaller }()
	defer func() { enqueueProviderFn = origEnqueue }()
	agent = &Service{ready: make(chan struct{}), config: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{}}}}
	close(agent.ready)
	caller := &fakeCacheCaller{data: map[string][]byte{}, lists: map[string][]string{}}
	agentStateCallerFn = func() dipper.RPCCaller { return caller }
	enqueueProviderFn = func(msg *dipper.Message) error {
		t.Fatalf("enqueue should not be called for done payload")

		return nil
	}

	sess := &agentSession{ID: "sess-done", Agent: "support-bot", ConversationID: "thread-done", State: "waiting_provider_chunk", CurrentTurnID: "turn-done", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	turn := &agentTurn{ID: "turn-done", SessionID: "sess-done", Agent: "support-bot", Provider: "openai-main", State: "waiting_provider_chunk", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	assert.NoError(t, agentruntime.SaveJSON(caller, agentSessionKey(sess.ID), sess, agentSessionTTL))
	assert.NoError(t, agentruntime.SaveJSON(caller, agentTurnKey(turn.ID), turn, agentTurnTTL))

	msg := &dipper.Message{Channel: dipper.ChannelEventbus, Subject: dipper.EventbusAgentReturn, Labels: map[string]string{"sessionID": "sess-done", "turnID": "turn-done"}, Payload: map[string]any{"done": true, "content": "all done"}}
	continueProviderTurn(nil, msg)

	storedTurn := &agentTurn{}
	loaded, err := agentruntime.LoadJSON(caller, agentTurnKey(turn.ID), storedTurn)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "streaming_complete", storedTurn.State)

	storedSession := &agentSession{}
	loaded, err = agentruntime.LoadJSON(caller, agentSessionKey(sess.ID), storedSession)
	assert.NoError(t, err)
	assert.True(t, loaded)
	assert.Equal(t, "streaming_complete", storedSession.State)
}

func TestPersistedTurnJSONContainsResolvedContext(t *testing.T) {
	turn := &agentTurn{ResolvedContext: &resolvedTurnContext{ConversationID: "thread-123"}}
	b, err := json.Marshal(turn)
	assert.NoError(t, err)
	assert.Contains(t, string(b), "resolved_context")
}
