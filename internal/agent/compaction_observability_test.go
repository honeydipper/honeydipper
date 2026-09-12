// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"bytes"
	"testing"

	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

// newDebugCaptureLogger returns a logger that writes DEBUG (and above) records
// as plain messages into buf. It is used to assert that the compaction decision
// points emitted the expected debug log lines (Phase 2 observability).
//
// op/go-logging gates every record with IsEnabledFor() against the GLOBAL
// defaultBackend (not the per-logger backend set below), and other tests in the
// package may re-create that global backend at INFO level via dipper.GetLogger.
// We therefore pin the level for this test-only module on the global backend so
// Debugf records are not filtered out regardless of other tests' logger setup.
func newDebugCaptureLogger(buf *bytes.Buffer) *logging.Logger {
	const module = "agent-compaction-observability-test"
	logging.SetLevel(logging.DEBUG, module)

	backend := logging.NewLogBackend(buf, "", 0)
	format := logging.MustStringFormatter("%{message}")
	formatted := logging.NewBackendFormatter(backend, format)
	leveled := logging.AddModuleLevel(formatted)
	leveled.SetLevel(logging.DEBUG, module)
	logger := logging.MustGetLogger(module)
	logger.SetBackend(leveled)

	return logger
}

// makeObservabilitySession builds a total_tokens session whose mock store has a
// DEBUG capture logger wired in so shouldCompact/compactHistory decisions can be
// observed. It returns the session and the buffer the logger writes into.
func makeObservabilitySession(threshold int) (*AgentSession, *bytes.Buffer) {
	s := makeTotalTokensSession(threshold)
	buf := &bytes.Buffer{}
	s.store.(*mockStore).logger = newDebugCaptureLogger(buf)

	return s, buf
}

// ---------------------------------------------------------------------------
// API-exposed metric == compaction metric
// ---------------------------------------------------------------------------

// TestPrevContextSize_APIExposedEqualsCompactionMetric verifies that the value
// the API exposes (ConvoState.PrevContextSize, persisted by syncPrevContextSize)
// is exactly the value that shouldCompact() compares against the threshold for
// threshold_type: total_tokens.
func TestPrevContextSize_APIExposedEqualsCompactionMetric(t *testing.T) {
	s := makeTotalTokensSession(1000)
	store := s.store.(*mockStore)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 800, OutputTokens: 200},
		{Role: RoleUser, Content: "q2"},
	}

	// Deriving the baseline must persist a snapshot into ConvoState.
	assert.True(t, s.refreshContextSize(), "baseline must be found")
	assert.Equal(t, 1000, s.PrevContextSize)

	cs := &ConvoState{}
	cs.load(s.ConvoID, store)
	assert.Equal(t, s.PrevContextSize, cs.PrevContextSize,
		"API-exposed PrevContextSize must equal the session baseline that shouldCompact compares")

	// shouldCompact must compare against exactly that persisted value.
	s.Agent.CompactionPolicy.Threshold = 1000
	assert.True(t, s.shouldCompact(), "baseline >= threshold must trigger compaction")
	s.Agent.CompactionPolicy.Threshold = 1001
	assert.False(t, s.shouldCompact(), "baseline < threshold must not trigger compaction")
}

// TestPrevContextSize_APIExposed_PersistedOnSelfHeal verifies that the
// incremental self-heal in processAgentMessage also persists the updated
// baseline to ConvoState.
func TestPrevContextSize_APIExposed_PersistedOnSelfHeal(t *testing.T) {
	s := makeTotalTokensSession(1000)
	store := s.store.(*mockStore)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 100, OutputTokens: 50},
	}
	s.PrevContextSize = 0

	s.processAgentMessage(&AgentMessage{
		Role:         RoleAgent,
		Content:      "a2",
		IsComplete:   true,
		InputTokens:  300,
		OutputTokens: 100,
	})

	assert.Equal(t, 400, s.PrevContextSize)
	cs := &ConvoState{}
	cs.load(s.ConvoID, store)
	assert.Equal(t, 400, cs.PrevContextSize,
		"self-heal baseline update must be persisted to ConvoState")
}

// TestPrevContextSize_APIExposed_ResetOnCompaction verifies that compaction
// resets the API-exposed metric in ConvoState back to 0.
func TestPrevContextSize_APIExposed_ResetOnCompaction(t *testing.T) {
	store, s := makeCompactionResultSession(t, false)
	// Seed a stale persisted baseline; compaction must reset it.
	lockedConvoStateUpdate(s.ConvoID, store, func(cs *ConvoState) {
		cs.PrevContextSize = 9999
	})

	call := AgentToolCall{
		FuncName: "ag__summ",
		Params: map[string]interface{}{
			"compaction_id": "convo-2_g1",
			"preserve":      2,
		},
	}
	got := s.handleCompactionResult(call, []map[string]interface{}{{"data": "COMPACTED SUMMARY"}})
	assert.True(t, got)
	assert.Equal(t, 0, s.PrevContextSize)

	cs := &ConvoState{}
	cs.load(s.ConvoID, store)
	assert.Equal(t, 0, cs.PrevContextSize,
		"compaction must reset the API-exposed PrevContextSize to 0")
}

// ---------------------------------------------------------------------------
// Debug logging
// ---------------------------------------------------------------------------

// TestShouldCompact_LogsTriggerDecision verifies that shouldCompact emits a
// debug log with the current context size, threshold, and trigger decision for
// threshold_type: total_tokens.
func TestShouldCompact_LogsTriggerDecision(t *testing.T) {
	s, logBuf := makeObservabilitySession(500)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 200, OutputTokens: 100},
		{Role: RoleUser, Content: "q2"},
	}
	s.PrevContextSize = 300

	assert.False(t, s.shouldCompact())
	logText := logBuf.String()
	assert.Contains(t, logText, "compaction check threshold_type=total_tokens")
	assert.Contains(t, logText, "context_size=300")
	assert.Contains(t, logText, "threshold=500")
	assert.Contains(t, logText, "trigger=false")
}

// TestShouldCompact_LogsSkipDecision verifies that shouldCompact emits a debug
// log when it skips because the last non-slash message is not a user message.
func TestShouldCompact_LogsSkipDecision(t *testing.T) {
	s, logBuf := makeObservabilitySession(500)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 200, OutputTokens: 100},
	}

	assert.False(t, s.shouldCompact())
	assert.Contains(t, logBuf.String(),
		"compaction skipped: last non-slash message is not a user message")
}

// TestCompactHistory_LogsThresholdContext verifies compactHistory logs the
// current threshold, context size, and history length before dispatching.
func TestCompactHistory_LogsThresholdContext(t *testing.T) {
	s, logBuf := makeObservabilitySession(100)
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "q1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true, InputTokens: 100, OutputTokens: 50},
		{Role: RoleUser, Content: "q2"},
		{Role: RoleAgent, Content: "a2", IsComplete: true, InputTokens: 200, OutputTokens: 100},
		{Role: RoleUser, Content: "q3"},
	}
	s.PrevContextSize = 300
	s.CompactedThisTurn = false

	assert.True(t, s.compactHistory())
	logText := logBuf.String()
	assert.Contains(t, logText, "compactHistory")
	assert.Contains(t, logText, "threshold_type=total_tokens")
	assert.Contains(t, logText, "context_size=300")
}
