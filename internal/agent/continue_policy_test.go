// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newContinuePolicySession builds a minimal chat-turn session for exercising
// sendToDriver's trailing-turn decision. The policy may be nil.
func newContinuePolicySession(t *testing.T, policy *agentpkg.ContinuePolicy) (*mockStore, *AgentSession) {
	t.Helper()
	agentA := config.Agent{
		Name:           "a",
		Driver:         "openai",
		Engine:         "gpt-4",
		SystemPrompt:   "sys prompt",
		ContinuePolicy: policy,
	}
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents:    map[string]config.Agent{"a": agentA},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}
	store := newMockStore(cfg)
	s := &AgentSession{
		store:      store,
		Agent:      &agentA,
		ID:         "cp-session",
		ConvoID:    "convo-cp",
		CurrentMsg: &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"text": "input"}},
		Type:       AgentSessionTypeChatTurn,
	}

	return store, s
}

// captureDriverHistory extracts the serialized history from the latest
// send_to_model call.
func captureDriverHistory(t *testing.T, store *mockStore) []AgentMessage {
	t.Helper()
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params, "sendToDriver must invoke the driver")
	history, ok := params["history"].([]AgentMessage)
	require.True(t, ok, "history must be a []AgentMessage")

	return history
}

func assertNoSynthetic(t *testing.T, history []AgentMessage, msg string) {
	t.Helper()
	for i, m := range history {
		assert.False(t, m.IsSynthetic, "%s: history[%d] role=%s must not be synthetic", msg, i, m.Role)
	}
}

// ---------------------------------------------------------------------------
// T1: decision matrix on the serialized history
// ---------------------------------------------------------------------------

func TestContinuePolicy_DecisionMatrix(t *testing.T) {
	type policyCase struct {
		name      string
		policy    *agentpkg.ContinuePolicy
		wantText  string
		expectRun bool
	}
	policies := []policyCase{
		{name: "nil", policy: nil, wantText: defaultContinuePrompt},
		{name: "auto", policy: &agentpkg.ContinuePolicy{Mode: "auto"}, wantText: defaultContinuePrompt},
		{name: "never", policy: &agentpkg.ContinuePolicy{Mode: "never"}, wantText: defaultContinuePrompt},
		{name: "auto_custom", policy: &agentpkg.ContinuePolicy{Mode: "auto", Prompt: "  Custom resume  "}, wantText: "Custom resume"},
		{name: "auto_whitespace", policy: &agentpkg.ContinuePolicy{Mode: "auto", Prompt: "   "}, wantText: defaultContinuePrompt},
	}

	tails := []struct {
		name      string
		history   []AgentMessage
		synthable bool
	}{
		{
			name:      "user",
			history:   []AgentMessage{{Role: RoleUser, Content: "hello"}},
			synthable: false,
		},
		{
			name: "tool_result",
			history: []AgentMessage{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleAgent, ToolCalls: []AgentToolCall{{FuncName: "x"}}},
				{Role: RoleToolResult, ToolResult: []map[string]interface{}{{"status": "success"}}},
			},
			synthable: false,
		},
		{
			name: "agent_with_toolcalls",
			history: []AgentMessage{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleAgent, ToolCalls: []AgentToolCall{{FuncName: "x"}}},
			},
			synthable: false,
		},
		{
			name: "agent_without_toolcalls",
			history: []AgentMessage{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleAgent, Content: "partial", IsComplete: false},
			},
			synthable: true,
		},
		{
			name: "tool_role",
			history: []AgentMessage{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleTool, Content: "out"},
			},
			synthable: false,
		},
		{
			name: "unknown_role",
			history: []AgentMessage{
				{Role: RoleUser, Content: "hello"},
				{Role: "mystery", Content: "?"},
			},
			synthable: false,
		},
		{
			name:      "empty",
			history:   nil,
			synthable: false,
		},
		{
			name:      "system_only",
			history:   []AgentMessage{{Role: RoleSystem, Content: "legacy"}},
			synthable: false,
		},
	}

	for _, tc := range tails {
		tc := tc
		for _, pc := range policies {
			pc := pc
			t.Run(tc.name+"/"+pc.name, func(t *testing.T) {
				store, s := newContinuePolicySession(t, pc.policy)
				s.history = tc.history

				s.sendToDriver()
				history := captureDriverHistory(t, store)

				if !tc.synthable {
					assertNoSynthetic(t, history, tc.name)
					return
				}

				last := history[len(history)-1]
				require.Equal(t, RoleUser, last.Role, "expected a synthesized trailing user turn")
				assert.True(t, last.IsSynthetic, "synthesized turn must be provenance-marked")
				assert.Equal(t, pc.wantText, last.Content)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// T2: multi-round tool turn produces zero synthetic messages
// ---------------------------------------------------------------------------

func TestContinuePolicy_MultiRoundToolTurn_NeverSynthesizes(t *testing.T) {
	store, s := newContinuePolicySession(t, nil)
	s.history = []AgentMessage{{Role: RoleUser, Content: "go"}}

	const rounds = 3
	for round := 0; round < rounds; round++ {
		s.history = append(s.history, AgentMessage{
			Role:      RoleAgent,
			ToolCalls: []AgentToolCall{{FuncName: "mcp__srv__tool"}},
		})
		s.ToolCalls = []AgentToolCall{{FuncName: "mcp__srv__tool"}}
		s.CurrentCall = 0

		msg := &dipper.Message{
			Labels: map[string]string{
				"turn_id":      strconv.Itoa(len(s.history)),
				"tool_call_id": "0",
				"status":       "success",
			},
			Payload: map[string]interface{}{
				"data": map[string]interface{}{"output": "tool-out"},
			},
		}
		s.processToolResult(msg)

		history := captureDriverHistory(t, store)
		assertNoSynthetic(t, history, "tool round")
		require.Equal(t, RoleToolResult, history[len(history)-1].Role,
			"round %d: the driver request must end on the tool result", round)
	}
}

// ---------------------------------------------------------------------------
// T3: filteredTail and lastNonSlashMsg cannot drift
// ---------------------------------------------------------------------------

func TestFilteredTail_MatchesLastNonSlashMsg(t *testing.T) {
	cases := map[string][]AgentMessage{
		"plain": {
			{Role: RoleUser, Content: "hello"},
			{Role: RoleAgent, Content: "done", IsComplete: true},
		},
		"slash_command_tail": {
			{Role: RoleUser, Content: "hello"},
			{Role: RoleUser, Content: "/retry", IsSlash: true},
		},
		"slash_reply_only": {
			{Role: RoleAgent, Content: "reply", IsComplete: true, IsSlash: true},
		},
		"slash_reply_after_real_tail": {
			{Role: RoleUser, Content: "hello"},
			{Role: RoleAgent, Content: "done", IsComplete: true},
			{Role: RoleAgent, Content: "reply", IsComplete: true, IsSlash: true},
		},
		"all_slash": {
			{Role: RoleUser, Content: "/help", IsSlash: true},
		},
		"empty": {},
	}

	for name, history := range cases {
		history := history
		t.Run(name, func(t *testing.T) {
			s := &AgentSession{history: history}
			got := s.lastNonSlashMsg()
			tail, ok := filteredTail(history)
			if got == nil {
				assert.False(t, ok, "filteredTail must report no tail when lastNonSlashMsg is nil")
				return
			}
			require.True(t, ok)
			assert.Equal(t, got.Role, tail.Role)
			assert.Equal(t, got.Content, tail.Content)
			assert.Equal(t, got.IsSlash, tail.IsSlash)
		})
	}
}

// ---------------------------------------------------------------------------
// T4: synthesis is never persisted
// ---------------------------------------------------------------------------

func TestContinuePolicy_SynthesisNotPersisted(t *testing.T) {
	store, s := newContinuePolicySession(t, nil)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAgent, Content: "partial", IsComplete: false},
	}
	before := append([]AgentMessage(nil), s.history...)

	s.sendToDriver()

	history := captureDriverHistory(t, store)
	require.True(t, history[len(history)-1].IsSynthetic, "synthesis must fire for an interrupted agent turn")

	for _, c := range store.getCalls() {
		assert.NotEqual(t, "cache:rpush", c, "synthesis must never rpush to convo_history")
	}
	store.mu.Lock()
	_, hasList := store.lists[ConvoHistoryKeyPrefix+s.ConvoID]
	store.mu.Unlock()
	assert.False(t, hasList, "convo_history must not contain the synthetic turn")

	assert.Equal(t, before, s.history, "in-memory history must be unchanged by synthesis")
	assert.False(t, s.history[len(s.history)-1].IsSynthetic)
}

// ---------------------------------------------------------------------------
// T5: synthesis does not affect token accounting
// ---------------------------------------------------------------------------

func TestContinuePolicy_SynthesisDoesNotChangeContextTokens(t *testing.T) {
	agentA := config.Agent{
		Name:         "a",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "sys prompt",
		TokenCounter: "simple",
	}
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents:    map[string]config.Agent{"a": agentA},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}
	store := newMockStore(cfg)
	s := &AgentSession{
		store:        store,
		Agent:        &agentA,
		ID:           "cp-token",
		ConvoID:      "convo-cp-token",
		CurrentMsg:   &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"text": "input"}},
		Type:         AgentSessionTypeChatTurn,
		TokenCounter: &SimpleTokenCounter{},
	}
	(&ConvoState{ConvoID: s.ConvoID, TTL: "72h"}).persist(store)

	s.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "hello"})
	s.appendConvoHistory(&AgentMessage{Role: RoleAgent, Content: "partial", IsComplete: false})
	before := s.getConvoContextTokens()
	require.Greater(t, before, 0)

	s.sendToDriver()
	history := captureDriverHistory(t, store)
	assert.True(t, history[len(history)-1].IsSynthetic, "expected synthesis for the interrupted turn")
	assert.Equal(t, before, s.getConvoContextTokens(), "synthesis must not change ContextTokens")
}

// ---------------------------------------------------------------------------
// T6: recover() yields the same matrix outcome as the live path
// ---------------------------------------------------------------------------

func TestContinuePolicy_Recover_MatchesLivePath(t *testing.T) {
	cases := []struct {
		name      string
		history   []AgentMessage
		synthable bool
	}{
		{
			name: "agent_interrupted",
			history: []AgentMessage{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleAgent, Content: "partial", IsComplete: false},
			},
			synthable: true,
		},
		{
			name: "tool_result",
			history: []AgentMessage{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleAgent, ToolCalls: []AgentToolCall{{FuncName: "x"}}},
				{Role: RoleToolResult, ToolResult: []map[string]interface{}{{"status": "success"}}},
			},
			synthable: false,
		},
		{
			name: "legacy_tool",
			history: []AgentMessage{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleTool, Content: "out"},
			},
			synthable: false,
		},
		{
			name:      "user",
			history:   []AgentMessage{{Role: RoleUser, Content: "hello"}},
			synthable: false,
		},
		{
			name:      "empty",
			history:   nil,
			synthable: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store, s := newContinuePolicySession(t, nil)
			s.history = tc.history

			s.recover()

			history := captureDriverHistory(t, store)
			if tc.synthable {
				last := history[len(history)-1]
				require.Equal(t, RoleUser, last.Role)
				assert.True(t, last.IsSynthetic)
				return
			}
			assertNoSynthetic(t, history, tc.name)
		})
	}
}

// ---------------------------------------------------------------------------
// T7: compaction resume paths
// ---------------------------------------------------------------------------

func TestContinuePolicy_CompactionResume(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store, s := makeCompactionResultSession(t, false)
		call := AgentToolCall{
			FuncName: "ag__summ",
			Params: map[string]interface{}{
				"compaction_id": "convo-2_g1",
				"preserve":      2,
			},
		}
		got := s.handleCompactionResult(call, []map[string]interface{}{{"status": "success", "data": "COMPACTED SUMMARY"}})
		require.True(t, got)

		history := captureDriverHistory(t, store)
		assertNoSynthetic(t, history, "compaction success")
		assert.Equal(t, RoleUser, history[len(history)-1].Role,
			"compaction resumes with the pending user message as the tail")
	})

	t.Run("failure", func(t *testing.T) {
		store, s := makeCompactionFailureSession(t, false)
		got := s.handleCompactionResult(compactionCall(), []map[string]interface{}{{"status": "success", "data": ""}})
		require.True(t, got)

		// This is the one sendToDriver call site that legitimately yields a
		// trailing RoleSystem message: compactionFailed injects the failure
		// placeholder before resuming. It must stay a logged no-op — a trailing
		// system notice is never an injection point.
		history := captureDriverHistory(t, store)
		assertNoSynthetic(t, history, "compaction failure")
		assert.Equal(t, RoleSystem, history[len(history)-1].Role,
			"compaction failure resumes on the RoleSystem placeholder")
	})
}

// ---------------------------------------------------------------------------
// T8: /retry keeps working end to end
// ---------------------------------------------------------------------------

func TestContinuePolicy_SlashRetry(t *testing.T) {
	t.Run("interrupted_agent_synthesizes_marked", func(t *testing.T) {
		store, s := newChatSlashStore(t, nil)
		s.history = []AgentMessage{
			{Role: RoleUser, Content: "hello"},
			{Role: RoleAgent, Content: "partial", IsComplete: false},
		}
		setChatText(s, AgentSessionTypeChatTurn, "/retry")

		s.run()

		history := captureDriverHistory(t, store)
		last := history[len(history)-1]
		require.Equal(t, RoleUser, last.Role)
		assert.True(t, last.IsSynthetic, "interrupted agent turn must resume with a marked marker")
		assert.Equal(t, defaultContinuePrompt, last.Content)
	})

	t.Run("interrupted_tool_call_no_synthesis", func(t *testing.T) {
		store, s := newChatSlashStore(t, nil)
		s.history = []AgentMessage{
			{Role: RoleUser, Content: "hello"},
			{Role: RoleAgent, ToolCalls: []AgentToolCall{{FuncName: "mcp__srv__tool"}}},
		}
		setChatText(s, AgentSessionTypeChatTurn, "/retry")

		s.run()

		history := captureDriverHistory(t, store)
		assertNoSynthetic(t, history, "/retry tool path")
		require.Equal(t, RoleToolResult, history[len(history)-1].Role)
	})

	t.Run("empty_history_replies", func(t *testing.T) {
		store, s := newChatSlashStore(t, nil)
		s.history = nil
		setChatText(s, AgentSessionTypeChatTurn, "/retry")

		s.run()

		assert.False(t, store.hasCall("driver:openai:send_to_model"), "empty history must not invoke the model")
		reply := s.history[len(s.history)-1]
		assert.Contains(t, reply.Content, "Nothing to retry")
	})
}

// ---------------------------------------------------------------------------
// Entry-point coverage: run() always appends a real user turn before sending
// ---------------------------------------------------------------------------

func TestContinuePolicy_RunEntryPoint_TailIsUser(t *testing.T) {
	store, s := newContinuePolicySession(t, nil)
	s.history = nil
	text := "real user message"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"text": text}}

	s.run()

	history := captureDriverHistory(t, store)
	last := history[len(history)-1]
	require.Equal(t, RoleUser, last.Role, "run() must send a trailing user turn")
	assert.Equal(t, text, last.Content)
	assert.False(t, last.IsSynthetic, "a real user turn must not be marked synthetic")
	assertNoSynthetic(t, history, "run()")
}

// ---------------------------------------------------------------------------
// T9: OpenAI-shaped tool round-trip ends on a tool result (no live driver)
// ---------------------------------------------------------------------------

// TestContinuePolicy_OpenAIToolRoundTrip_EndsOnToolResult asserts the request
// shape hd-driver-openai relies on: after a tool round-trip the serialized
// history ends on exactly one RoleToolResult message (mapped by the driver to a
// canonical terminal `tool` message) with no synthetic user turn. This is the
// unit-level stand-in for an OpenAI smoke test (no live driver in CI).
func TestContinuePolicy_OpenAIToolRoundTrip_EndsOnToolResult(t *testing.T) {
	store, s := newContinuePolicySession(t, nil)
	s.history = []AgentMessage{{Role: RoleUser, Content: "please inspect"}}
	s.history = append(s.history, AgentMessage{
		Role:      RoleAgent,
		ToolCalls: []AgentToolCall{{FuncName: "mcp__srv__tool"}},
	})
	s.ToolCalls = []AgentToolCall{{FuncName: "mcp__srv__tool"}}
	s.CurrentCall = 0

	s.processToolResult(&dipper.Message{
		Labels: map[string]string{
			"turn_id":      strconv.Itoa(len(s.history)),
			"tool_call_id": "0",
			"status":       "success",
		},
		Payload: map[string]interface{}{"data": map[string]interface{}{"output": "tool-out"}},
	})

	history := captureDriverHistory(t, store)
	last := history[len(history)-1]
	require.Equal(t, RoleToolResult, last.Role)
	require.Len(t, last.ToolResult, 1)
	assert.Equal(t, "tool-out", last.ToolResult[0]["data"])
	assertNoSynthetic(t, history, "openai tool round-trip")
}

// ---------------------------------------------------------------------------
// T10: IsSynthetic JSON round-trip / omission
// ---------------------------------------------------------------------------

func TestMessage_IsSyntheticJSONRoundTrip(t *testing.T) {
	marked := AgentMessage{Role: RoleUser, Content: "resume", IsSynthetic: true}
	encoded, err := json.Marshal(marked)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"is_synthetic":true`)

	var decoded AgentMessage
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.True(t, decoded.IsSynthetic)
	assert.Equal(t, "resume", decoded.Content)

	plain := AgentMessage{Role: RoleUser, Content: "real"}
	encodedPlain, err := json.Marshal(plain)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedPlain), "is_synthetic")
}
