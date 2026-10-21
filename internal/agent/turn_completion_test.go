// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration

package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deliver the model response only after the supervisor has loaded its initial
// history and released the session lock to wait. No scheduling race is needed.
type responseDuringPoll struct {
	*memCacheHelper
	onUnlock     func()
	onTurnUnlock func()
	turnOwner    string
	turnUnlocks  []string
	unlockErr    error
}

func (h *responseDuringPoll) Call(feature, method string, params interface{}, labels ...string) ([]byte, error) {
	p, _ := params.(map[string]interface{})
	if feature == "locker" && method == "unlock" && strings.HasPrefix(p["name"].(string), ConvoTurnLockPrefix) {
		owner, _ := p["owner"].(string)
		h.turnUnlocks = append(h.turnUnlocks, owner)
		if h.onTurnUnlock != nil {
			callback := h.onTurnUnlock
			h.onTurnUnlock = nil
			callback()
		}
		if h.unlockErr != nil {
			return nil, h.unlockErr
		}
		if owner == "" {
			if h.turnOwner == "" {
				return nil, errors.New("fail to unlock")
			}
			h.turnOwner = ""
		} else if owner == h.turnOwner {
			h.turnOwner = ""
		}
	}
	if feature == "locker" && method == "unlock" && p["name"] == AgentKeyPrefix+"turn" && h.onUnlock != nil {
		callback := h.onUnlock
		h.onUnlock = nil
		callback()
	}

	return h.memCacheHelper.Call(feature, method, params, labels...)
}

func TestPollInference_TimeoutCleanupPreservesReasonAndSuccessor(t *testing.T) {
	for _, lockOwner := range []string{"", "turn", "next-turn"} {
		for _, unlockFails := range []bool{false, true} {
			name := lockOwner
			if unlockFails {
				name += "/unavailable"
			}
			t.Run(name, func(t *testing.T) {
				helper := &responseDuringPoll{memCacheHelper: newMemCacheHelper(), turnOwner: lockOwner}
				if unlockFails {
					helper.unlockErr = errors.New("locker unavailable")
				}
				seedPollSession(t, helper.memCacheHelper, "turn", "thread", []AgentMessage{{Role: RoleUser, Content: "unfinished"}})
				ref := &ConvoSessionRef{SessionID: "turn", Status: ConvoSessionStatusActive}
				helper.cache[ConvoStateKeyPrefix+"thread"] = string(dipper.SerializeContent(&ConvoState{
					ConvoID: "thread", LastSession: ref, ActiveSession: ref, TTL: ConvoStreamTTL,
				}))
				store := NewAgentStore(helper, "").(*PersistentAgentStore)
				if lockOwner == "next-turn" {
					helper.onUnlock = func() {
						lockedConvoStateUpdate("thread", store, func(cs *ConvoState) {
							cs.registerSession("next-turn", "test_agent", AgentSessionTypeChatTurn, true)
							cs.LastTurnOwner = "next-turn"
						})
					}
				}
				store.PollInference(&dipper.Message{Labels: map[string]string{
					"agent_session_id": "turn", "resume_key": "workflow.1", "timeout": "20ms",
					"wait_for_completion": "true", "cancel_on_timeout": "true",
				}})
				emitted := helper.getEmitted()
				require.Len(t, emitted, 1)
				assert.Equal(t, "failure", emitted[0].Labels["status"])
				assert.Equal(t, "poll timeout after 20ms", emitted[0].Labels["reason"])
				var state ConvoState
				encoded, _ := helper.getCache(ConvoStateKeyPrefix + "thread")
				require.NoError(t, json.Unmarshal([]byte(encoded), &state))
				if lockOwner == "next-turn" {
					assert.Equal(t, ConvoSessionStatusActive, state.ActiveSession.Status)
					assert.Equal(t, "next-turn", helper.turnOwner)
					assert.Equal(t, []string{"turn"}, helper.turnUnlocks)
				} else {
					assert.Equal(t, ConvoSessionStatusCancelled, state.ActiveSession.Status)
					assert.Equal(t, []string{"turn"}, helper.turnUnlocks)
				}
			})
		}
	}
}

func TestReleaseTurnLock_CannotReleaseSuccessor(t *testing.T) {
	helper := &responseDuringPoll{memCacheHelper: newMemCacheHelper(), turnOwner: "next-turn"}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)
	session := &AgentSession{ID: "turn", TurnLockKey: ConvoTurnLockPrefix + "thread", store: store}
	session.releaseTurnLock()
	session.releaseTurnLock()
	assert.Equal(t, "next-turn", helper.turnOwner)
	assert.Equal(t, []string{"turn"}, helper.turnUnlocks)
}

func TestCancelConversations_RetriesOwnerAfterUnlockFailure(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		t.Run(fmt.Sprint(scoped), func(t *testing.T) {
			helper := &responseDuringPoll{memCacheHelper: newMemCacheHelper(), turnOwner: "turn"}
			seedPollSession(t, helper.memCacheHelper, "turn", "thread", nil)
			store := NewAgentStore(helper, "").(*PersistentAgentStore)
			sessionID := ""
			if scoped {
				sessionID = "turn"
			}
			helper.unlockErr = errors.New("transient locker failure")
			store.cancelConversations("thread", "", sessionID)
			assert.Equal(t, "turn", helper.turnOwner)
			helper.unlockErr = nil
			store.cancelConversations("thread", "", sessionID)
			assert.Empty(t, helper.turnOwner)
			// Retrying after a successor acquired the key must leave it intact.
			helper.turnOwner = "next-turn"
			store.cancelConversations("thread", "", sessionID)
			assert.Equal(t, "next-turn", helper.turnOwner)
		})
	}
}

func TestCancelConvo_ReleasesTerminalOwnerAfterPersistUnlockFailure(t *testing.T) {
	for _, status := range []string{ConvoSessionStatusComplete, ConvoSessionStatusFailed} {
		t.Run(status, func(t *testing.T) {
			helper := &responseDuringPoll{memCacheHelper: newMemCacheHelper(), turnOwner: "turn"}
			seedPollSession(t, helper.memCacheHelper, "turn", "thread", []AgentMessage{
				{Role: RoleAgent, Content: "final answer", IsComplete: true},
			})
			store := NewAgentStore(helper, "").(*PersistentAgentStore)
			session := &AgentSession{}
			session.setup(&dipper.Message{Labels: map[string]string{"agent_session_id": "turn"}}, store, false)
			session.loadConvoHistory()
			session.TurnLockKey = ConvoTurnLockPrefix + "thread"
			if status == ConvoSessionStatusFailed {
				session.ErrorReason = "driver failure"
			}
			helper.unlockErr = errors.New("transient terminal unlock failure")
			require.Panics(t, func() { session.persist(true) })
			before := &ConvoState{}
			before.load("thread", store)
			require.Equal(t, status, before.ActiveSession.Status)
			require.Equal(t, "turn", helper.turnOwner)
			helper.unlockErr = nil
			store.CancelConvo(&dipper.Message{Labels: map[string]string{"convo_id": "thread"}})
			assert.Empty(t, helper.turnOwner)
			after := &ConvoState{}
			after.load("thread", store)
			assert.Equal(t, before, after, "cleanup must preserve terminal status and failure evidence")
			// Retrying terminal cleanup cannot delete a successor's lock.
			helper.turnOwner = "next-turn"
			store.CancelConvo(&dipper.Message{Labels: map[string]string{"convo_id": "thread"}})
			assert.Equal(t, "next-turn", helper.turnOwner)
		})
	}
}

func TestCancelConvo_ReleasesRunningOwnerAfterQueuedTurnReplacesRefs(t *testing.T) {
	for _, scenario := range []string{"cleanup", "retry", "successor"} {
		t.Run(scenario, func(t *testing.T) {
			helper := &responseDuringPoll{memCacheHelper: newMemCacheHelper(), turnOwner: "running-turn"}
			store := NewAgentStore(helper, "").(*PersistentAgentStore)
			lockedConvoStateUpdate("thread", store, func(cs *ConvoState) {
				cs.TTL = ConvoStreamTTL
				cs.FirstSession = &ConvoSessionRef{SessionID: "first-completed", Status: ConvoSessionStatusComplete}
				cs.registerSession("running-turn", "test_agent", AgentSessionTypeChatTurn, true)
			})
			running := &AgentSession{ID: "running-turn", ConvoID: "thread", store: store}
			running.lockTurn(store, "thread")
			lockedConvoStateUpdate("thread", store, func(cs *ConvoState) {
				cs.registerSession("queued-turn", "test_agent", AgentSessionTypeChatTurn, true)
			})
			if scenario == "successor" {
				// A new turn may acquire the key after cancellation snapshots its
				// targets but before any of their owner-checked unlocks execute.
				helper.onTurnUnlock = func() {
					helper.turnOwner = "successor"
					lockedConvoStateUpdate("thread", store, func(cs *ConvoState) {
						cs.registerSession("successor", "test_agent", AgentSessionTypeChatTurn, true)
					})
					next := &AgentSession{ID: "successor", ConvoID: "thread", store: store}
					next.lockTurn(store, "thread")
				}
			}
			if scenario == "retry" {
				helper.unlockErr = errors.New("transient locker failure")
				store.CancelConvo(&dipper.Message{Labels: map[string]string{"convo_id": "thread"}})
				require.Equal(t, "running-turn", helper.turnOwner)
				helper.unlockErr = nil
			}
			store.CancelConvo(&dipper.Message{Labels: map[string]string{"convo_id": "thread"}})
			state := &ConvoState{}
			state.load("thread", store)
			assert.Equal(t, ConvoSessionStatusComplete, state.FirstSession.Status)
			assert.Contains(t, helper.turnUnlocks, "running-turn")
			if scenario == "successor" {
				assert.Equal(t, "successor", helper.turnOwner)
				assert.Equal(t, "successor", state.ActiveSession.SessionID)
				assert.Equal(t, ConvoSessionStatusActive, state.ActiveSession.Status)
			} else {
				assert.Empty(t, helper.turnOwner)
				assert.Equal(t, "queued-turn", state.ActiveSession.SessionID)
				assert.Equal(t, ConvoSessionStatusCancelled, state.ActiveSession.Status)
			}
		})
	}
}

func TestCancelledQueuedTurn_DoesNotRunAfterAcquiringLock(t *testing.T) {
	for _, entry := range []string{"StartInference", "runTurn"} {
		t.Run(entry, func(t *testing.T) {
			helper := &mockStoreHelper{mockStore: *newMockStore(newMemCacheHelper().cfg)}
			store := NewAgentStore(helper, "").(*PersistentAgentStore)
			helper.lockTriggers = map[string]func(){
				ConvoTurnLockPrefix + "thread": func() {
					store.CancelConvo(&dipper.Message{Labels: map[string]string{"convo_id": "thread"}})
				},
			}
			if entry == "StartInference" {
				store.StartInference(&dipper.Message{
					Labels:  map[string]string{"agent_name": "test_agent", "resume_key": "workflow.1"},
					Payload: map[string]interface{}{"convo_id": "thread", "text": "cancel this queued request"},
				})
			} else {
				store.runTurn("test_agent", "thread", "cancel this queued request", "", "", "")
			}
			assert.False(t, helper.hasCall("driver:openai:send_to_model"))
			state := &ConvoState{}
			state.load("thread", store)
			require.NotNil(t, state.ActiveSession)
			assert.Equal(t, "conversation cancelled", state.ActiveSession.ErrorReason)
			params := helper.getCallParams("locker:unlock")
			assert.Equal(t, ConvoTurnLockPrefix+"thread", params["name"])
			assert.Equal(t, state.ActiveSession.SessionID, params["owner"])
			session := &AgentSession{}
			session.load(state.ActiveSession.SessionID, store)
			assert.Equal(t, "conversation cancelled", session.ErrorReason)
		})
	}
}

func TestPollInference_TimeoutReleasesOwnerAfterQueuedTurnReplacesRefs(t *testing.T) {
	helper := &responseDuringPoll{memCacheHelper: newMemCacheHelper(), turnOwner: "turn"}
	seedPollSession(t, helper.memCacheHelper, "turn", "thread", []AgentMessage{{Role: RoleUser, Content: "unfinished"}})
	store := NewAgentStore(helper, "").(*PersistentAgentStore)
	lockedConvoStateUpdate("thread", store, func(cs *ConvoState) {
		cs.FirstSession = &ConvoSessionRef{SessionID: "first-completed", Status: ConvoSessionStatusComplete}
	})
	helper.onUnlock = func() {
		lockedConvoStateUpdate("thread", store, func(cs *ConvoState) {
			cs.registerSession("queued-turn", "test_agent", AgentSessionTypeChatTurn, true)
		})
	}
	store.PollInference(&dipper.Message{Labels: map[string]string{
		"agent_session_id": "turn", "resume_key": "workflow.1", "timeout": "20ms",
		"wait_for_completion": "true", "cancel_on_timeout": "true",
	}})
	emitted := helper.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "failure", emitted[0].Labels["status"])
	assert.Equal(t, "poll timeout after 20ms", emitted[0].Labels["reason"])
	assert.Empty(t, helper.turnOwner)
	state := &ConvoState{}
	state.load("thread", store)
	assert.Equal(t, "queued-turn", state.ActiveSession.SessionID)
	assert.Equal(t, ConvoSessionStatusActive, state.ActiveSession.Status)
}

func TestPollInference_ObservesFinalResponseAfterSlackDelivery(t *testing.T) {
	for _, content := range []string{"", "SLACK_RESPONSE_COMPLETE"} {
		for _, followup := range []bool{false, true} {
			name := content
			if followup {
				name += "/followup"
			}
			t.Run(name, func(t *testing.T) {
				helper := &responseDuringPoll{memCacheHelper: newMemCacheHelper()}
				seedPollSession(t, helper.memCacheHelper, "turn", "thread", []AgentMessage{
					{Role: RoleAgent, IsComplete: true, ToolCalls: []AgentToolCall{{FuncName: "wf__send_slack_message"}}},
					{Role: RoleToolResult, ToolResult: []map[string]interface{}{{"status": "success", "output": "PONG"}}},
				})
				ref := &ConvoSessionRef{SessionID: "turn", Status: ConvoSessionStatusActive}
				helper.cache[ConvoStateKeyPrefix+"thread"] = string(dipper.SerializeContent(&ConvoState{
					ConvoID: "thread", FirstSession: ref, LastSession: ref, ActiveSession: ref, TTL: ConvoStreamTTL,
				}))
				store := NewAgentStore(helper, "").(*PersistentAgentStore)
				var original AgentSession
				original.load("turn", store)
				original.TurnLockKey = ConvoTurnLockPrefix + "thread"
				helper.cache[AgentKeyPrefix+"turn"] = string(dipper.SerializeContent(&original))
				helper.turnOwner = "turn"
				helper.onUnlock = func() {
					store.ReceiveInference(&dipper.Message{
						Labels: map[string]string{"agent_session_id": "turn", "status": "success"},
						Payload: map[string]interface{}{"message": map[string]interface{}{
							"Role": RoleAgent, "content": content, "is_complete": true,
						}},
					})
					if followup {
						helper.turnOwner = "next-turn"
						lockedConvoStateUpdate("thread", store, func(cs *ConvoState) {
							cs.registerSession("next-turn", "test_agent", AgentSessionTypeChatTurn, true)
						})
						// The new turn must not become the old supervisor's response.
						s := &AgentSession{ConvoID: "thread", store: store}
						s.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "follow up"})
					}
				}
				store.PollInference(&dipper.Message{Labels: map[string]string{
					"agent_session_id": "turn", "resume_key": "workflow.1", "timeout": "1500ms",
					"wait_for_completion": "true", "cancel_on_timeout": "true",
				}})
				emitted := helper.getEmitted()
				require.Len(t, emitted, 1)
				assert.Equal(t, "success", emitted[0].Labels["status"])
				assert.Empty(t, emitted[0].Labels["reason"])
				payload, ok := emitted[0].Payload.(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, 3, payload["state"].(AgentState).HistoryLen)
				messages := payload["full_messages"].([]map[string]string)
				if content == "" {
					assert.Empty(t, messages)
				} else {
					require.Len(t, messages, 1)
					assert.Equal(t, content, messages[0]["content"])
				}
				if followup {
					assert.Equal(t, "next-turn", helper.turnOwner)
					state := &ConvoState{}
					state.load("thread", store)
					assert.Equal(t, "next-turn", state.ActiveSession.SessionID)
					assert.Equal(t, ConvoSessionStatusActive, state.ActiveSession.Status)
				} else {
					assert.Empty(t, helper.turnOwner)
				}
				assert.NotContains(t, helper.calls, "driver:openai:send_to_model")
				var persisted AgentSession
				encoded, _ := helper.getCache(AgentKeyPrefix + "turn")
				require.NoError(t, json.Unmarshal([]byte(encoded), &persisted))
				assert.Empty(t, persisted.ErrorReason)
			})
		}
	}
}

func TestPollInference_DriverFailureRemainsVisible(t *testing.T) {
	helper := newMemCacheHelper()
	seedPollSession(t, helper, "turn", "thread", []AgentMessage{{Role: RoleUser, Content: "unfinished"}})
	store := NewAgentStore(helper, "").(*PersistentAgentStore)
	store.ReceiveInference(&dipper.Message{
		Labels: map[string]string{"agent_session_id": "turn", "status": "error", "reason": "length_limit"},
		Payload: map[string]interface{}{"message": map[string]interface{}{
			"Role": RoleAgent, "is_complete": true,
		}},
	})
	lockedConvoStateUpdate("thread", store, func(cs *ConvoState) {
		cs.registerSession("next-turn", "test_agent", AgentSessionTypeChatTurn, true)
	})
	store.PollInference(&dipper.Message{Labels: map[string]string{
		"agent_session_id": "turn", "resume_key": "workflow.1", "timeout": "20ms", "wait_for_completion": "true",
	}})
	emitted := helper.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "error", emitted[0].Labels["status"])
	assert.Equal(t, "length_limit", emitted[0].Labels["reason"])
}
