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

// makeTotalTokensSession builds a session with a total_tokens compaction policy
// whose threshold is `threshold`. The mock store always exposes a "summ"
// summarization agent so compactHistory() can dispatch when needed.
func makeTotalTokensSession(threshold int) *AgentSession {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"summ": {Name: "summ", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})
	agentA := config.Agent{
		Name:   "bot",
		Driver: "openai",
		Engine: "gpt-4",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy:           agentpkg.CompactionStrategySummarize,
			Threshold:          threshold,
			ThresholdType:      "total_tokens",
			PreserveRecent:     1,
			SummarizationAgent: "summ",
		},
	}

	return &AgentSession{
		store: store, Agent: &agentA, ID: "s1", ConvoID: "convo-1",
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"convo_id": "convo-1"},
		},
	}
}

// seedMockHistory writes serialized messages into the mock store's history list
// so cache:lrange returns them the way the real Redis-backed store does.
func seedMockHistory(m *mockStore, convoID string, history []AgentMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lists == nil {
		m.lists = map[string][]string{}
	}
	key := ConvoHistoryKeyPrefix + convoID
	for _, msg := range history {
		m.lists[key] = append(m.lists[key], string(dipper.SerializeContent(msg)))
	}
}

// ---------------------------------------------------------------------------
// refreshContextSize – canonical metric from persisted history
// ---------------------------------------------------------------------------

// TestRefreshContextSize_BackfillFromHistory verifies that an existing long
// conversation with a zero baseline derives the correct value from history on
// upgrade (no migration/backfill pass needed), and that the baseline is the
// LATEST complete agent message's tokens, not the accumulated sum.
func TestRefreshContextSize_BackfillFromHistory(t *testing.T) {
	s := makeTotalTokensSession(1000)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 100, OutputTokens: 50},
		{Role: RoleUser, Content: "q2"},
		{Role: RoleAgent, Content: "a2", IsComplete: true, InputTokens: 200, OutputTokens: 80},
		{Role: RoleUser, Content: "q3"},
	}
	s.PrevContextSize = 0 // a freshly restored/upgraded session has zero baseline

	ok := s.refreshContextSize()
	assert.True(t, ok)
	// Latest call (a2): 200+80=280, NOT the accumulated 100+50+200+80=430.
	assert.Equal(t, 280, s.PrevContextSize)
}

// TestRefreshContextSize_SkipsZeroTokenAgentMessages verifies the scan helper
// skips agent messages that carry no driver-reported input tokens.
func TestRefreshContextSize_SkipsZeroTokenAgentMessages(t *testing.T) {
	s := makeTotalTokensSession(1000)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a-zero-1", IsComplete: true}, // no tokens at all
		{Role: RoleUser, Content: "q2"},
		{Role: RoleAgent, Content: "a-zero-2", IsComplete: true, InputTokens: 0, OutputTokens: 0},
		{Role: RoleUser, Content: "q3"},
		{Role: RoleAgent, Content: "a-with-tokens", IsComplete: true, InputTokens: 300, OutputTokens: 100},
		{Role: RoleUser, Content: "q4"},
	}
	assert.True(t, s.refreshContextSize())
	assert.Equal(t, 400, s.PrevContextSize)

	// When every agent message has zero tokens there is no baseline at all.
	s2 := makeTotalTokensSession(1000)
	s2.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a", IsComplete: true, InputTokens: 0, OutputTokens: 0},
		{Role: RoleUser, Content: "q2"},
	}
	assert.False(t, s2.refreshContextSize())
	assert.Equal(t, 0, s2.PrevContextSize)
}

// TestRefreshContextSize_CompleteDefinition verifies "complete" is precisely
// RoleAgent && IsComplete && !IsChunk && !IsSlash: incomplete, chunk, slash and
// non-agent messages are all skipped.
func TestRefreshContextSize_CompleteDefinition(t *testing.T) {
	s := makeTotalTokensSession(1000)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a-incomplete", IsComplete: false, InputTokens: 500, OutputTokens: 100},
		{Role: RoleUser, Content: "q2"},
		{Role: RoleAgent, Content: "a-chunk", IsComplete: true, IsChunk: true, InputTokens: 500, OutputTokens: 100},
		{Role: RoleUser, Content: "q3"},
		{Role: RoleAgent, Content: "a-slash", IsComplete: true, IsSlash: true, InputTokens: 500, OutputTokens: 100},
		{Role: RoleUser, Content: "q4"},
		{Role: RoleToolResult, ToolResult: []map[string]interface{}{{"status": "success"}}},
		{Role: RoleUser, Content: "q5"},
		{Role: RoleAgent, Content: "a-real", IsComplete: true, InputTokens: 10, OutputTokens: 5},
		{Role: RoleUser, Content: "q6"},
	}
	assert.True(t, s.refreshContextSize())
	assert.Equal(t, 15, s.PrevContextSize)
}

// TestRefreshContextSize_EmptyHistory verifies a brand-new conversation (no
// prior agent message) yields a zero baseline.
func TestRefreshContextSize_EmptyHistory(t *testing.T) {
	s := makeTotalTokensSession(1000)
	assert.False(t, s.refreshContextSize())
	assert.Equal(t, 0, s.PrevContextSize)
}

// TestRefreshContextSize_CompactionMarkerGate verifies the re-trigger guard:
// after a compaction whose resume produced NO new agent message, preserved-tail
// messages carrying pre-compaction (large) tokens must not re-establish a
// baseline, while a fresh agent message at-or-after the marker self-heals it.
func TestRefreshContextSize_CompactionMarkerGate(t *testing.T) {
	s := makeTotalTokensSession(1000)
	// Post-compaction history: summary + preserved tail carrying pre-compaction
	// tokens. The compaction marker is the history length at compaction time.
	s.history = []AgentMessage{
		{Role: RoleSystem, Content: "Here is a summary of the conversation so far:\n..."},
		{Role: RoleUser, Content: "old-q"},
		{Role: RoleAgent, Content: "preserved-large", IsComplete: true, InputTokens: 5000, OutputTokens: 1000},
		{Role: RoleUser, Content: "pending-q"},
	}
	s.CompactionHistoryIdx = len(s.history)
	s.PrevContextSize = 0

	// The preserved tail's large tokens must NOT re-trigger compaction.
	assert.False(t, s.refreshContextSize(), "preserved-tail tokens must not re-establish the baseline")
	assert.Equal(t, 0, s.PrevContextSize)

	// A fresh agent message appended at-or-after the marker self-heals the baseline.
	s.history = append(s.history, AgentMessage{Role: RoleAgent, Content: "fresh", IsComplete: true, InputTokens: 300, OutputTokens: 100})
	assert.True(t, s.refreshContextSize())
	assert.Equal(t, 400, s.PrevContextSize)
}

// ---------------------------------------------------------------------------
// shouldCompact – total_tokens trigger behavior
// ---------------------------------------------------------------------------

// TestShouldCompact_TotalTokens_Boundaries verifies the <, ==, > threshold
// boundaries. History must end with a real (non-slash) user message.
func TestShouldCompact_TotalTokens_Boundaries(t *testing.T) {
	history := []AgentMessage{{Role: RoleUser, Content: "m1"}, {Role: RoleUser, Content: "m2"}}
	cases := []struct {
		threshold int
		baseline  int
		want      bool
	}{
		{100, 99, false}, // below threshold
		{100, 100, true}, // exactly at threshold
		{100, 101, true}, // above threshold
	}
	for _, c := range cases {
		s := makeTotalTokensSession(c.threshold)
		s.history = history
		s.PrevContextSize = c.baseline
		assert.Equal(t, c.want, s.shouldCompact(), "threshold=%d baseline=%d", c.threshold, c.baseline)
	}
}

// TestShouldCompact_TotalTokens_FirstTurnNeverTriggers verifies a brand-new
// conversation's first turn never triggers total_tokens compaction (no prior
// agent message -> baseline 0).
func TestShouldCompact_TotalTokens_FirstTurnNeverTriggers(t *testing.T) {
	s := makeTotalTokensSession(100)
	s.history = []AgentMessage{{Role: RoleUser, Content: "first message"}}
	s.refreshContextSize()
	assert.Equal(t, 0, s.PrevContextSize)
	assert.False(t, s.shouldCompact(), "brand-new first turn must never trigger total_tokens compaction")
}

// TestShouldCompact_TotalTokens_InterruptedTurnBackfill verifies that an
// interrupted previous turn (unresolved tool calls) still triggers compaction
// on the next real user input: the last complete agent message exists even
// before the unresolved tool call is resolved.
func TestShouldCompact_TotalTokens_InterruptedTurnBackfill(t *testing.T) {
	s := makeTotalTokensSession(1000)
	// Prior turn interrupted mid-tool-call: a complete agent message requested a
	// tool but no tool result ever arrived (cancelled/errored).
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "", IsComplete: true, InputTokens: 2000, OutputTokens: 100, ToolCalls: []AgentToolCall{{FuncName: "sys_s__fn"}}},
	}
	// StartInference/runTurn resolve unresolved tool calls before the new turn.
	s.resolveUnresolvedToolCalls()
	// The next real user message starts the new turn.
	s.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "q2"})

	s.refreshContextSize()
	assert.Equal(t, 2100, s.PrevContextSize, "interrupted prior turn must not zero the baseline")
	assert.True(t, s.shouldCompact(), "interrupted prior turn must still trigger on the next user input")
}

// TestShouldCompact_TotalTokens_OncePerTurnGuard verifies compaction fires at
// most once per real user turn even when the self-healed baseline still exceeds
// the threshold.
func TestShouldCompact_TotalTokens_OncePerTurnGuard(t *testing.T) {
	s := makeTotalTokensSession(500)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 1000, OutputTokens: 100},
		{Role: RoleUser, Content: "m2"},
	}
	s.refreshContextSize()
	assert.True(t, s.shouldCompact())

	// Compaction fired for this turn; the guard blocks a second fire.
	s.CompactedThisTurn = true
	assert.False(t, s.shouldCompact(), "compaction must fire at most once per real user turn")

	// A fresh agent reply carrying the compacted context self-heals the baseline
	// but the same-turn guard still holds.
	s.appendConvoHistory(&AgentMessage{Role: RoleAgent, Content: "fresh", IsComplete: true, InputTokens: 600, OutputTokens: 50})
	s.PrevContextSize = 650
	assert.False(t, s.shouldCompact())

	// The next user turn (run()) clears the guard and appends a real user
	// message; compaction may fire again on top of the self-healed baseline.
	s.CompactedThisTurn = false
	s.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "m3"})
	assert.True(t, s.shouldCompact(), "the next real user turn may fire compaction once more")
}

// TestShouldCompact_TotalTokens_SlashOriginExcluded verifies slash-origin
// messages are never part of the baseline and a trailing slash command never
// triggers compaction.
func TestShouldCompact_TotalTokens_SlashOriginExcluded(t *testing.T) {
	s := makeTotalTokensSession(100)
	// The only "agent" message is slash-origin with huge tokens; the real
	// conversation has no agent reply at all, so the baseline stays 0.
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "/compact", IsComplete: true, IsSlash: true, InputTokens: 50000, OutputTokens: 50000},
		{Role: RoleUser, Content: "m2"},
	}
	s.refreshContextSize()
	assert.Equal(t, 0, s.PrevContextSize, "slash-origin messages must be excluded from the baseline")
	assert.False(t, s.shouldCompact())
}

// ---------------------------------------------------------------------------
// processAgentMessage – incremental O(1) baseline update
// ---------------------------------------------------------------------------

// TestProcessAgentMessage_IncrementalBaselineUpdate verifies the baseline is
// updated incrementally and reflects the LATEST model call (not the accumulated
// sum), while s.InputTokens/s.OutputTokens accounting for the UI is unchanged.
func TestProcessAgentMessage_IncrementalBaselineUpdate(t *testing.T) {
	s := makeTotalTokensSession(1000)
	s.history = []AgentMessage{{Role: RoleUser, Content: "q1"}}

	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 100, OutputTokens: 50})
	assert.Equal(t, 150, s.PrevContextSize)

	// Second call in the same turn: baseline overwritten (latest call), while
	// the UI accounting accumulates.
	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: "a2", IsComplete: true, InputTokens: 200, OutputTokens: 80})
	assert.Equal(t, 280, s.PrevContextSize, "baseline must reflect the latest call, not the sum")
	assert.Equal(t, 300, s.InputTokens, "UI input-token accounting must remain accumulated")
	assert.Equal(t, 130, s.OutputTokens, "UI output-token accounting must remain accumulated")

	// A slash-origin agent reply must not move the baseline.
	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: "slash-reply", IsComplete: true, IsSlash: true, InputTokens: 9999, OutputTokens: 9999})
	assert.Equal(t, 280, s.PrevContextSize, "slash-origin message must not update the baseline")
}

// TestProcessAgentMessage_SkipsChunkAndIncomplete for the incremental update:
// chunks and incomplete messages are not baselines.
func TestProcessAgentMessage_SkipsChunkAndIncomplete(t *testing.T) {
	s := makeTotalTokensSession(1000)
	s.history = []AgentMessage{{Role: RoleUser, Content: "q1"}}

	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: "chunk", IsComplete: true, IsChunk: true, InputTokens: 999, OutputTokens: 1})
	assert.Equal(t, 0, s.PrevContextSize, "chunks must not set the baseline")

	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: "incomplete", IsComplete: false, InputTokens: 999, OutputTokens: 1})
	assert.Equal(t, 0, s.PrevContextSize, "incomplete messages must not set the baseline")
}

// ---------------------------------------------------------------------------
// once-per-turn guard wiring
// ---------------------------------------------------------------------------

// TestCompactHistory_SetsOncePerTurnGuard verifies compactHistory marks the
// turn so compaction cannot re-fire before the next real user message.
func TestCompactHistory_SetsOncePerTurnGuard(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"summ": {Name: "summ", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})
	agentA := config.Agent{
		Name: "bot", Driver: "openai", Engine: "gpt-4",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy: agentpkg.CompactionStrategySummarize, Threshold: 3,
			ThresholdType: "history_len", PreserveRecent: 1, SummarizationAgent: "summ",
		},
	}
	s := &AgentSession{store: store, Agent: &agentA, ID: "s1", ConvoID: "convo-1"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	s.history = []AgentMessage{{Role: RoleUser, Content: "m1"}, {Role: RoleUser, Content: "m2"}, {Role: RoleUser, Content: "m3"}}

	ok := s.compactHistory()
	require.True(t, ok)
	assert.True(t, s.CompactedThisTurn, "compactHistory must set the once-per-turn guard when it dispatches")
}

// TestRun_ClearsOncePerTurnGuard verifies run() clears the guard for the next
// real user turn.
func TestRun_ClearsOncePerTurnGuard(t *testing.T) {
	_, s := newChatSlashStore(t, nil)
	s.CompactedThisTurn = true
	setChatText(s, AgentSessionTypeChatTurn, "next real message")
	s.run()
	assert.False(t, s.CompactedThisTurn, "a fresh real user message must clear the once-per-turn guard")
}

// TestHandleCompactionResult_SetsMarkerAndResetsBaseline verifies compaction
// records the history boundary and resets PrevContextSize so preserved-tail
// tokens cannot re-trigger on the next turn.
func TestHandleCompactionResult_SetsMarkerAndResetsBaseline(t *testing.T) {
	store, s := makeCompactionResultSession(t, false)
	call := AgentToolCall{
		FuncName: "ag__summ",
		Params: map[string]interface{}{
			"compaction_id": "convo-2_g1",
			"preserve":      2,
		},
	}
	got := s.handleCompactionResult(call, []map[string]interface{}{{"status": "success", "data": "COMPACTED SUMMARY"}})
	assert.True(t, got)
	assert.Equal(t, len(s.history), s.CompactionHistoryIdx, "compaction must record the history boundary")
	assert.Equal(t, 0, s.PrevContextSize, "compaction must reset the baseline")

	// The preserved tail carries pre-compaction tokens but the marker must
	// prevent refreshContextSize from re-establishing them.
	before := s.PrevContextSize
	s.refreshContextSize()
	assert.Equal(t, before, s.PrevContextSize)
	assert.Equal(t, 0, s.PrevContextSize, "preserved-tail tokens must not re-establish the baseline after compaction")
	_ = store
}

// ---------------------------------------------------------------------------
// session-restore coverage
// ---------------------------------------------------------------------------

// TestPrevContextSize_PersistsAcrossRestore verifies the compaction baseline
// fields are serialized with the session and survive ContinueInference /
// PollInference / recover() style restores.
func TestPrevContextSize_PersistsAcrossRestore(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"a": {Name: "a", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}})
	s := &AgentSession{
		ID: "sess-1", ConvoID: "convo-1", store: store,
		Agent:                &config.Agent{Name: "a", Driver: "openai", Engine: "gpt-4"},
		PrevContextSize:      1234,
		CompactedThisTurn:    true,
		CompactionHistoryIdx: 5,
	}
	s.persist(false)

	restored := &AgentSession{}
	restored.setup(&dipper.Message{Labels: map[string]string{"agent_session_id": s.ID}}, store, false)
	assert.Equal(t, 1234, restored.PrevContextSize, "PrevContextSize must survive session restore")
	assert.True(t, restored.CompactedThisTurn, "CompactedThisTurn must survive session restore")
	assert.Equal(t, 5, restored.CompactionHistoryIdx, "CompactionHistoryIdx must survive session restore")
}

// TestRestore_ReDerivesBaselineFromHistory verifies that on restore the
// baseline is re-derived from the loaded history even when the persisted
// PrevContextSize was stale/zero.
func TestRestore_ReDerivesBaselineFromHistory(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"a": {Name: "a", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}})
	convoID := "convo-restore"
	seedMockHistory(store, convoID, []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 300, OutputTokens: 100},
		{Role: RoleUser, Content: "q2"},
	})

	s := &AgentSession{
		ID: "sess-restore", ConvoID: convoID, store: store,
		Agent:           &config.Agent{Name: "a", Driver: "openai", Engine: "gpt-4"},
		PrevContextSize: 0, // stale persisted baseline
	}
	s.persist(false)

	restored := &AgentSession{}
	restored.setup(&dipper.Message{Labels: map[string]string{"agent_session_id": s.ID}}, store, false)
	restored.loadConvoHistory()
	assert.Equal(t, 0, restored.PrevContextSize, "persisted stale baseline is restored as-is")
	restored.refreshContextSize()
	assert.Equal(t, 400, restored.PrevContextSize, "restore must re-derive the baseline from history")
}

// ---------------------------------------------------------------------------
// runTurn integration – post-lock refresh wires into a real turn
// ---------------------------------------------------------------------------

// TestRunTurn_RefreshesBaselineUnderTurnLock drives a full runTurn and verifies
// that the baseline is refreshed under the turn lock (not the pre-lock
// snapshot) so total_tokens compaction fires on the user message.
func TestRunTurn_RefreshesBaselineUnderTurnLock(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {
				Name: "bot", Driver: "openai", Engine: "gpt-4", SystemPrompt: "You are helpful.",
				CompactionPolicy: &agentpkg.CompactionPolicy{
					Strategy: "summarize", Threshold: 500, ThresholdType: "total_tokens",
					PreserveRecent: 1, SummarizationAgent: "summ",
				},
			},
			"summ": {Name: "summ", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	convoID := "convo-live"
	// Seed history whose latest complete agent message carries 1100 tokens
	// (>= threshold 500).
	seedMockHistory(&helper.mockStore, convoID, []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 1000, OutputTokens: 100},
	})

	store.runTurn("bot", convoID, "hello", "user1", "", "")

	// Compaction must have dispatched (agent_call for the summarizer) instead of
	// sending the over-threshold context straight to the model.
	emitted := helper.getEmitted()
	found := false
	for _, m := range emitted {
		if m.Subject == "agent_call" {
			if v, ok := m.Labels["sub_agent_name"]; ok && v == "summ" {
				found = true

				break
			}
		}
	}
	assert.True(t, found, "runTurn must refresh the baseline under the lock and dispatch compaction")
	assert.False(t, helper.hasCall("driver:openai:send_to_model"), "compaction must intercept the over-threshold send")
	assert.True(t, helper.hasCall("cache:lrange"), "runTurn must reload history under the turn lock")
}

// TestRunTurn_NoPriorAgentMessage_NoCompaction verifies that a brand-new
// conversation's first turn does not trigger total_tokens compaction even
// through the full runTurn path.
func TestRunTurn_NoPriorAgentMessage_NoCompaction(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {
				Name: "bot", Driver: "openai", Engine: "gpt-4", SystemPrompt: "You are helpful.",
				CompactionPolicy: &agentpkg.CompactionPolicy{
					Strategy: "summarize", Threshold: 1, ThresholdType: "total_tokens",
					PreserveRecent: 1, SummarizationAgent: "summ",
				},
			},
			"summ": {Name: "summ", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	convoID := "convo-new"
	store.runTurn("bot", convoID, "hello", "user1", "", "")

	// No prior agent message -> baseline 0 -> no compaction; the user message is
	// sent straight to the model.
	emitted := helper.getEmitted()
	for _, m := range emitted {
		assert.NotEqual(t, "agent_call", m.Subject, "brand-new first turn must not dispatch compaction")
	}
	assert.True(t, helper.hasCall("driver:openai:send_to_model"), "brand-new first turn must send to the model")
}

// ---------------------------------------------------------------------------
// cross-turn marker persistence (Issue 1: marker must survive across turns)
// ---------------------------------------------------------------------------

// TestHandleCompactionResult_PersistsMarkerToConvoState verifies that
// handleCompactionResult records the compaction boundary marker in ConvoState,
// not just on the in-memory session, so it can be re-seeded by the fresh
// AgentSession created for the next real user turn.
func TestHandleCompactionResult_PersistsMarkerToConvoState(t *testing.T) {
	store, s := makeCompactionResultSession(t, false)
	call := AgentToolCall{
		FuncName: "ag__summ",
		Params: map[string]interface{}{
			"compaction_id": "convo-2_g1",
			"preserve":      2,
		},
	}
	require.True(t, s.handleCompactionResult(call, []map[string]interface{}{{"status": "success", "data": "COMPACTED SUMMARY"}}))
	marker := s.CompactionHistoryIdx
	require.Greater(t, marker, 0, "compaction must set a non-zero boundary marker")

	// The marker must be persisted in ConvoState (handleCompactionResult runs
	// lockedConvoStateUpdate).
	cs := &ConvoState{}
	cs.load("convo-2", store)
	require.Equal(t, marker, cs.LastCompactionHistoryLen,
		"compaction boundary marker must be persisted in ConvoState")
}

// TestCrossTurn_NewSessionSeedsMarkerFromConvoState verifies the core Issue 1
// fix: a brand-new AgentSession (created for each real user message, labelID
// == "") seeds its CompactionHistoryIdx from the persisted ConvoState marker
// instead of defaulting to 0. After a compaction whose post-compaction resume
// produced no fresh agent message, the next turn must NOT re-derive a stale
// baseline from the preserved tail's pre-compaction (large) tokens.
func TestCrossTurn_NewSessionSeedsMarkerFromConvoState(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {Name: "bot", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})

	// Simulate the persisted state after Turn N's compaction: compacted history
	// = [summary system, preserved tail...] where the preserved agent message
	// still carries pre-compaction (large) tokens, and ConvoState records the
	// compaction boundary marker (= length of the compacted history).
	const marker = 3
	seedMockHistory(store, "convo-cross", []AgentMessage{
		{Role: RoleSystem, Content: "Here is a summary of the conversation so far:\n..."},
		{Role: RoleUser, Content: "old-q"},
		{Role: RoleAgent, Content: "preserved-large", IsComplete: true, InputTokens: 5000, OutputTokens: 1000},
	})
	lockedConvoStateUpdate("convo-cross", store, func(cs *ConvoState) {
		cs.LastCompactionHistoryLen = marker
	})

	// Turn N+1: a brand-new session (fresh AgentSession) must seed its marker
	// from ConvoState.LastCompactionHistoryLen.
	s := &AgentSession{}
	s.setup(&dipper.Message{
		Labels: map[string]string{"agent_name": "bot"},
		Payload: map[string]interface{}{
			"convo_id": "convo-cross",
			"text":     "next question",
		},
	}, store, false)

	assert.Equal(t, marker, s.CompactionHistoryIdx,
		"new session must seed its compaction marker from ConvoState, not default to 0")
	assert.False(t, s.refreshContextSize(),
		"preserved-tail pre-compaction tokens must not re-establish a baseline across turns")
	assert.Equal(t, 0, s.PrevContextSize)
}

// TestRunTurn_CrossTurn_MarkerPreventsRetrigger exercises the full runTurn path
// for the edge case the fix addresses: Turn N compacted, the post-compaction
// resume failed before producing a fresh agent message, and history still holds
// a preserved-tail agent message with pre-compaction tokens. Turn N+1 creates a
// brand-new session; because the marker is persisted in ConvoState and re-seeded
// on the new session, compaction must NOT re-trigger on this turn.
func TestRunTurn_CrossTurn_MarkerPreventsRetrigger(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {
				Name: "bot", Driver: "openai", Engine: "gpt-4", SystemPrompt: "You are helpful.",
				CompactionPolicy: &agentpkg.CompactionPolicy{
					Strategy: "summarize", Threshold: 500, ThresholdType: "total_tokens",
					PreserveRecent: 1, SummarizationAgent: "summ",
				},
			},
			"summ": {Name: "summ", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	convoID := "convo-cross-turn"
	// Persisted state after Turn N's compaction: compacted history length 3,
	// with the preserved agent message carrying 6000 pre-compaction tokens.
	seedMockHistory(&helper.mockStore, convoID, []AgentMessage{
		{Role: RoleSystem, Content: "Here is a summary of the conversation so far:\n..."},
		{Role: RoleUser, Content: "old-q"},
		{Role: RoleAgent, Content: "preserved-large", IsComplete: true, InputTokens: 5000, OutputTokens: 1000},
	})
	lockedConvoStateUpdate(convoID, store, func(cs *ConvoState) {
		cs.LastCompactionHistoryLen = 3
	})

	// Turn N+1 with a new user message. Without the cross-turn marker fix this
	// would re-derive a baseline of 6000 (>= threshold 500) and dispatch
	// compaction; with the marker seeded the baseline stays 0 and the turn goes
	// straight to the model.
	store.runTurn("bot", convoID, "hello", "user1", "", "")

	emitted := helper.getEmitted()
	for _, m := range emitted {
		assert.NotEqual(t, "agent_call", m.Subject,
			"cross-turn preserved-tail tokens must not re-trigger compaction")
	}
	assert.True(t, helper.hasCall("driver:openai:send_to_model"),
		"the new turn must be sent to the model, not compacted")
}
