// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration

package agent

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Model the Redis list trimming that the general cache helper leaves as a no-op.
type trimmingPollHistory struct{ *memCacheHelper }

func (h *trimmingPollHistory) Call(feature, method string, params interface{}, labels ...string) ([]byte, error) {
	if feature != "cache" || method != "ltrim" {
		return h.memCacheHelper.Call(feature, method, params, labels...)
	}
	p := params.(map[string]interface{})
	key := p["key"].(string)
	h.mu.Lock()
	defer h.mu.Unlock()
	var history []AgentMessage
	if err := json.Unmarshal([]byte(h.cache[key]), &history); err != nil {
		return nil, fmt.Errorf("decode cached poll history: %w", err)
	}
	start := p["start"].(int)
	if start < 0 {
		start = max(0, len(history)+start)
	}
	h.cache[key] = string(dipper.SerializeContent(history[start:]))

	return []byte("OK"), nil
}

func TestPollInference_CompletedResponseSurvivesHistoryMutation(t *testing.T) {
	for _, mutation := range []string{"trim", "compact", "reset"} {
		for _, content := range []string{"original final answer", ""} {
			for _, completionOnly := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/completion=%t", mutation, content, completionOnly), func(t *testing.T) {
					helper := &trimmingPollHistory{newMemCacheHelper()}
					seedPollSession(t, helper.memCacheHelper, "turn", "thread", []AgentMessage{
						{Role: RoleUser, Content: "earlier question"},
						{Role: RoleAgent, Content: "earlier answer", IsComplete: true},
						{Role: RoleUser, Content: "current question"},
					})
					store := NewAgentStore(helper, "").(*PersistentAgentStore)
					original := &AgentSession{}
					original.setup(&dipper.Message{Labels: map[string]string{"agent_session_id": "turn"}}, store, false)
					original.Agent.MaxHistoryLen = 4
					original.loadConvoHistory()
					original.LastPoll = 3
					original.appendConvoHistory(&AgentMessage{Role: RoleAgent, Content: content, IsComplete: true})
					original.persist(true)
					lockedConvoStateUpdate("thread", store, func(cs *ConvoState) {
						cs.registerSession("next-turn", "test_agent", AgentSessionTypeChatTurn, true)
					})
					next := &AgentSession{ID: "next-turn", ConvoID: "thread", Agent: original.Agent, store: store}
					switch mutation {
					case "compact":
						require.NoError(t, next.replaceLiveHistory([]AgentMessage{{Role: RoleSystem, Content: "new summary"}}))
					case "reset":
						require.NoError(t, next.replaceLiveHistory(nil))
					}
					next.loadConvoHistory()
					next.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "follow up"})
					next.appendConvoHistory(&AgentMessage{Role: RoleAgent, Content: "SUCCESSOR ANSWER", IsComplete: true})
					store.PollInference(&dipper.Message{Labels: map[string]string{
						"agent_session_id": "turn", "resume_key": "workflow.1", "timeout": "20ms",
						"wait_for_completion": fmt.Sprint(completionOnly),
					}})
					emitted := helper.getEmitted()
					require.Len(t, emitted, 1)
					if content == "" && !completionOnly {
						assert.Equal(t, "failure", emitted[0].Labels["status"])
						assert.Equal(t, "poll timeout after 20ms", emitted[0].Labels["reason"])
					} else {
						assert.Equal(t, "success", emitted[0].Labels["status"])
						payload := emitted[0].Payload.(map[string]interface{})
						assert.Equal(t, 4, payload["state"].(AgentState).HistoryLen)
						messages := payload["full_messages"].([]map[string]string)
						if content == "" {
							assert.Empty(t, messages)
						} else {
							require.Len(t, messages, 1)
							assert.Equal(t, content, messages[0]["content"])
							// An ordinary poll must not replay the consumed response.
							store.PollInference(&dipper.Message{Labels: map[string]string{
								"agent_session_id": "turn", "resume_key": "workflow.2", "timeout": "20ms",
							}})
							emitted = helper.getEmitted()
							require.Len(t, emitted, 2)
							assert.Equal(t, "failure", emitted[1].Labels["status"])
							assert.Equal(t, "poll timeout after 20ms", emitted[1].Labels["reason"])
						}
					}
				})
			}
		}
	}
}
