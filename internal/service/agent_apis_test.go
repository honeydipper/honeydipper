// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/agent"
	"github.com/honeydipper/honeydipper/v4/internal/api"
	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/require"
)

// recoveryMemHelper is an in-process stand-in for the Redis-backed cache/locker
// drivers, mirroring the helper used by the agent-package regression test. It
// is enough to seed a conversation, evict it, and observe handleConvoTurnAPI.
type recoveryMemHelper struct {
	mu      sync.Mutex
	cache   map[string]string
	emitted []*dipper.Message
	cfg     *config.Config
}

func newRecoveryMemHelper() *recoveryMemHelper {
	return &recoveryMemHelper{
		cache: map[string]string{},
		cfg: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
			"test_agent": {Name: "test_agent", Engine: "openai", Driver: "openai", ModelData: map[string]interface{}{}},
			"new_agent":  {Name: "new_agent", Engine: "openai", Driver: "openai", ModelData: map[string]interface{}{}},
		}}},
	}
}

func (h *recoveryMemHelper) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	p, _ := params.(map[string]interface{})
	switch feature + ":" + method {
	case "cache:load":
		k := p["key"].(string)
		h.mu.Lock()
		v, ok := h.cache[k]
		h.mu.Unlock()
		if !ok {
			return []byte{}, nil
		}

		return []byte(v), nil
	case "cache:save":
		k := p["key"].(string)
		v := p["value"].(string)
		h.mu.Lock()
		h.cache[k] = v
		h.mu.Unlock()

		return []byte("OK"), nil
	case "cache:del":
		k := p["key"].(string)
		h.mu.Lock()
		delete(h.cache, k)
		h.mu.Unlock()

		return []byte("1"), nil
	case "cache:lrange":
		k := p["key"].(string)
		h.mu.Lock()
		v, ok := h.cache[k]
		h.mu.Unlock()
		if !ok || v == "" {
			return []byte("[]"), nil
		}

		return []byte(v), nil
	case "locker:lock", "locker:unlock":
		return []byte(""), nil
	}

	return []byte("[]"), nil
}

func (h *recoveryMemHelper) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	return nil
}

func (h *recoveryMemHelper) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return h.Call(feature, method, nil, labelsKV...)
}

func (h *recoveryMemHelper) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}

func (h *recoveryMemHelper) GetName() string { return "recovery-mem-helper" }

func (h *recoveryMemHelper) GetConfig() *config.Config { return h.cfg }

func (h *recoveryMemHelper) EmitMessage(msg dipper.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emitted = append(h.emitted, &msg)
}

func (h *recoveryMemHelper) hasCache(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	_, ok := h.cache[key]

	return ok
}

// captureReceiver records the single message sent through the response.
type captureReceiver struct {
	mu       sync.Mutex
	captured *dipper.Message
}

func (c *captureReceiver) SendMessage(m *dipper.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.captured = m
}

// TestHandleConvoTurnAPI_EvictedConvoNoAgent_ReturnsExpired is a PHASE-2 test.
// It asserts the FIXED behavior: when convo_state:<id> has been reclaimed by Redis
// and no agent is supplied, handleConvoTurnAPI returns the ConversationExpiredResponse.
func TestHandleConvoTurnAPI_EvictedConvoNoAgent_ReturnsExpired(t *testing.T) {
	helper := newRecoveryMemHelper()
	convoID := "evicted-convo-svc-1"

	// (a) Seed an existing conversation, then (b) evict it.
	seeded := map[string]interface{}{
		"convo_id":     convoID,
		"last_session": map[string]interface{}{"agent_name": "test_agent", "type": "chat_turn"},
	}
	seededBytes, err := json.Marshal(seeded)
	require.NoError(t, err)
	helper.cache[agent.ConvoStateKeyPrefix+convoID] = string(seededBytes)
	if _, err := helper.Call("cache", "del", map[string]interface{}{
		"key": agent.ConvoStateKeyPrefix + convoID,
	}); err != nil {
		t.Fatalf("eviction del failed: %v", err)
	}
	if ok := helper.hasCache(agent.ConvoStateKeyPrefix + convoID); ok {
		t.Fatalf("eviction did not remove convo_state")
	}

	realStore := agent.NewAgentStore(helper, "")
	prev := agentStore
	agentStore = realStore
	defer func() { agentStore = prev }()

	receiver := &captureReceiver{}
	resp := &api.Response{
		EventBus: receiver,
		Request: &dipper.Message{
			Labels: map[string]string{"user": "u", "user_provider": "p"},
			Payload: map[string]interface{}{
				"convoID": convoID,
				"body":    `{"text":"hi"}`,
			},
		},
		Factrory: api.NewResponseFactory(),
	}
	// Return() calls Factrory.Live.Done(); balance it to avoid a panic.
	resp.Factrory.Live.Add(1)

	// (c) Drive the FIXED revive path through the real API handler.
	handleConvoTurnAPI(resp)

	receiver.mu.Lock()
	captured := receiver.captured
	receiver.mu.Unlock()
	require.NotNil(t, captured, "expected handleConvoTurnAPI to return a result")

	payload, ok := captured.Payload.([]byte)
	require.True(t, ok, "expected raw byte payload")
	require.JSONEq(t, ConversationExpiredResponse, string(payload),
		"fixed behavior must return conversation_expired response")

	// After the goroutine completes, the convo_state must remain absent (no
	// recreation) which confirms the turn never actually started.
	realStore.Wait()
	recreated := helper.hasCache(agent.ConvoStateKeyPrefix + convoID)
	require.False(t, recreated, "fixed behavior must not recreate convo_state on evicted convo without agent")
}

// TestHandleConvoTurnAPI_EvictedConvoWithAgent_Recovers asserts that when an
// agent is supplied for an evicted conversation, the conversation is recreated
// and the turn starts successfully.
func TestHandleConvoTurnAPI_EvictedConvoWithAgent_Recovers(t *testing.T) {
	helper := newRecoveryMemHelper()
	convoID := "evicted-convo-svc-2"

	// Seed and evict
	seeded := map[string]interface{}{
		"convo_id":     convoID,
		"last_session": map[string]interface{}{"agent_name": "old_agent", "type": "chat_turn"},
	}
	seededBytes, err := json.Marshal(seeded)
	require.NoError(t, err)
	helper.cache[agent.ConvoStateKeyPrefix+convoID] = string(seededBytes)
	if _, err := helper.Call("cache", "del", map[string]interface{}{
		"key": agent.ConvoStateKeyPrefix + convoID,
	}); err != nil {
		t.Fatalf("eviction del failed: %v", err)
	}

	realStore := agent.NewAgentStore(helper, "")
	prev := agentStore
	agentStore = realStore
	defer func() { agentStore = prev }()

	receiver := &captureReceiver{}
	resp := &api.Response{
		EventBus: receiver,
		Request: &dipper.Message{
			Labels: map[string]string{"user": "u", "user_provider": "p"},
			Payload: map[string]interface{}{
				"convoID": convoID,
				"body":    `{"text":"hi", "agent": "test_agent"}`,
			},
		},
		Factrory: api.NewResponseFactory(),
	}
	resp.Factrory.Live.Add(1)

	// Supply an agent - should recover and start turn
	handleConvoTurnAPI(resp)

	receiver.mu.Lock()
	captured := receiver.captured
	receiver.mu.Unlock()
	require.NotNil(t, captured, "expected handleConvoTurnAPI to return a result")

	payload, ok := captured.Payload.([]byte)
	require.True(t, ok, "expected raw byte payload")
	require.JSONEq(t, `{"ok":true}`, string(payload), "recovery must return ok true")

	// ConvoState should be recreated
	realStore.Wait()
	recreated := helper.hasCache(agent.ConvoStateKeyPrefix + convoID)
	require.True(t, recreated, "recovery must recreate convo_state")
}

// TestHandleConvoTurnAPI_LiveConvo_UsesExisting asserts normal turn behavior
// when ConvoState is present.
func TestHandleConvoTurnAPI_LiveConvo_UsesExisting(t *testing.T) {
	helper := newRecoveryMemHelper()
	convoID := "live-convo-svc-1"

	// Seed a live conversation (don't evict)
	seeded := map[string]interface{}{
		"convo_id":     convoID,
		"last_session": map[string]interface{}{"agent_name": "test_agent", "type": "chat_turn"},
	}
	seededBytes, err := json.Marshal(seeded)
	require.NoError(t, err)
	helper.cache[agent.ConvoStateKeyPrefix+convoID] = string(seededBytes)

	realStore := agent.NewAgentStore(helper, "")
	prev := agentStore
	agentStore = realStore
	defer func() { agentStore = prev }()

	receiver := &captureReceiver{}
	resp := &api.Response{
		EventBus: receiver,
		Request: &dipper.Message{
			Labels: map[string]string{"user": "u", "user_provider": "p"},
			Payload: map[string]interface{}{
				"convoID": convoID,
				"body":    `{"text":"hi"}`,
			},
		},
		Factrory: api.NewResponseFactory(),
	}
	resp.Factrory.Live.Add(1)

	// Normal turn - no agent needed in body
	handleConvoTurnAPI(resp)

	receiver.mu.Lock()
	captured := receiver.captured
	receiver.mu.Unlock()
	require.NotNil(t, captured)

	payload, ok := captured.Payload.([]byte)
	require.True(t, ok)
	require.JSONEq(t, `{"ok":true}`, string(payload))

	// ConvoState should still exist
	realStore.Wait()
	recreated := helper.hasCache(agent.ConvoStateKeyPrefix + convoID)
	require.True(t, recreated, "live convo must keep convo_state")
}
