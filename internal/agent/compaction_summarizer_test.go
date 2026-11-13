// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// makeSummarizerBoundarySession builds a session with a compaction policy that
// dispatches to the "summ" summarization agent. The last non-slash user message
// ("m3", index 3) is the triggering user message that should be excluded from
// the summarizer's input via summarize_upto. When customPrompt is true the
// policy carries a custom SummarizationPrompt.
func makeSummarizerBoundarySession(t *testing.T, customPrompt bool) (*mockStore, *AgentSession) {
	t.Helper()
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"summ": {Name: "summ", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})
	cp := &agentpkg.CompactionPolicy{
		Strategy:           agentpkg.CompactionStrategySummarize,
		ThresholdType:      "history_len",
		Threshold:          3,
		PreserveRecent:     2,
		SummarizationAgent: "summ",
	}
	if customPrompt {
		cp.SummarizationPrompt = "CUSTOM PROMPT: condense the conversation."
	}
	agentA := config.Agent{Name: "bot", Driver: "openai", Engine: "gpt-4", CompactionPolicy: cp}
	s := &AgentSession{store: store, Agent: &agentA, ID: "s1", ConvoID: "convo-1"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "m3"}, // triggering user message at index 3
	}

	return store, s
}

// ---------------------------------------------------------------------------
// structural: summarizer input excludes the triggering user message + reminder
// ---------------------------------------------------------------------------

// TestCompactHistory_PassesSummarizeUpto_DefaultPrompt verifies compactHistory
// passes a summarize_upto boundary equal to the index of the triggering user
// message (so the summarizer's loaded history excludes it) and that the default
// prompt carries the always-injected "summarize, do not answer" reminder.
func TestCompactHistory_PassesSummarizeUpto_DefaultPrompt(t *testing.T) {
	store, s := makeSummarizerBoundarySession(t, false)
	seedMockHistory(store, s.ConvoID, s.history)

	ok := s.compactHistory()
	require.True(t, ok)

	require.Len(t, s.ToolCalls, 1)
	tc := s.ToolCalls[0]
	require.Equal(t, "ag__summ", tc.FuncName)

	// summarize_upto excludes the triggering user message (index 3).
	v, ok := dipper.GetMapDataInt(tc.Params, "summarize_upto")
	require.True(t, ok, "summarize_upto param must be present")
	assert.Equal(t, 3, v, "summarize_upto must exclude the triggering user message")

	// Reminder is always present in the default prompt input.
	input, ok := dipper.GetMapDataStr(tc.Params, "input")
	require.True(t, ok)
	assert.Contains(t, input, compactionSummarizeReminder)
	assert.Contains(t, input, "Summarize the above conversation history")
}

// TestCompactHistory_InjectReminder_CustomPrompt verifies the reminder is also
// injected when a custom SummarizationPrompt is configured.
func TestCompactHistory_InjectReminder_CustomPrompt(t *testing.T) {
	store, s := makeSummarizerBoundarySession(t, true)
	seedMockHistory(store, s.ConvoID, s.history)

	ok := s.compactHistory()
	require.True(t, ok)
	require.Len(t, s.ToolCalls, 1)

	input, ok := dipper.GetMapDataStr(s.ToolCalls[0].Params, "input")
	require.True(t, ok)
	assert.Contains(t, input, compactionSummarizeReminder, "reminder must be present with a custom prompt")
	assert.Contains(t, input, "CUSTOM PROMPT", "custom prompt must be preserved")
}

// TestCompactHistory_NonCompactionCallsUnaffected verifies the summarize_upto
// gate: a sub-agent tool call that does not carry summarize_upto (a normal
// ag__ call) is dispatched without the param, so non-compaction calls are
// completely unaffected.
func TestCompactHistory_NonCompactionCallsUnaffected(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["summ"] = config.Agent{Name: "summ", Driver: "openai", Engine: "gpt-4"}
	s := &AgentSession{store: store, Agent: &config.Agent{Name: "bot"}, ID: "s1", ConvoID: "convo-1"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}

	// A normal ag__ tool call without compaction params.
	tc := AgentToolCall{FuncName: "ag__summ", Params: map[string]interface{}{"input": "hello"}}
	s.handleAgentToolCall(tc, "convo-1")

	emitted := store.getEmitted()
	require.GreaterOrEqual(t, len(emitted), 1)
	var payload map[string]interface{}
	for _, m := range emitted {
		if m.Subject == "agent_call" {
			payload = m.Payload.(map[string]interface{})

			break
		}
	}
	require.NotNil(t, payload)
	_, present := payload["summarize_upto"]
	assert.False(t, present, "non-compaction ag__ calls must NOT carry summarize_upto")
}

// ---------------------------------------------------------------------------
// structural: archived _gN history intact (still contains the user message)
// ---------------------------------------------------------------------------

// TestCompactHistory_ArchiveIntact_ContainsTriggeringUserMessage verifies the
// full _gN archive is kept intact and still contains the triggering user
// message: only the summarizer's loaded input excludes it, never the archive.
func TestCompactHistory_ArchiveIntact_ContainsTriggeringUserMessage(t *testing.T) {
	store, s := makeSummarizerBoundarySession(t, false)
	seedMockHistory(store, s.ConvoID, s.history)

	ok := s.compactHistory()
	require.True(t, ok)

	// compactHistory archived the live history under convo-1_g1 before appending
	// the compaction tool-call card, so the archive holds the full pre-compaction
	// history including the triggering user message ("m3").
	archived := persistedLiveHistory(t, store, "convo-1_g1")
	require.Len(t, archived, 4, "archive must hold the full pre-compaction history")
	contents := make([]string, 0, len(archived))
	for _, m := range archived {
		contents = append(contents, m.Content)
	}
	assert.Contains(t, contents, "m3", "archived _gN history must still contain the triggering user message")
	assert.Contains(t, contents, "m1")
	assert.Contains(t, contents, "a1")
}

// ---------------------------------------------------------------------------
// structural: summarizer loadConvoHistory honors the boundary (gated on presence)
// ---------------------------------------------------------------------------

// TestLoadConvoHistory_HonorsSummarizeUpto verifies the summarizer's
// loadConvoHistory truncates the loaded (archived) history at the summarize_upto
// boundary, excluding the triggering user message.
func TestLoadConvoHistory_HonorsSummarizeUpto(t *testing.T) {
	store := newMockStore(nil)
	s := &AgentSession{store: store, ConvoID: "convo-1"}
	history := []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "m3"},
	}
	seedMockHistory(store, s.ConvoID, history)

	s.SummarizeUpto = 3
	s.loadConvoHistory()
	require.Len(t, s.history, 3)
	assert.Equal(t, "m1", s.history[0].Content)
	assert.Equal(t, "a1", s.history[1].Content)
	assert.Equal(t, "m2", s.history[2].Content)
}

// TestLoadConvoHistory_NoBoundary_LoadsFullHistory verifies that without a
// summarize_upto boundary (0), loadConvoHistory loads the full history so
// non-compaction ag__ sub-agent calls are unaffected.
func TestLoadConvoHistory_NoBoundary_LoadsFullHistory(t *testing.T) {
	store := newMockStore(nil)
	s := &AgentSession{store: store, ConvoID: "convo-1"}
	history := []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "m3"},
	}
	seedMockHistory(store, s.ConvoID, history)

	s.SummarizeUpto = 0 // not set
	s.loadConvoHistory()
	require.Len(t, s.history, 4, "non-compaction calls must load the full history")
}

// ---------------------------------------------------------------------------
// structural: compaction boundary bookkeeping unchanged by the exclusion
// ---------------------------------------------------------------------------

// TestCompactHistory_DoesNotShiftBoundaryBookkeeping verifies the summarize_upto
// exclusion (which only affects the summarizer's loaded history) does not shift
// the parent session's CompactionHistoryIdx or PrevContextSize.
func TestCompactHistory_DoesNotShiftBoundaryBookkeeping(t *testing.T) {
	store, s := makeSummarizerBoundarySession(t, false)
	seedMockHistory(store, s.ConvoID, s.history)
	// Simulate a prior compaction boundary and baseline.
	s.CompactionHistoryIdx = 2
	s.PrevContextSize = 100

	ok := s.compactHistory()
	require.True(t, ok)

	assert.Equal(t, 2, s.CompactionHistoryIdx, "compaction boundary must not shift due to the exclusion")
	assert.Equal(t, 100, s.PrevContextSize, "compaction baseline must not shift due to the exclusion")
}

// TestCompactHistory_RefreshContextSizeConsistent verifies refreshContextSize
// still derives the baseline from the parent history after compaction dispatches
// (the exclusion does not disturb the boundary-based scan).
func TestCompactHistory_RefreshContextSizeConsistent(t *testing.T) {
	store, s := makeSummarizerBoundarySession(t, false)
	seedMockHistory(store, s.ConvoID, s.history)
	// Seed a baseline from the complete agent message at index 1.
	s.CompactionHistoryIdx = 0
	s.history[1].InputTokens = 100
	s.history[1].OutputTokens = 50

	ok := s.compactHistory()
	require.True(t, ok)

	assert.True(t, s.refreshContextSize())
	assert.Equal(t, 150, s.PrevContextSize, "baseline must be derived from the latest complete agent message")
}

// ---------------------------------------------------------------------------
// best-effort adherence check (mock-based; overlaps Phase A)
// ---------------------------------------------------------------------------

// TestCompaction_AnswerShapedSummary_PreservesHistory is a mock-based best-effort
// adherence check: even when the summarizer returns an answer-shaped string
// (as if it answered the user's question instead of summarizing), compaction
// still completes and the preserved tail (containing the triggering user
// message) remains intact. Real-model summarization behavior is not unit-testable;
// this only verifies the pipeline does not lose history in that scenario.
func TestCompaction_AnswerShapedSummary_PreservesHistory(t *testing.T) {
	store, s := makeSummarizerBoundarySession(t, false)
	seedMockHistory(store, s.ConvoID, s.history)

	// Dispatch compaction the way sendToDriver does.
	ok := s.compactHistory()
	require.True(t, ok)

	// Capture the dispatched compaction tool call.
	require.Len(t, s.ToolCalls, 1)
	call := s.ToolCalls[0]

	// Simulate the summarizer returning an answer-shaped string (non-empty,
	// success) — the model ignored the reminder and answered instead. The
	// validation layer accepts it as a genuine summary (non-empty), so
	// compaction completes gracefully without truncating live history.
	got := s.handleCompactionResult(call, []map[string]interface{}{
		{"status": "success", "data": "The answer to your question is 42."},
	})
	assert.True(t, got, "answer-shaped summary must still complete compaction gracefully")

	// The compacted history begins with the summary system message and the
	// preserved tail is intact (contains the triggering user message "m3").
	require.GreaterOrEqual(t, len(s.history), 1)
	assert.Equal(t, RoleSystem, s.history[0].Role)
	assert.Contains(t, s.history[0].Content, "The answer to your question is 42.")
	tailContents := make([]string, 0, len(s.history))
	for _, m := range s.history[1:] {
		tailContents = append(tailContents, m.Content)
	}
	assert.Contains(t, tailContents, "m3", "preserved tail must keep the triggering user message")

	// The full _gN archive remains intact as a durable safety net.
	archived := persistedLiveHistory(t, store, "convo-1_g1")
	contents := make([]string, 0, len(archived))
	for _, m := range archived {
		contents = append(contents, m.Content)
	}
	assert.Contains(t, contents, "m3", "archive must remain intact even with an answer-shaped summary")
}
