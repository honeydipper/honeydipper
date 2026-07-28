// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"bytes"
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
		cfg:   &config.Config{DataSet: &config.DataSet{Agents: map[string]config.Agent{}}},
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
	leveled.SetLevel(logging.ERROR, "")
	leveled.SetLevel(logging.ERROR, "agent-recovery-test")
	logger := logging.MustGetLogger("agent-recovery-test")
	logger.SetBackend(leveled)

	return logger
}

// TestStartTurn_ConvoStateEvicted_RegressesToSilentNoOp is a PHASE-1 regression
// test. It encodes TODAY'S BROKEN behavior so that Phase 2's recovery fix flips
// it. When a conversation's convo_state:<id> key has been reclaimed by Redis
// (TTL/eviction) and no agent is supplied, StartTurn cannot resolve an agent
// name, logs "cannot determine agent name for convo <id>", returns silently,
// and the API handler still answers {"ok":true}. No turn starts, no history is
// appended, and ConvoState.ActiveSession is never set.
func TestStartTurn_ConvoStateEvicted_RegressesToSilentNoOp(t *testing.T) {
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

	// (c) Drive the broken revive path.
	store.StartTurn(convoID, "are you still there?", "user1", "", "")
	store.Wait()

	// Assertion 1: the unresolvable-agent branch was hit.
	logOut := logBuf.String()
	require.Contains(t, logOut, "cannot determine agent name for convo "+convoID,
		"expected StartTurn to hit the unresolvable-agent branch")

	// Assertion 2: ConvoState was NOT recreated (no new convo_state persisted).
	_, recreated := helper.getCache(ConvoStateKeyPrefix + convoID)
	require.False(t, recreated, "broken behavior must NOT recreate convo_state")
	require.Equal(t, savesBefore, helper.callCount("cache:save"),
		"broken behavior must not persist a new convo_state")

	// Assertion 3: no history appended to convo_history:<id>.
	hist, _ := helper.getCache(ConvoHistoryKeyPrefix + convoID)
	require.Empty(t, hist, "broken behavior must not append to convo_history")

	// Assertion 4: no model/AI driver call nor agent_response emitted (no turn).
	require.Empty(t, helper.getEmitted(), "broken behavior must not emit any message")
}

// TestStartTurn_ConvoStatePresent_UsesExistingAgent is the happy-path companion
// that documents the NORMAL behavior the evicted case diverges from. With a live
// ConvoState the agent name resolves and StartTurn proceeds past the guard (it
// only fails afterward because this test provides no real AI driver). It exists
// to make the eviction regression test's intent unambiguous.
func TestStartTurn_ConvoStatePresent_UsesExistingAgent(t *testing.T) {
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

	// With a present ConvoState the guard is NOT triggered: StartTurn proceeds
	// to runTurn, which eventually fails for lack of a real AI driver (not the
	// "cannot determine agent name" log). We only assert the guard log is absent.
	store.StartTurn(convoID, "hi", "user1", "", "")
	store.Wait()

	require.NotContains(t, logBuf.String(), "cannot determine agent name for convo "+convoID,
		"present convo_state must resolve an agent and skip the guard")
}
