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

	"github.com/honeydipper/honeydipper/v4/internal/config"
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Phase 1 regression: store getters return nil for missing entities
// ---------------------------------------------------------------------------

// TestCompactHistory_MissingSummarizer_BailsBeforeArchive encodes the user's
// actual scenario: a compaction policy whose summarization_agent references a
// non-existent agents key (e.g. the `campactor`/`compactor` typo). The nil
// return from GetAgent must be honored: compactHistory returns false BEFORE
// archiveConvo (no _gN archive key is created), the once-per-turn guard is NOT
// set, no agent_call is dispatched, and a RoleSystem skip placeholder is
// appended so the skip is visible in history.
func TestCompactHistory_MissingSummarizer_BailsBeforeArchive(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			// The user's typo: the map key is "campactor" but the policy
			// references "compactor" (or vice versa), so GetAgent("compactor")
			// returns nil.
			"campactor": {Name: "campactor", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})
	agentA := config.Agent{
		Name: "bot", Driver: "openai", Engine: "gpt-4",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy:           agentpkg.CompactionStrategySummarize,
			ThresholdType:      "history_len",
			Threshold:          3,
			PreserveRecent:     2,
			SummarizationAgent: "compactor", // typo - no such agents key
		},
	}
	s := &AgentSession{store: store, Agent: &agentA, ID: "s1", ConvoID: "convo-1"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "m3"}, // triggering user message
	}
	seedMockHistory(store, s.ConvoID, s.history)

	// Capture the warning so the "clear log" part of the requirement is
	// asserted, not just the skip behavior.
	var logBuf bytes.Buffer
	s.store.(*mockStore).logger = newDebugCaptureLogger(&logBuf)

	ok := s.compactHistory()
	assert.False(t, ok, "missing summarizer must skip compaction")

	// A clear, diagnostic warning names the configured summarizer.
	logText := logBuf.String()
	assert.Contains(t, logText, "compaction skipped")
	assert.Contains(t, logText, "compactor", "the warning must name the configured summarization agent")

	// Ordering verified: no archive was created (compactHistory bails before
	// archiveConvo). There is no _g1 archive key in the mock store's lists.
	store.mu.Lock()
	_, hasArchive := store.lists[ConvoHistoryKeyPrefix+"convo-1_g1"]
	store.mu.Unlock()
	assert.False(t, hasArchive, "no archive key must be created when the summarizer is missing")

	// The once-per-turn guard is NOT set (compaction did not dispatch).
	assert.False(t, s.CompactedThisTurn, "CompactedThisTurn must not be set when the summarizer is missing")

	// No agent_call to a summarizer was dispatched.
	assert.False(t, store.hasCall("agent_call"), "no summarizer dispatch for a missing agent")

	// A RoleSystem skip placeholder is appended so the skip is visible in history.
	require.GreaterOrEqual(t, len(s.history), 1)
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleSystem, last.Role)
	assert.Contains(t, last.Content, "compactor")
}

// TestCompactHistory_MissingSummarizer_DoesNotSetGuard verifies that a missing
// summarizer does not set CompactedThisTurn, so a later valid turn may still
// fire compaction.
func TestCompactHistory_MissingSummarizer_DoesNotSetGuard(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents:    map[string]config.Agent{},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})
	agentA := config.Agent{
		Name: "bot",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy:           agentpkg.CompactionStrategySummarize,
			ThresholdType:      "history_len",
			Threshold:          3,
			PreserveRecent:     1,
			SummarizationAgent: "summ",
		},
	}
	s := &AgentSession{store: store, Agent: &agentA, ID: "s1", ConvoID: "convo-1"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "m3"},
	}
	seedMockHistory(store, s.ConvoID, s.history)

	assert.False(t, s.compactHistory())
	assert.False(t, s.CompactedThisTurn, "missing summarizer must not set the once-per-turn guard")
}

// TestCompactHistory_SummarizerWithoutName_DispatchesConfiguredName verifies
// that a found summarizer agent WITHOUT a Name field (Phase 2 would populate it
// from the map key; until then the configured name drives the tool call) still
// dispatches `ag__<configured>` (e.g. ag__summ), never a bare `ag__`. This
// makes the sub-agent resolvable by the configured name downstream.
func TestCompactHistory_SummarizerWithoutName_DispatchesConfiguredName(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			// No Name field set on the agent; the map key is the source of truth.
			"summ": {Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})
	agentA := config.Agent{
		Name: "bot",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy:           agentpkg.CompactionStrategySummarize,
			ThresholdType:      "history_len",
			Threshold:          3,
			PreserveRecent:     2,
			SummarizationAgent: "summ",
		},
	}
	s := &AgentSession{store: store, Agent: &agentA, ID: "s1", ConvoID: "convo-1"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "m3"},
	}
	seedMockHistory(store, s.ConvoID, s.history)

	ok := s.compactHistory()
	require.True(t, ok)

	require.Len(t, s.ToolCalls, 1)
	assert.Equal(t, "ag__summ", s.ToolCalls[0].FuncName, "FuncName must be built from the configured summarization_agent, not the empty Name field")
}

// TestCompactHistory_SummarizerWithoutEngine_Skips verifies that a found
// summarizer with no driver/engine is treated as unusable: compaction is
// skipped gracefully (Warning + RoleSystem placeholder), no archive is created,
// and no ag__ call is dispatched with an empty driver/engine.
func TestCompactHistory_SummarizerWithoutEngine_Skips(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"summ": {Name: "summ"}, // found but unusable: no driver/engine
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})
	agentA := config.Agent{
		Name: "bot",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy:           agentpkg.CompactionStrategySummarize,
			ThresholdType:      "history_len",
			Threshold:          3,
			PreserveRecent:     1,
			SummarizationAgent: "summ",
		},
	}
	s := &AgentSession{store: store, Agent: &agentA, ID: "s1", ConvoID: "convo-1"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "m3"},
	}
	seedMockHistory(store, s.ConvoID, s.history)

	assert.False(t, s.compactHistory(), "unusable summarizer must skip compaction")
	store.mu.Lock()
	_, hasArchive := store.lists[ConvoHistoryKeyPrefix+"convo-1_g1"]
	store.mu.Unlock()
	assert.False(t, hasArchive, "no archive must be created for an unusable summarizer")
	assert.False(t, store.hasCall("agent_call"))
	assert.False(t, s.CompactedThisTurn)
	require.GreaterOrEqual(t, len(s.history), 1)
	assert.Equal(t, RoleSystem, s.history[len(s.history)-1].Role)
}

// TestCompactHistory_MissingSummarizer_SlashInitiated_RepliesAndNoPlaceholder
// verifies the slash /compact path: a missing summarizer produces the
// slash-specific explanatory reply and does NOT append a RoleSystem placeholder
// (the reply is the surface for slash-initiated skips).
func TestCompactHistory_MissingSummarizer_SlashInitiated_RepliesAndNoPlaceholder(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents:    map[string]config.Agent{},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})
	agentA := config.Agent{
		Name: "bot", Driver: "openai", Engine: "gpt-4",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy:           agentpkg.CompactionStrategySummarize,
			SummarizationAgent: "summ",
			PreserveRecent:     2,
		},
	}
	s := &AgentSession{store: store, Agent: &agentA, ID: "s1", ConvoID: "convo-1"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "m3"},
	}

	// Directly invoke slashCompact so the reply is produced. The slash path
	// checks the summarizer BEFORE setting SlashInitiatedCompaction, so the
	// flag must remain false (zero value) for a missing summarizer.
	handled := slashCompact(s, "")
	assert.True(t, handled)
	assert.False(t, s.SlashInitiatedCompaction, "flag must not be set when the summarizer is missing")
	require.NotEmpty(t, s.history)
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, last.Role, "slash path replies via an agent message")
	assert.True(t, last.IsComplete)
	assert.Contains(t, last.Content, "summ")
	assert.False(t, store.hasCall("agent_call"))
}

// ---------------------------------------------------------------------------
// Phase 1 regression: slashRefresh degrades gracefully on a removed agent
// ---------------------------------------------------------------------------

// TestSlashRefresh_AgentRemoved_NoSetConvoAgentNil verifies that /refresh with
// an agent removed from config replies gracefully, does NOT call setConvoAgent
// with nil, and does NOT persist a nil agent into ConvoState.Agent.
func TestSlashRefresh_AgentRemoved_NoSetConvoAgentNil(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		// The agent "a" is NOT in the config (removed after session setup).
		Agents:    map[string]config.Agent{},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})
	// The session was set up while the agent existed; its ConvoState carries the
	// prior agent config, but the store's current config no longer has it.
	s := &AgentSession{
		store:   store,
		Agent:   &config.Agent{Name: "a", Driver: "openai", Engine: "gpt-4", SystemPrompt: "You are helpful."},
		ID:      "s-refresh",
		ConvoID: "convo-refresh",
	}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{
		"convo_id": s.ConvoID,
		"text":     "hello",
	}}

	// Pre-persist a ConvoState carrying the prior agent so we can verify the
	// /refresh handler does NOT clobber it with nil.
	prior := &ConvoState{ConvoID: s.ConvoID, TTL: ConvoStreamTTL, Agent: s.Agent}
	store.resp["cache:load:"+ConvoStateKeyPrefix+s.ConvoID] = mustMarshalJSON(prior)

	handled := slashRefresh(s, "")
	assert.True(t, handled)
	assert.False(t, store.hasCall("agent_call"))

	// The reply is a graceful explanation, not a panic or silent success.
	require.NotEmpty(t, s.history)
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, last.Role)
	assert.Contains(t, last.Content, "Cannot refresh")
	assert.Contains(t, last.Content, "no longer found")

	// setConvoAgent(nil) must NOT have run: the session Agent is untouched.
	assert.NotNil(t, s.Agent, "session Agent must not be overwritten with nil")
	assert.Equal(t, "a", s.Agent.Name)

	// ConvoState.Agent must not be persisted as nil.
	cs := &ConvoState{}
	cs.load(s.ConvoID, store)
	assert.NotNil(t, cs.Agent, "ConvoState.Agent must not be persisted as nil")
}

// ---------------------------------------------------------------------------
// Phase 1 regression: ErrAgentNotFound is now live in resolveOrRecreateConvoState
// ---------------------------------------------------------------------------

// TestResolveOrRecreateConvoState_ErrAgentNotFound_Live verifies the previously
// dead ErrAgentNotFound branch: GetAgent now returns nil for a missing agent, so
// recreating a ConvoState with a nonexistent agent surfaces ErrAgentNotFound.
func TestResolveOrRecreateConvoState_ErrAgentNotFound_Live(t *testing.T) {
	helper := newMemCacheHelper()
	// The helper's config has test_agent/new_agent/etc but NOT "ghost_agent".
	store := &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      newCaptureLogger(&bytes.Buffer{}),
	}

	// Use the persistent store via StartTurn: convo state missing + agent
	// supplied -> recreate path -> GetAgent returns nil -> ErrAgentNotFound.
	err := store.StartTurn("ghost-convo", "hi", "u", "", "", "ghost_agent", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

// ---------------------------------------------------------------------------
// Phase 1 regression: summarizer recursion guard
// ---------------------------------------------------------------------------

// TestStartAgentCall_SummarizerRecursionGuard verifies that a compaction
// sub-agent session does NOT re-trigger compaction: even when the summarizer
// agent itself carries a compaction_policy, StartAgentCall clears it on the
// sub-agent session so the summarizer summarizes the archive instead of
// dispatching another ag__ call (which would recurse).
func TestStartAgentCall_SummarizerRecursionGuard(t *testing.T) {
	helper := newMemCacheHelper()
	// The summarizer agent has its own compaction_policy that WOULD recurse if
	// left intact on the sub-agent session.
	helper.cfg.DataSet.Agents["summ"] = config.Agent{
		Name: "summ", Driver: "openai", Engine: "gpt-4",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy:           agentpkg.CompactionStrategySummarize,
			ThresholdType:      "history_len",
			Threshold:          1,
			PreserveRecent:     1,
			SummarizationAgent: "summ",
		},
	}
	store := &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      newCaptureLogger(&bytes.Buffer{}),
	}

	// Seed an archived history long enough that, without the recursion guard,
	// the summarizer would trigger compaction again (threshold=1).
	archived := []AgentMessage{
		{Role: RoleUser, Content: "old1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true},
		{Role: RoleUser, Content: "old2"},
	}
	for _, m := range archived {
		_, _ = helper.Call("cache", "rpush", map[string]interface{}{
			"key":   ConvoHistoryKeyPrefix + "convo-1_g1",
			"value": string(dipper.SerializeContent(m)),
		})
	}

	agentCallsBefore := helper.callCount("eventbus:agent_call")

	msg := &dipper.Message{
		Labels: map[string]string{
			"sub_agent_name":   "summ",
			"agent_session_id": "parent-sess-1",
			"turn_id":          "1",
			"tool_call_id":     "0",
			"convo_id":         "convo-1",
		},
		Payload: map[string]interface{}{
			"input":          "Summarize the conversation.",
			"compaction_id":  "convo-1_g1",
			"summarize_upto": 2,
		},
	}
	store.StartAgentCall(msg)
	store.Wait()

	// No nested agent_call was dispatched by the summarizer session (no
	// recursion), and the summarizer proceeded to send to the model.
	agentCallsAfter := helper.callCount("eventbus:agent_call")
	assert.Equal(t, agentCallsBefore, agentCallsAfter, "summarizer must not re-trigger compaction (no nested agent_call)")
	assert.GreaterOrEqual(t, helper.callCount("driver:openai:send_to_model"), 1, "summarizer must send the summary request to the model")
}
