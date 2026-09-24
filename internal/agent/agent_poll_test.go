// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"sync"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockingPollStore struct {
	*memCacheHelper

	mu             sync.Mutex
	lockName       string
	defaultTimeout time.Duration
	release        <-chan struct{}
	lockTimeouts   []time.Duration
}

func (h *blockingPollStore) Call(
	feature string,
	method string,
	params interface{},
	labelsKV ...string,
) ([]byte, error) {
	if feature == "locker" && method == "lock" {
		lockParams, _ := params.(map[string]interface{})
		if lockParams["name"] == h.lockName {
			timeout := h.defaultTimeout
			for i := 0; i+1 < len(labelsKV); i += 2 {
				if labelsKV[i] == "timeout" {
					timeout = dipper.Must(time.ParseDuration(labelsKV[i+1])).(time.Duration)

					break
				}
			}
			h.mu.Lock()
			h.lockTimeouts = append(h.lockTimeouts, timeout)
			h.mu.Unlock()

			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-h.release:
				return []byte(""), nil
			case <-timer.C:
				return nil, dipper.ErrTimeout
			}
		}
	}

	return h.memCacheHelper.Call(feature, method, params, labelsKV...)
}

func (h *blockingPollStore) firstLockTimeout(t *testing.T) time.Duration {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	require.NotEmpty(t, h.lockTimeouts)

	return h.lockTimeouts[0]
}

func seedPollSession(t *testing.T, helper *memCacheHelper, sessionID, convoID string, history []AgentMessage) {
	t.Helper()
	agent := helper.cfg.DataSet.Agents["test_agent"]
	session := &AgentSession{
		ID:      sessionID,
		ConvoID: convoID,
		Agent:   &agent,
		Type:    AgentSessionTypeChatTurn,
		TTL:     AgentSessionDefaultTTL,
	}
	helper.cache[AgentKeyPrefix+sessionID] = string(dipper.SerializeContent(session))
	helper.cache[ConvoHistoryKeyPrefix+convoID] = string(dipper.SerializeContent(history))
}

func TestEmitPollResponse_EmptyHistorySupportsPendingAndTerminalResponses(t *testing.T) {
	store := newMockStore(nil)
	agent := config.Agent{Name: "test-agent"}
	session := &AgentSession{
		ID:      "empty-history-session",
		ConvoID: "empty-history-convo",
		Agent:   &agent,
		store:   store,
	}
	msg := &dipper.Message{Labels: map[string]string{"resume_key": "workflow.1"}}

	require.NotPanics(t, func() {
		assert.False(t, session.emitPollResponse(msg))
	})
	assert.Empty(t, store.getEmitted())

	session.NewPendingContent = true
	session.PendingContent = "still working"
	require.NotPanics(t, func() {
		assert.True(t, session.emitPollResponse(msg))
	})
	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	payload := emitted[0].Payload.(map[string]interface{})
	assert.Equal(t, map[string]string{
		"content":  "still working",
		"is_chunk": "true",
	}, payload["live_message"])
	assert.Equal(t, "success", msg.Labels["status"])

	session.history = []AgentMessage{{Role: RoleAgent, Content: "done", IsComplete: true}}
	require.NotPanics(t, func() {
		assert.True(t, session.emitPollResponse(msg))
	})
	emitted = store.getEmitted()
	require.Len(t, emitted, 2)
	terminalPayload := emitted[1].Payload.(map[string]interface{})
	fullMessages := terminalPayload["full_messages"].([]map[string]string)
	require.Len(t, fullMessages, 1)
	assert.Equal(t, "done", fullMessages[0]["content"])
	assert.Equal(t, "success", msg.Labels["status"])
}

func TestEmitPollResponse_EmptyHistoryReturnsSessionError(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:          "failed-empty-history-session",
		Agent:       &config.Agent{Name: "test-agent"},
		store:       store,
		ErrorReason: "agent failed before creating history",
	}
	msg := &dipper.Message{Labels: map[string]string{"resume_key": "workflow.2"}}

	require.NotPanics(t, func() {
		assert.True(t, session.emitPollResponse(msg))
	})
	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "error", msg.Labels["status"])
	assert.Equal(t, session.ErrorReason, msg.Labels["reason"])
}

func TestPollInference_WaitsForOverlappingTurnAndReturnsTerminalResponse(t *testing.T) {
	release := make(chan struct{})
	helper := &blockingPollStore{
		memCacheHelper: newMemCacheHelper(),
		lockName:       AgentKeyPrefix + "queued-session",
		defaultTimeout: 10 * time.Millisecond,
		release:        release,
	}
	seedPollSession(t, helper.memCacheHelper, "queued-session", "shared-convo", []AgentMessage{
		{Role: RoleAgent, Content: "resolved", IsComplete: true},
	})
	store := NewAgentStore(helper, "").(*PersistentAgentStore)
	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_session_id": "queued-session",
			"resume_key":       "workflow.3",
			"timeout":          "250ms",
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		store.PollInference(msg)
	}()
	time.Sleep(30 * time.Millisecond)
	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("PollInference did not finish after the queued turn released its session lock")
	}
	assert.Greater(t, helper.firstLockTimeout(t), helper.defaultTimeout)
	emitted := helper.getEmitted()
	require.NotEmpty(t, emitted)
	response := emitted[0]
	assert.Equal(t, "success", msg.Labels["status"])
	payload := response.Payload.(map[string]interface{})
	fullMessages := payload["full_messages"].([]map[string]string)
	require.Len(t, fullMessages, 1)
	assert.Equal(t, "resolved", fullMessages[0]["content"])
}

func TestPollInference_SessionLockUsesEndToEndPollTimeout(t *testing.T) {
	release := make(chan struct{})
	helper := &blockingPollStore{
		memCacheHelper: newMemCacheHelper(),
		lockName:       AgentKeyPrefix + "blocked-session",
		defaultTimeout: 5 * time.Millisecond,
		release:        release,
	}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)
	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_session_id": "blocked-session",
			"resume_key":       "workflow.4",
			"timeout":          "40ms",
		},
	}

	store.PollInference(msg)

	assert.Greater(t, helper.firstLockTimeout(t), helper.defaultTimeout)
	emitted := helper.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "failure", emitted[0].Labels["status"])
	assert.Equal(t, "poll timeout after 40ms", emitted[0].Labels["reason"])
	assert.NotEqual(t, "timeout", emitted[0].Labels["reason"])
}
