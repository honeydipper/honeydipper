// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/require"
)

// memCacheHelper is an in-process stand-in for the Redis-backed cache/locker
// drivers. It is intentionally minimal: it only needs to faithfully model the
// convo_state:<id> / convo_history:<id> keys and the lock operations that
// PersistentAgentStore touches, so a test can deterministically create a
// conversation, evict it, and observe the broken revive behavior.
type memCacheHelper struct {
	mu      sync.Mutex
	cache   map[string]string
	emitted []*dipper.Message
	calls   []string
	cfg     *config.Config
}

func newMemCacheHelper() *memCacheHelper {
	return &memCacheHelper{
		cache: map[string]string{},
		cfg: &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{
			"test_agent":      {Name: "test_agent", Engine: "openai", Driver: "openai", ModelData: map[string]interface{}{}},
			"new_agent":       {Name: "new_agent", Engine: "openai", Driver: "openai", ModelData: map[string]interface{}{}},
			"old_agent":       {Name: "old_agent", Engine: "openai", Driver: "openai", ModelData: map[string]interface{}{}},
			"original_agent":  {Name: "original_agent", Engine: "openai", Driver: "openai", ModelData: map[string]interface{}{}},
			"different_agent": {Name: "different_agent", Engine: "openai", Driver: "openai", ModelData: map[string]interface{}{}},
		}}},
	}
}

func (h *memCacheHelper) record(call string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, call)
}

// Call implements dipper.RPCCaller for the subset of cache/locker methods the
// agent store uses. Unknown methods return an empty JSON array so callers that
// expect a slice are not surprised.
func (h *memCacheHelper) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	key := feature + ":" + method
	h.record(key)
	p, _ := params.(map[string]interface{})
	switch key {
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
	case "cache:rpush":
		k := p["key"].(string)
		v := p["value"].(string)
		h.mu.Lock()
		if cur, ok := h.cache[k]; ok && cur != "" && cur != "[]" {
			h.cache[k] = cur[:len(cur)-1] + "," + v + "]"
		} else {
			h.cache[k] = "[" + v + "]"
		}
		h.mu.Unlock()

		return []byte("1"), nil
	case "cache:ltrim", "cache:stream_hset":
		return []byte("OK"), nil
	case "locker:lock", "locker:unlock":
		return []byte(""), nil
	}

	return []byte("[]"), nil
}

func (h *memCacheHelper) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	h.record(feature + ":" + method)

	return nil
}

func (h *memCacheHelper) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return h.Call(feature, method, nil, labelsKV...)
}

func (h *memCacheHelper) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	h.record(feature + ":" + method)

	return nil
}

func (h *memCacheHelper) GetName() string { return "mem-cache-helper" }

func (h *memCacheHelper) GetConfig() *config.Config { return h.cfg }

func (h *memCacheHelper) EmitMessage(msg dipper.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emitted = append(h.emitted, &msg)
}

func (h *memCacheHelper) getCache(key string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.cache[key]

	return v, ok
}

func (h *memCacheHelper) callCount(call string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, c := range h.calls {
		if c == call {
			n++
		}
	}

	return n
}

func (h *memCacheHelper) getEmitted() []*dipper.Message {
	h.mu.Lock()
	defer h.mu.Unlock()

	return append([]*dipper.Message(nil), h.emitted...)
}

// newCaptureLogger returns a logger that writes only ERROR (and above) records
// as plain messages into buf. It is used to assert that a broken code path
// emitted the expected error log line.
func newCaptureLogger(buf *bytes.Buffer) *logging.Logger {
	backend := logging.NewLogBackend(buf, "", 0)
	format := logging.MustStringFormatter("%{message}")
	formatted := logging.NewBackendFormatter(backend, format)
	leveled := logging.AddModuleLevel(formatted)
	leveled.SetLevel(logging.INFO, "")
	leveled.SetLevel(logging.INFO, "agent-recovery-test")
	logger := logging.MustGetLogger("agent-recovery-test")
	logger.SetBackend(leveled)

	return logger
}

// TestStartTurn_ConvoStateEvicted_NoAgent_ReturnsError is a PHASE-2 test.
// It asserts the FIXED behavior: when a conversation's convo_state:<id> key has been
// reclaimed by Redis (TTL/eviction) and no agent is supplied, StartTurn returns an error
// (does not silently succeed). No turn starts, no history is appended, and ConvoState
// ActiveSession is never set.
func TestStartTurn_ConvoStateEvicted_NoAgent_ReturnsError(t *testing.T) {
	helper := newMemCacheHelper()
	convoID := "evicted-convo-1"

	// (a) Seed an existing conversation so a convo_state:<id> exists.
	seeded := &ConvoState{
		ConvoID: convoID,
		FirstSession: &ConvoSessionRef{
			SessionID: "sess-prev",
			AgentName: "test_agent",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
		LastSession: &ConvoSessionRef{
			SessionID: "sess-prev",
			AgentName: "test_agent",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
	}
	helper.cache[ConvoStateKeyPrefix+convoID] = string(dipper.SerializeContent(seeded))

	// Sanity: the seeded state is present before eviction.
	if _, ok := helper.getCache(ConvoStateKeyPrefix + convoID); !ok {
		t.Fatalf("test setup failed: seeded convo_state missing")
	}

	// (b) Evict the convo_state:<id> key to simulate Redis reclaim/TTL.
	if _, err := helper.Call("cache", "del", map[string]interface{}{
		"key": ConvoStateKeyPrefix + convoID,
	}); err != nil {
		t.Fatalf("eviction del failed: %v", err)
	}
	if _, ok := helper.getCache(ConvoStateKeyPrefix + convoID); ok {
		t.Fatalf("eviction did not remove convo_state")
	}

	var logBuf bytes.Buffer
	store := &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      newCaptureLogger(&logBuf),
	}

	savesBefore := helper.callCount("cache:save")

	// (c) Drive the FIXED revive path - no agent supplied.
	err := store.StartTurn(convoID, "are you still there?", "user1", "", "", "", false)
	store.Wait()

	// Assertion 1: StartTurn returns an error (unrecoverable).
	require.Error(t, err, "StartTurn must return error when convo evicted and no agent supplied")
	require.Contains(t, err.Error(), "conversation "+convoID+" expired and no agent supplied to recreate")

	// Assertion 2: ConvoState was NOT recreated (no new convo_state persisted).
	_, recreated := helper.getCache(ConvoStateKeyPrefix + convoID)
	require.False(t, recreated, "fixed behavior must NOT recreate convo_state when no agent supplied")
	require.Equal(t, savesBefore, helper.callCount("cache:save"), "fixed behavior must not persist a new convo_state")

	// Assertion 3: no history appended to convo_history:<id>.
	hist, _ := helper.getCache(ConvoHistoryKeyPrefix + convoID)
	require.Empty(t, hist, "fixed behavior must not append to convo_history")

	// Assertion 4: no model/AI driver call nor agent_response emitted (no turn).
	require.Empty(t, helper.getEmitted(), "fixed behavior must not emit any message")

	// Assertion 5: the old "cannot determine agent name" log is NOT present.
	require.NotContains(t, logBuf.String(), "cannot determine agent name for convo "+convoID,
		"fixed behavior must not log the old unresolvable-agent message")
}

// TestStartTurn_ConvoStateEvicted_WithAgent_Recovers recreates the ConvoState
// when an agent is supplied for an evicted conversation. This is the core
// recovery path: the conversation continues seamlessly with a new turn.
func TestStartTurn_ConvoStateEvicted_WithAgent_Recovers(t *testing.T) {
	helper := newMemCacheHelper()
	convoID := "evicted-convo-2"

	// (a) Seed and then evict (same as above).
	seeded := &ConvoState{
		ConvoID: convoID,
		FirstSession: &ConvoSessionRef{
			SessionID: "sess-prev",
			AgentName: "old_agent",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
		LastSession: &ConvoSessionRef{
			SessionID: "sess-prev",
			AgentName: "old_agent",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
	}
	helper.cache[ConvoStateKeyPrefix+convoID] = string(dipper.SerializeContent(seeded))
	if _, err := helper.Call("cache", "del", map[string]interface{}{
		"key": ConvoStateKeyPrefix + convoID,
	}); err != nil {
		t.Fatalf("eviction del failed: %v", err)
	}

	var logBuf bytes.Buffer
	store := &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      newCaptureLogger(&logBuf),
	}

	// (b) Drive the recovery path - agent supplied, no override needed (cs missing).
	err := store.StartTurn(convoID, "are you still there?", "user1", "", "", "test_agent", false)
	require.NoError(t, err, "StartTurn must succeed when agent supplied for evicted convo")
	store.Wait()

	// Assertion 1: ConvoState WAS recreated with the new agent.
	csData, ok := helper.getCache(ConvoStateKeyPrefix + convoID)
	require.True(t, ok, "recovery must recreate convo_state")
	var cs ConvoState
	require.NoError(t, json.Unmarshal([]byte(csData), &cs))
	require.Equal(t, "test_agent", cs.Agent.Name, "recreated ConvoState must have the supplied agent")

	// Assertion 2: history was appended (turn started).
	hist, _ := helper.getCache(ConvoHistoryKeyPrefix + convoID)
	require.NotEmpty(t, hist, "recovery must append to convo_history")

	// Assertion 3: no "cannot determine agent name" log.
	require.NotContains(t, logBuf.String(), "cannot determine agent name",
		"recovery must not log the old unresolvable-agent message")

	// Assertion 4: log should indicate recreation.
	require.Contains(t, logBuf.String(), "recreated", "recovery should log recreated state")
}

// TestStartTurn_ConvoStatePresent_NoOverride_UsesExisting asserts the
// normal turn path: when ConvoState is present and no agent override,
// the existing state is used (agent resolved from LastSession/FirstSession).
func TestStartTurn_ConvoStatePresent_NoOverride_UsesExisting(t *testing.T) {
	helper := newMemCacheHelper()
	convoID := "live-convo-1"

	seeded := &ConvoState{
		ConvoID: convoID,
		LastSession: &ConvoSessionRef{
			SessionID: "sess-live",
			AgentName: "test_agent",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
	}
	helper.cache[ConvoStateKeyPrefix+convoID] = string(dipper.SerializeContent(seeded))

	var logBuf bytes.Buffer
	store := &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      newCaptureLogger(&logBuf),
	}

	// With a present ConvoState and no agent supplied, the guard is NOT triggered.
	// StartTurn proceeds to runTurn (which fails for lack of real AI driver).
	err := store.StartTurn(convoID, "hi", "user1", "", "", "", false)
	require.NoError(t, err, "StartTurn must succeed when cs present")

	// Wait for the turn to complete (it will fail at driver level, not at agent resolution).
	store.Wait()

	require.NotContains(t, logBuf.String(), "cannot determine agent name for convo "+convoID,
		"present convo_state must resolve an agent and skip the guard")

	// ConvoState should still have the original agent (not recreated).
	csData, ok := helper.getCache(ConvoStateKeyPrefix + convoID)
	require.True(t, ok)
	var cs ConvoState
	require.NoError(t, json.Unmarshal([]byte(csData), &cs))
	require.Equal(t, "test_agent", cs.Agent.Name, "existing agent must be preserved")
}

// TestStartTurn_ConvoStatePresent_Override_Recreates asserts that when
// ConvoState is present but agentOverride=true with a new agent, the state
// is recreated with the new agent.
func TestStartTurn_ConvoStatePresent_Override_Recreates(t *testing.T) {
	helper := newMemCacheHelper()
	convoID := "live-convo-override"

	seeded := &ConvoState{
		ConvoID: convoID,
		LastSession: &ConvoSessionRef{
			SessionID: "sess-live",
			AgentName: "old_agent",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
	}
	helper.cache[ConvoStateKeyPrefix+convoID] = string(dipper.SerializeContent(seeded))

	var logBuf bytes.Buffer
	store := &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      newCaptureLogger(&logBuf),
	}

	// With agentOverride=true and a different agent, the state should be recreated.
	err := store.StartTurn(convoID, "hi", "user1", "", "", "new_agent", true)
	require.NoError(t, err)
	store.Wait()

	// ConvoState should be recreated with the new agent.
	csData, ok := helper.getCache(ConvoStateKeyPrefix + convoID)
	require.True(t, ok)
	var cs ConvoState
	require.NoError(t, json.Unmarshal([]byte(csData), &cs))
	require.Equal(t, "new_agent", cs.Agent.Name, "override must recreate with new agent")
	require.Contains(t, logBuf.String(), "recreated", "override should log recreated state")
}

// TestStartTurn_ConvoStatePresent_WithAgentNoOverride_SticksToExisting asserts
// that when ConvoState is present AND agent is supplied but agentOverride=false,
// the existing state is used (agent from state is kept, supplied agent ignored).
func TestStartTurn_ConvoStatePresent_WithAgentNoOverride_SticksToExisting(t *testing.T) {
	helper := newMemCacheHelper()
	convoID := "live-convo-stick"

	seeded := &ConvoState{
		ConvoID: convoID,
		LastSession: &ConvoSessionRef{
			SessionID: "sess-live",
			AgentName: "original_agent",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
	}
	helper.cache[ConvoStateKeyPrefix+convoID] = string(dipper.SerializeContent(seeded))

	var logBuf bytes.Buffer
	store := &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      newCaptureLogger(&logBuf),
	}

	// Supply a different agent but NO override - should stick to existing.
	err := store.StartTurn(convoID, "hi", "user1", "", "", "different_agent", false)
	require.NoError(t, err)
	store.Wait()

	// ConvoState should still have the original agent.
	csData, ok := helper.getCache(ConvoStateKeyPrefix + convoID)
	require.True(t, ok)
	var cs ConvoState
	require.NoError(t, json.Unmarshal([]byte(csData), &cs))
	require.Equal(t, "original_agent", cs.Agent.Name, "stick must keep original agent")
	require.NotContains(t, logBuf.String(), "recreated", "stick must not log recreated state")
}
