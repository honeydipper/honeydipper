// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// compactionCall builds the standard compaction tool call for a given archive
// key (compactID).
func compactionCall() AgentToolCall {
	return AgentToolCall{
		FuncName: "ag__summ",
		Params: map[string]interface{}{
			"compaction_id": "convo-2_g1",
			"preserve":      2,
		},
	}
}

// makeCompactionFailureSession builds a session in the exact state a compaction
// attempt leaves behind: pre-compaction history (old1..old4) archived at
// convo-2_g1, the compaction tool-call card as the last live message, and the
// once-per-turn guard set (as compactHistory sets it when it dispatches).
func makeCompactionFailureSession(t *testing.T, slashInitiated bool) (*mockStore, *AgentSession) {
	t.Helper()
	store, s := makeCompactionResultSession(t, slashInitiated)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "old1"},
		{Role: RoleUser, Content: "old2"},
		{Role: RoleUser, Content: "old3"},
		{Role: RoleUser, Content: "old4"},
		{Role: RoleAgent, Content: "", ToolCalls: []AgentToolCall{{FuncName: "ag__summ"}}},
	}
	// Seed the clean pre-compaction archive (captured before the card was
	// appended) and the persisted live history (with the card).
	seedMockHistory(store, "convo-2_g1", s.history[:4])
	seedMockHistory(store, "convo-2", s.history)
	s.CompactedThisTurn = true

	return store, s
}

// persistedLiveHistory reads the persisted convo_history list exactly as the
// cache:lrange handler would return it.
func persistedLiveHistory(t *testing.T, store *mockStore, convoID string) []AgentMessage {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	vals := store.lists[ConvoHistoryKeyPrefix+convoID]
	out := make([]AgentMessage, 0, len(vals))
	for _, v := range vals {
		var m AgentMessage
		require.NoError(t, json.Unmarshal([]byte(v), &m))
		out = append(out, m)
	}

	return out
}

// archiveContents returns the messages expected to be preserved in the _gN
// archive for the standard failure session.
func archiveContents() []AgentMessage {
	return []AgentMessage{
		{Role: RoleUser, Content: "old1"},
		{Role: RoleUser, Content: "old2"},
		{Role: RoleUser, Content: "old3"},
		{Role: RoleUser, Content: "old4"},
	}
}

// assertLiveHistoryPreserved verifies the core invariant: after a compaction
// failure, every pre-compaction (archived) real message is still present in the
// live history and no message was dropped.
func assertLiveHistoryPreserved(t *testing.T, s *AgentSession) {
	t.Helper()
	contents := make([]string, 0, len(s.history))
	for _, m := range s.history {
		contents = append(contents, m.Content)
	}
	for _, want := range []string{"old1", "old2", "old3", "old4"} {
		assert.Contains(t, contents, want, "live history must preserve archived message %q", want)
	}
}

// ---------------------------------------------------------------------------
// validation-first: empty / degenerate / errored summaries never truncate
// ---------------------------------------------------------------------------

// TestHandleCompactionResult_EmptySummary_NoTruncate verifies a genuinely empty
// summary is rejected BEFORE any archive/delete/replace: the live history is
// preserved, a placeholder is injected, the automatic turn resumes, and no
// compaction boundary is recorded.
func TestHandleCompactionResult_EmptySummary_NoTruncate(t *testing.T) {
	store, s := makeCompactionFailureSession(t, false)

	got := s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "success", "data": ""}})
	assert.True(t, got)

	// The automatic failure path resumes the turn with full history.
	assert.True(t, store.hasCall("driver:openai:send_to_model"), "empty summary must still resume the turn")
	// Live history preserved + placeholder injected.
	assertLiveHistoryPreserved(t, s)
	assert.GreaterOrEqual(t, len(s.history), len(archiveContents()))
	// No compaction happened: no boundary, no baseline reset beyond 0, and the
	// rollback did not set a compaction marker.
	assert.Equal(t, 0, s.CompactionHistoryIdx, "no compaction boundary on failure")
	assert.Equal(t, 0, s.PrevContextSize)
}

// TestHandleCompactionResult_WhitespaceSummary_NoTruncate verifies whitespace-
// only output is treated as empty.
func TestHandleCompactionResult_WhitespaceSummary_NoTruncate(t *testing.T) {
	store, s := makeCompactionFailureSession(t, false)

	got := s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "success", "data": "   \n\t "}})
	assert.True(t, got)
	assert.True(t, store.hasCall("driver:openai:send_to_model"))
	assertLiveHistoryPreserved(t, s)
}

// TestHandleCompactionResult_ErroredResult_NoTruncate is the core regression
// guard for the reported bug: a failed summarizer (StartAgentCall's
// SafeExitOnError) emits status=failure with no data.output. Before the fix this
// produced json.Marshal(nil) == "null", which passed the old empty check and
// deleted the live history. Now the status label is consulted FIRST and the
// failure degrades gracefully.
func TestHandleCompactionResult_ErroredResult_NoTruncate(t *testing.T) {
	store, s := makeCompactionFailureSession(t, false)

	got := s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "failure", "reason": "summarizer exploded", "data": nil}})
	assert.True(t, got)

	// The turn must not stall: automatic path resumes with full history.
	assert.True(t, store.hasCall("driver:openai:send_to_model"), "failed summarization must still resume the turn")
	// Live history is NOT replaced with a garbage summary; it is preserved.
	assertLiveHistoryPreserved(t, s)
	assert.GreaterOrEqual(t, len(s.history), len(archiveContents()))
	assert.Equal(t, 0, s.CompactionHistoryIdx, "failed compaction must not set a boundary")
	// No "null" garbage anywhere in the live history.
	for _, m := range s.history {
		assert.NotEqual(t, "null", m.Content, "garbage summary must never enter live history")
	}
}

// TestHandleCompactionResult_DegenerateJSON_NoTruncate verifies the exact
// JSON-literal outputs a broken summarizer can emit (json.Marshal(nil) ==
// "null", empty object/array) are rejected.
func TestHandleCompactionResult_DegenerateJSON_NoTruncate(t *testing.T) {
	for _, degenerate := range []string{"null", "{}", "[]"} {
		t.Run(degenerate, func(t *testing.T) {
			store, s := makeCompactionFailureSession(t, false)
			got := s.handleCompactionResult(compactionCall(),
				[]map[string]interface{}{{"status": "success", "data": degenerate}})
			assert.True(t, got)
			assert.True(t, store.hasCall("driver:openai:send_to_model"))
			assertLiveHistoryPreserved(t, s)
			for _, m := range s.history {
				assert.NotEqual(t, degenerate, strings.TrimSpace(m.Content), "degenerate summary must never enter live history")
			}
		})
	}
}

// TestHandleCompactionResult_MissingStatus_IsFailure verifies that a result
// whose status label is absent (or not "success") is treated as a failure: the
// status label is consulted first and anything other than "success" is rejected.
func TestHandleCompactionResult_MissingStatus_IsFailure(t *testing.T) {
	store, s := makeCompactionFailureSession(t, false)

	got := s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"data": "COMPACTED SUMMARY"}})
	assert.True(t, got)
	assert.True(t, store.hasCall("driver:openai:send_to_model"))
	// The summary was valid but the status was not "success": degrade.
	assertLiveHistoryPreserved(t, s)
	assert.Equal(t, 0, s.CompactionHistoryIdx)
}

// ---------------------------------------------------------------------------
// placeholder marker
// ---------------------------------------------------------------------------

// TestCompactionFailure_PlaceholderMarker verifies the RoleSystem placeholder
// is injected on failure and records the archive location.
func TestCompactionFailure_PlaceholderMarker(t *testing.T) {
	_, s := makeCompactionFailureSession(t, false)

	s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "failure", "data": nil}})

	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleSystem, last.Role, "placeholder must be a RoleSystem message")
	assert.Contains(t, last.Content, "convo-2_g1", "placeholder must record the archive location")
	assert.Contains(t, last.Content, "full conversation history has been preserved")
}

// ---------------------------------------------------------------------------
// archive failure: no panic, graceful skip
// ---------------------------------------------------------------------------

// TestCompactHistory_ArchiveFailure_NoPanic verifies compactHistory no longer
// panics (dipper.Must) when archiveConvo fails: it returns false so the caller
// degrades gracefully, and it does NOT set the once-per-turn guard.
func TestCompactHistory_ArchiveFailure_NoPanic(t *testing.T) {
	store, s := makeCompactionResultSession(t, false)
	store.callHook = func(feature, method string, params map[string]interface{}) error {
		if feature == "cache" && method == "lrange" {
			if k, _ := params["key"].(string); k == ConvoHistoryKeyPrefix+"convo-2" {
				return errors.New("injected archive lrange failure")
			}
		}

		return nil
	}

	// Must NOT panic.
	assert.False(t, s.compactHistory(), "archive failure must degrade instead of panic")
	assert.False(t, s.CompactedThisTurn, "once-per-turn guard must not be set when archive fails")
	assert.False(t, store.hasCall("agent_call"), "summarizer must not be dispatched without an archive")
}

// TestInitNewSession_ForgetHistory_ArchiveFailure_NoPanic verifies the
// forget_history reset tolerates an archive failure instead of panicking via
// dipper.Must (the previous behavior), and still proceeds with the reset.
func TestInitNewSession_ForgetHistory_ArchiveFailure_NoPanic(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["someagent"] = config.Agent{Name: "someagent"}

	cs := &ConvoState{
		ConvoID:         "convo-fh-fail",
		Agent:           &config.Agent{Name: "someagent"},
		PrevContextSize: 999,
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-fh-fail"] = mustMarshalJSON(cs)
	prevHistory := []AgentMessage{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAgent, Content: "Hi there!", IsComplete: true, InputTokens: 500, OutputTokens: 300},
	}
	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-fh-fail"] = mustMarshalJSON(prevHistory)

	// Fail the SECOND lrange (archiveConvo) but let the first (loadConvoHistory)
	// succeed.
	lrangeCount := 0
	store.callHook = func(feature, method string, params map[string]interface{}) error {
		if feature == "cache" && method == "lrange" {
			lrangeCount++
			if lrangeCount == 2 {
				return errors.New("injected archive lrange failure")
			}
		}

		return nil
	}

	msg := &dipper.Message{
		Labels: map[string]string{"agent_name": "someagent"},
		Payload: map[string]interface{}{
			"type":           AgentSessionTypeChatTurn,
			"convo_id":       "convo-fh-fail",
			"forget_history": true,
			"text":           "start fresh",
		},
	}

	s := &AgentSession{}
	// Must NOT panic.
	s.setup(msg, store, false)

	// The reset proceeded: history was cleared to a single marker message.
	require.Len(t, s.history, 1, "forget_history must still reset history even when archival fails")
	assert.Equal(t, RoleSystem, s.history[0].Role)
	// The compaction baseline must be reset to 0.
	csAfter := &ConvoState{}
	csAfter.load(s.ConvoID, store)
	assert.Equal(t, 0, csAfter.PrevContextSize)
	assert.Equal(t, 0, csAfter.LastCompactionHistoryLen)
}

// ---------------------------------------------------------------------------
// rollback semantics: complete archive restores; partial archive keeps live
// ---------------------------------------------------------------------------

// TestCompactionFailure_Rollback_RestoresFullHistory verifies that on failure
// with a complete _gN archive, the live history (in memory and persisted) is
// restored to the pre-compaction state plus the placeholder, and no real
// message is lost.
func TestCompactionFailure_Rollback_RestoresFullHistory(t *testing.T) {
	store, s := makeCompactionFailureSession(t, false)

	s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "failure", "data": nil}})

	// In-memory history = archived messages + placeholder (the card is gone).
	require.Len(t, s.history, len(archiveContents())+1, "rollback must drop the compaction card and keep the full history + placeholder")
	for i, want := range archiveContents() {
		assert.Equal(t, want.Content, s.history[i].Content, "live history[%d] after rollback", i)
	}
	assert.Equal(t, RoleSystem, s.history[len(s.history)-1].Role)

	// Persisted live history reflects the same restored state.
	persisted := persistedLiveHistory(t, store, "convo-2")
	require.Len(t, persisted, len(archiveContents())+1)
	for i, want := range archiveContents() {
		assert.Equal(t, want.Content, persisted[i].Content, "persisted live history[%d]", i)
	}
	// No compaction boundary was recorded.
	assert.Equal(t, 0, s.CompactionHistoryIdx)
	assert.Equal(t, 0, s.PrevContextSize)
}

// TestCompactionFailure_PartialArchive_KeepsLiveHistory verifies that when the
// _gN archive is incomplete (best-effort rpush failed partway), the rollback
// tolerates it and keeps the (fuller) live history rather than replacing it
// with the partial archive.
func TestCompactionFailure_PartialArchive_KeepsLiveHistory(t *testing.T) {
	store, s := makeCompactionResultSession(t, false)
	// Live history: old1..old4 + compaction card.
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "old1"},
		{Role: RoleUser, Content: "old2"},
		{Role: RoleUser, Content: "old3"},
		{Role: RoleUser, Content: "old4"},
		{Role: RoleAgent, Content: "", ToolCalls: []AgentToolCall{{FuncName: "ag__summ"}}},
	}
	// Partial archive: only 3 of the 4 pre-compaction messages made it.
	seedMockHistory(store, "convo-2_g1", []AgentMessage{
		{Role: RoleUser, Content: "old1"},
		{Role: RoleUser, Content: "old2"},
		{Role: RoleUser, Content: "old3"},
	})
	s.CompactedThisTurn = true

	s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "failure", "data": nil}})

	// The live history (old1..old4 + card) is kept, plus the placeholder: no
	// real message was replaced by the partial archive or lost.
	require.Len(t, s.history, 6, "partial archive must keep the full live history + placeholder")
	assertLiveHistoryPreserved(t, s)
	assert.Equal(t, RoleSystem, s.history[len(s.history)-1].Role, "placeholder appended after kept history")
}

// ---------------------------------------------------------------------------
// failure-atomic replace: verify rpush before marking success
// ---------------------------------------------------------------------------

// TestHandleCompactionResult_ReplaceRpushFailure_RollsBack verifies the
// failure-atomic replace: when a rpush of the new compacted history fails after
// the live history was deleted, the result is NOT marked successful — instead
// the code rolls back from the clean _gN archive so no history is lost.
func TestHandleCompactionResult_ReplaceRpushFailure_RollsBack(t *testing.T) {
	store, s := makeCompactionFailureSession(t, false)

	// Fail the FIRST rpush (the summary write of newHistory); rollback's own
	// rpushes come after and succeed.
	rpushCount := 0
	store.callHook = func(feature, method string, params map[string]interface{}) error {
		if feature == "cache" && method == "rpush" {
			rpushCount++
			if rpushCount == 1 {
				return errors.New("injected rpush failure")
			}
		}

		return nil
	}

	got := s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "success", "data": "COMPACTED SUMMARY"}})
	assert.True(t, got)

	// No compaction boundary was recorded (compaction did NOT succeed).
	assert.Equal(t, 0, s.CompactionHistoryIdx)
	assert.Equal(t, 0, s.PrevContextSize)
	// The turn resumed automatically with the restored full history.
	assert.True(t, store.hasCall("driver:openai:send_to_model"))
	// Live history restored from the archive + placeholder, no message lost.
	assertLiveHistoryPreserved(t, s)
	assert.GreaterOrEqual(t, len(s.history), len(archiveContents()))
}

// ---------------------------------------------------------------------------
// automatic vs slash-initiated degradation
// ---------------------------------------------------------------------------

// TestCompactionFailure_Automatic_ResumesWithFullHistory verifies the automatic
// failure path resumes the model conversation (sendToDriver) with the full
// history rather than leaving the turn stuck.
func TestCompactionFailure_Automatic_ResumesWithFullHistory(t *testing.T) {
	store, s := makeCompactionFailureSession(t, false)

	got := s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "failure", "data": nil}})
	assert.True(t, got)
	assert.True(t, store.hasCall("driver:openai:send_to_model"), "automatic failure path must resume via sendToDriver")
	assert.False(t, s.SlashInitiatedCompaction)

	// The history sent to the driver is the FULL preserved history (not a
	// truncated "null" + tail).
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	driverHistory, ok := params["history"].([]AgentMessage)
	require.True(t, ok)
	joined := ""
	for _, m := range driverHistory {
		joined += m.Content
	}
	assert.Contains(t, joined, "old1")
	assert.Contains(t, joined, "old4")
	assert.NotContains(t, joined, "null\n<!-- archived_convo:")
}

// TestCompactionFailure_SlashInitiated_ClearFailureReply verifies the /compact
// failure path posts a clear failure reply, does NOT resume the model, and
// clears the slash-initiated flag so the conversation is never left stuck.
func TestCompactionFailure_SlashInitiated_ClearFailureReply(t *testing.T) {
	store, s := makeCompactionFailureSession(t, true)

	got := s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "failure", "reason": "summarizer down", "data": nil}})
	assert.True(t, got)

	assert.False(t, s.SlashInitiatedCompaction, "slash-initiated flag must be cleared after handling")
	// /compact must NOT resume the model conversation.
	assert.False(t, store.hasCall("driver:openai:send_to_model"), "slash-initiated failure must not call sendToDriver")
	// A clear failure reply was appended (marked IsSlash, visible to UI/poll).
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, last.Role)
	assert.True(t, last.IsSlash)
	assert.True(t, last.IsComplete)
	assert.Contains(t, last.Content, "Compaction failed")
	// Live history preserved.
	assertLiveHistoryPreserved(t, s)
}

// ---------------------------------------------------------------------------
// once-per-turn guard on the degraded path
// ---------------------------------------------------------------------------

// TestCompactionFailure_NoRefireSameTurn_RefireNextTurn verifies the decision
// locked in requirement 4: CompactedThisTurn stays set after a failed
// compaction + degraded sendToDriver so compaction does not immediately re-fire
// within the same turn, and run() clears it on the next real user turn so a
// retry IS allowed then.
func TestCompactionFailure_NoRefireSameTurn_RefireNextTurn(t *testing.T) {
	store, s := makeCompactionFailureSession(t, false)
	// Configure a concrete, decidable threshold (makeCompactionResultSession
	// leaves ThresholdType empty, which would always skip compaction).
	s.Agent.CompactionPolicy.ThresholdType = "history_len"
	s.Agent.CompactionPolicy.Threshold = 3
	s.CompactedThisTurn = true // compactHistory set this when it dispatched

	// Simulate the failed compaction result being handled (guard stays set).
	got := s.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "failure", "data": nil}})
	assert.True(t, got)
	assert.True(t, s.CompactedThisTurn, "failed compaction must leave the once-per-turn guard set")

	// Even though the full history is preserved (still over threshold), the
	// once-per-turn guard prevents an immediate re-fire within the failed turn.
	assert.False(t, s.shouldCompact(), "must not re-fire compaction within the failed turn")

	// Next real user turn: run() clears the guard and appends a fresh user
	// message, so re-firing is allowed again.
	s.CompactedThisTurn = false
	s.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "next-turn-question"})
	assert.True(t, s.shouldCompact(), "the next real user turn may fire compaction once more")
	_ = store
}

// ---------------------------------------------------------------------------
// session-restore mid-compaction
// ---------------------------------------------------------------------------

// TestCompactionFailure_SessionRestore_MidCompaction verifies that a session
// restored from cache while the summarizer is still in flight handles the
// failed result correctly: the persisted compaction state (history with card,
// flags) survives, and the failure path degrades without truncating history.
func TestCompactionFailure_SessionRestore_MidCompaction(t *testing.T) {
	store, s := makeCompactionFailureSession(t, true)

	// Persist the session mid-compaction (summarizer dispatched, waiting).
	s.persist(false)

	// Restore a fresh session from cache, as ContinueInference would.
	restored := &AgentSession{}
	restored.setup(&dipper.Message{Labels: map[string]string{"agent_session_id": s.ID}}, store, false)
	restored.loadConvoHistory()
	restored.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}

	// The compaction state survived restore.
	require.True(t, restored.SlashInitiatedCompaction, "slash-initiated flag must survive restore")
	require.Len(t, restored.history, 5, "history with compaction card must survive restore")

	// Handle the failed result on the restored session.
	got := restored.handleCompactionResult(compactionCall(),
		[]map[string]interface{}{{"status": "failure", "data": nil}})
	assert.True(t, got)

	// No model resume (slash path), clear failure reply, history preserved.
	assert.False(t, store.hasCall("driver:openai:send_to_model"))
	assert.False(t, restored.SlashInitiatedCompaction)
	assertLiveHistoryPreserved(t, restored)
	assert.Equal(t, 0, restored.CompactionHistoryIdx)
	last := restored.history[len(restored.history)-1]
	assert.Contains(t, last.Content, "Compaction failed")
}

// ---------------------------------------------------------------------------
// live history never shrinks on failure
// ---------------------------------------------------------------------------

// TestCompactionFailure_LiveHistoryNeverShrinks drives every failure category
// through the failure path and asserts the persisted live history never drops
// below the pre-compaction message count, for both the automatic and slash
// paths.
func TestCompactionFailure_LiveHistoryNeverShrinks(t *testing.T) {
	failResults := []map[string]interface{}{
		{"status": "success", "data": ""},
		{"status": "success", "data": "null"},
		{"status": "success", "data": "{}"},
		{"status": "success", "data": "[]"},
		{"status": "success", "data": "   "},
		{"status": "failure", "reason": "boom", "data": nil},
	}
	for _, slash := range []bool{false, true} {
		for _, res := range failResults {
			name := "slash=" + strconv.FormatBool(slash) + " data=" + strconv.Quote(fmt.Sprintf("%v", res["data"]))
			t.Run(name, func(t *testing.T) {
				store, s := makeCompactionFailureSession(t, slash)
				preCompactionLen := len(archiveContents())

				got := s.handleCompactionResult(compactionCall(), []map[string]interface{}{res})
				assert.True(t, got)

				// Persisted live history must be >= the pre-compaction count and
				// must contain every archived real message.
				persisted := persistedLiveHistory(t, store, "convo-2")
				assert.GreaterOrEqual(t, len(persisted), preCompactionLen,
					"live history must never shrink below the pre-compaction count")
				persistedContents := make([]string, 0, len(persisted))
				for _, m := range persisted {
					persistedContents = append(persistedContents, m.Content)
				}
				for _, want := range []string{"old1", "old2", "old3", "old4"} {
					assert.Contains(t, persistedContents, want, "persisted live history must keep %q", want)
				}
				// No compaction boundary recorded.
				assert.Equal(t, 0, s.CompactionHistoryIdx)

				if slash {
					assert.False(t, store.hasCall("driver:openai:send_to_model"))
				} else {
					assert.True(t, store.hasCall("driver:openai:send_to_model"), "automatic failure must resume")
				}
			})
		}
	}
}
