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
// parseSlashCommand
// ---------------------------------------------------------------------------

func TestParseSlashCommand(t *testing.T) {
	cases := []struct {
		text     string
		expected *slashCommand
	}{
		{"/help", &slashCommand{name: "help", args: ""}},
		{"/help ", &slashCommand{name: "help", args: ""}},
		{"/compact", &slashCommand{name: "compact", args: ""}},
		{"/model gpt-4", &slashCommand{name: "model", args: "gpt-4"}},
		{"/model   openai:gpt-4", &slashCommand{name: "model", args: "openai:gpt-4"}},
		{"/retry some args here", &slashCommand{name: "retry", args: "some args here"}},
		{"/", &slashCommand{name: "", args: ""}},
		{"help", nil},
		{"not a slash", nil},
		{"  /model x", &slashCommand{name: "model", args: "x"}},
		{"", nil},
	}
	for _, c := range cases {
		got := parseSlashCommand(c.text)
		assert.Equal(t, c.expected, got, "parseSlashCommand(%q)", c.text)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newChatSlashStore(t *testing.T, mutate func(*config.Config)) (*mockStore, *AgentSession) {
	t.Helper()
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"a": {Name: "a", Driver: "openai", Engine: "gpt-4", SystemPrompt: "You are helpful."},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}}
	if mutate != nil {
		mutate(cfg)
	}
	store := newMockStore(cfg)

	msg := &dipper.Message{
		Labels: map[string]string{"agent_name": "a"},
		Payload: map[string]interface{}{
			"convo_id": "convo-slash",
			"text":     "/help",
		},
	}
	s := &AgentSession{}
	s.setup(msg, store, false)

	return store, s
}

// DriverConfigWithAgentEngines returns a drivers config exposing one
// agent driver ("openai") with two engines, shaped the way
// drivers.daemon.drivers.<name> and drivers.<name>.engines are laid out.
func DriverConfigWithAgentEngines() map[string]interface{} {
	return map[string]interface{}{
		"openai": map[string]interface{}{
			"engines": map[string]interface{}{
				"gpt-4":   map[string]interface{}{},
				"gpt-3.5": map[string]interface{}{},
			},
		},
		"daemon": map[string]interface{}{
			"drivers": map[string]interface{}{
				"openai": map[string]interface{}{
					"meta": map[string]interface{}{
						"labels": []interface{}{"agent_driver"},
					},
				},
			},
		},
	}
}

// setChatText mutates the session's current message text and type.
func setChatText(s *AgentSession, typ, text string) {
	payload, ok := s.CurrentMsg.Payload.(map[string]interface{})
	if !ok {
		panic("expected map payload")
	}
	payload["text"] = text
	payload["type"] = typ
	s.Type = typ
}

func requireLastAgentReply(t *testing.T, s *AgentSession) AgentMessage {
	t.Helper()
	require.NotEmpty(t, s.history)
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, last.Role)
	assert.True(t, last.IsComplete)
	assert.Empty(t, last.ToolCalls)
	// Every reply produced for a slash command must be marked IsSlash so it is
	// excluded from the model context (this helper is only used for slash tests).
	assert.True(t, last.IsSlash, "slash reply must be marked IsSlash")

	return last
}

// ---------------------------------------------------------------------------
// gating tests
// ---------------------------------------------------------------------------

func TestSlashRun_ChatTurnHelp_DoesNotSendToDriver(t *testing.T) {
	store, s := newChatSlashStore(t, nil)

	s.run()

	// No model invocation for a slash command.
	assert.False(t, store.hasCall("driver:openai:send_to_model"))
	// History: the marked slash command text (RoleUser) followed by the marked reply.
	require.Len(t, s.history, 2)
	assert.Equal(t, RoleUser, s.history[0].Role)
	assert.Equal(t, "/help", s.history[0].Content)
	assert.True(t, s.history[0].IsSlash, "slash command text must be marked IsSlash")
	last := requireLastAgentReply(t, s)
	assert.True(t, last.IsSlash, "reply to a slash command must be marked IsSlash")
	assert.Contains(t, last.Content, "/help")
	assert.Contains(t, last.Content, "/compact")
	assert.Contains(t, last.Content, "/model")
	assert.Contains(t, last.Content, "/refresh")
}

func TestSlashRun_InferenceTextPassesThrough(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	setChatText(s, AgentSessionTypeInference, "/help")

	s.run()

	// Inference sessions always flow through untouched, even with a leading slash.
	assert.True(t, store.hasCall("driver:openai:send_to_model"))
	require.Len(t, s.history, 1)
	assert.Equal(t, RoleUser, s.history[0].Role)
	assert.Equal(t, "/help", s.history[0].Content)
}

func TestSlashRun_NonSlashChatPassesThrough(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	setChatText(s, AgentSessionTypeChatTurn, "hello there")

	s.run()

	// Non-slash chat text flows through untouched.
	assert.True(t, store.hasCall("driver:openai:send_to_model"))
	require.Len(t, s.history, 1)
	assert.Equal(t, RoleUser, s.history[0].Role)
	assert.Equal(t, "hello there", s.history[0].Content)
}

func TestSlashRun_UnknownCommand_Replies(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	setChatText(s, AgentSessionTypeChatTurn, "/bogus")

	s.run()

	assert.False(t, store.hasCall("driver:openai:send_to_model"))
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "Unknown command")
	assert.Contains(t, last.Content, "/help")
}

// ---------------------------------------------------------------------------
// /help
// ---------------------------------------------------------------------------

func TestSlashHelp_ListsAllCommands(t *testing.T) {
	_, s := newChatSlashStore(t, nil)
	s.run()

	last := requireLastAgentReply(t, s)
	for _, cmd := range []string{"/help", "/refresh", "/model", "/compact", "/retry"} {
		assert.Contains(t, last.Content, cmd)
	}
}

func TestSlashHelp_IsMarkdown(t *testing.T) {
	_, s := newChatSlashStore(t, nil)
	s.run()

	last := requireLastAgentReply(t, s)
	content := last.Content
	// The help output is proper markdown: a bold header, blank-line separated
	// bullet lines, and a trailing instruction line joined with newlines.
	assert.Contains(t, content, "**Available commands**")
	assert.Contains(t, content, "\n- `/help`")
	assert.Contains(t, content, "- `/help` — Show this list of commands")
	assert.Contains(t, content, "- `/compact` — Compact the conversation history to save context")
	assert.Contains(t, content, "- `/retry` — Regenerate the last response")
	assert.Contains(t, content, "- `/refresh` — Rebuild the system prompt from current agent config")
	assert.Contains(t, content, "- `/model` — Switch the model for this conversation")
	assert.Contains(t, content, "Type a command followed by Enter to run it.")
}

// ---------------------------------------------------------------------------
// /refresh
// ---------------------------------------------------------------------------

func TestSlashRefresh_ReinterpolatesAndPersistsAgent(t *testing.T) {
	store, s := newChatSlashStore(t, func(cfg *config.Config) {
		a := cfg.DataSet.Agents["a"]
		a.SystemPrompt = "Prompt for {{ .agent_name }}"
		cfg.DataSet.Agents["a"] = a
	})
	setChatText(s, AgentSessionTypeChatTurn, "/refresh")

	s.run()

	// reply confirms the refresh and persists the re-interpolated agent.
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "refreshed")

	cs := &ConvoState{}
	cs.load(s.ConvoID, store)
	require.NotNil(t, cs.Agent)
	assert.Equal(t, "a", cs.Agent.Name)
}

// ---------------------------------------------------------------------------
// /model
// ---------------------------------------------------------------------------

func TestSlashModel_BareName_PersistsDriverAndEngine(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	setChatText(s, AgentSessionTypeChatTurn, "/model gpt-4")

	s.run()

	assert.False(t, store.hasCall("driver:openai:send_to_model"))
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "openai:gpt-4")

	cs := &ConvoState{}
	cs.load(s.ConvoID, store)
	require.NotNil(t, cs.Agent)
	assert.Equal(t, "gpt-4", cs.Agent.Engine)
	assert.Equal(t, "openai", cs.Agent.Driver)
}

func TestSlashModel_DriverEngineForm_Persists(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	setChatText(s, AgentSessionTypeChatTurn, "/model openai:gpt-3.5")

	s.run()

	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "openai:gpt-3.5")

	cs := &ConvoState{}
	cs.load(s.ConvoID, store)
	require.NotNil(t, cs.Agent)
	assert.Equal(t, "gpt-3.5", cs.Agent.Engine)
	assert.Equal(t, "openai", cs.Agent.Driver)
}

func TestSlashModel_Invalid_RepliesErrorWithEngines(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	setChatText(s, AgentSessionTypeChatTurn, "/model nope")

	s.run()

	assert.False(t, store.hasCall("driver:openai:send_to_model"))
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "Unknown model: nope")
	assert.Contains(t, last.Content, "gpt-4")
	assert.Contains(t, last.Content, "gpt-3.5")
}

func TestSlashModel_NoArgs_RepliesUsage(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	setChatText(s, AgentSessionTypeChatTurn, "/model")

	s.run()

	assert.False(t, store.hasCall("driver:openai:send_to_model"))
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "Usage")
}

// ---------------------------------------------------------------------------
// /compact
// ---------------------------------------------------------------------------

func TestSlashCompact_NotConfigured_Replies(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	setChatText(s, AgentSessionTypeChatTurn, "/compact")

	s.run()

	assert.False(t, store.hasCall("driver:openai:send_to_model"))
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "not configured")
	assert.False(t, s.SlashInitiatedCompaction, "flag must be cleared when compaction cannot run")
	assert.Empty(t, store.getEmitted())
}

func TestSlashCompact_ForceDispatchesSummarizer(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"a": {
				Name:   "a",
				Driver: "openai",
				Engine: "gpt-4",
				CompactionPolicy: &agentpkg.CompactionPolicy{
					Strategy:           "summarize",
					SummarizationAgent: "summ",
					PreserveRecent:     2,
				},
			},
			"summ": {Name: "summ", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})

	msg := &dipper.Message{
		Labels: map[string]string{"agent_name": "a"},
		Payload: map[string]interface{}{
			"convo_id": "convo-compact",
			"text":     "/compact",
		},
	}
	s := &AgentSession{}
	s.setup(msg, store, false)

	// Seed a history long enough to compact.
	s.history = make([]AgentMessage, 12)
	for i := range s.history {
		s.history[i] = AgentMessage{Role: RoleUser, Content: "m" + string(rune('a'+i))}
	}

	s.run()

	// compactHistory dispatched a summarizer sub-agent: it returns immediately
	// to the async compaction flow and never calls sendToDriver or reply().
	assert.False(t, store.hasCall("driver:openai:send_to_model"))
	// The session is marked as a slash-initiated compaction so
	// handleCompactionResult knows to post the confirmation and NOT resume the
	// model conversation when the summarizer returns.
	assert.True(t, s.SlashInitiatedCompaction, "slash-initiated compaction flag must be set before dispatch")

	emitted := store.getEmitted()
	require.NotEmpty(t, emitted, "expected an agent_call to the summarizer")
	assert.Equal(t, "agent_call", emitted[0].Subject)

	// The slash command text IS recorded as a marked RoleUser message (IsSlash)
	// so the UI has a complete record; it is excluded from the model context via
	// the IsSlash marker. The summarizer tool-call entry is REAL model context
	// and must NOT be marked IsSlash.
	require.NotEmpty(t, s.history)
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, last.Role)
	assert.NotEmpty(t, last.ToolCalls)
	assert.False(t, last.IsSlash, "summarizer tool-call is real model context and must not be marked IsSlash")

	// The slash text entry itself is marked IsSlash.
	var slashEntry *AgentMessage
	for i := range s.history {
		if s.history[i].Content == "/compact" {
			slashEntry = &s.history[i]
		}
	}
	require.NotNil(t, slashEntry, "expected the /compact command text in history")
	assert.Equal(t, RoleUser, slashEntry.Role)
	assert.True(t, slashEntry.IsSlash, "slash command text must be marked IsSlash")
}

// ---------------------------------------------------------------------------
// reply returned via history + poll
// ---------------------------------------------------------------------------

func TestSlashRun_NonTurnReply_ObservableViaHistoryAndPoll(t *testing.T) {
	store, s := newChatSlashStore(t, nil)

	s.run()

	// The complete RoleAgent message is persisted in the conversation history.
	last := requireLastAgentReply(t, s)
	assert.True(t, last.IsComplete)
	require.True(t, store.hasCall("cache:rpush"), "reply must be persisted to convo history")

	// The poll loop must observe the reply as an agent_response without any
	// model invocation. Start LastPoll at 0 so the poll collects the reply.
	s.LastPoll = 0
	pollMsg := &dipper.Message{
		Labels: map[string]string{
			"resume_key":       "wf.0",
			"agent_session_id": s.ID,
		},
	}
	emittedBefore := len(store.getEmitted())
	handled := s.emitPollResponse(pollMsg)
	assert.True(t, handled, "emitPollResponse should emit the non-turn reply")

	emitted := store.getEmitted()
	require.Greater(t, len(emitted), emittedBefore)
	lastEmitted := emitted[len(emitted)-1]
	assert.Equal(t, "agent_response", lastEmitted.Subject)
	assert.Equal(t, "success", pollMsg.Labels["status"], "poll loop should mark the response successful")

	fullMessages, ok := lastEmitted.Payload.(map[string]interface{})["full_messages"].([]map[string]string)
	require.True(t, ok, "expected full_messages in poll response")
	require.NotEmpty(t, fullMessages)
	assert.Contains(t, fullMessages[0]["content"], "/help")
}

// ---------------------------------------------------------------------------
// IsSlash refinement tests
// ---------------------------------------------------------------------------

func TestSlash_ReplyIsMarked_NormalModelMessageIsNot(t *testing.T) {
	store, s := newChatSlashStore(t, nil)

	// A slash command reply must be marked IsSlash.
	s.appendConvoHistory(&AgentMessage{Role: RoleUser, User: "u", Content: "/help", IsSlash: true})
	s.reply("some help output")
	require.NotEmpty(t, s.history)
	reply := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, reply.Role)
	assert.True(t, reply.IsSlash, "slash reply must be marked IsSlash")

	// A normal model message via processAgentMessage must NOT be marked.
	s.TokenCounter = nil
	modelMsg := &AgentMessage{Role: RoleAgent, Content: "normal model output", IsComplete: true}
	s.processAgentMessage(modelMsg)
	last := s.history[len(s.history)-1]
	assert.Equal(t, "normal model output", last.Content)
	assert.False(t, last.IsSlash, "normal model message must not be marked IsSlash")
	_ = store
}

func TestSlash_ModelNeverSeesSlashMessages(t *testing.T) {
	store, s := newChatSlashStore(t, nil)

	// Interleave normal conversation messages with slash-origin messages.
	s.history = []AgentMessage{
		{Role: RoleUser, User: "u", Content: "hello"},
		{Role: RoleAgent, Content: "hi there", IsComplete: true},
		{Role: RoleUser, User: "u", Content: "/help", IsSlash: true},
		{Role: RoleAgent, Content: "Available slash commands...", IsComplete: true, IsSlash: true},
		{Role: RoleUser, User: "u", Content: "next question"},
	}

	// Call sendToDriver directly and inspect the history passed to the driver.
	s.sendToDriver()
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params, "sendToDriver must invoke the driver")
	driverHistory, ok := params["history"].([]AgentMessage)
	require.True(t, ok)

	// Build expected: system prompt plus only the non-slash messages in order.
	expected := []string{"You are helpful.", "hello", "hi there", "next question"}
	require.Len(t, driverHistory, len(expected))
	for i, want := range expected {
		assert.Equal(t, want, driverHistory[i].Content, "driver history[%d]", i)
		assert.False(t, driverHistory[i].IsSlash, "driver must never see slash-origin messages")
	}
}

func TestSlash_MarkedReplyStillReturnedViaPoll(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	s.run()

	// The marked reply is still returned via emitPollResponse/agent_response on poll.
	require.Len(t, s.history, 2)
	assert.True(t, s.history[1].IsSlash, "reply should be marked IsSlash")
	s.LastPoll = 0
	pollMsg := &dipper.Message{Labels: map[string]string{
		"resume_key":       "wf.0",
		"agent_session_id": s.ID,
	}}
	emittedBefore := len(store.getEmitted())
	handled := s.emitPollResponse(pollMsg)
	assert.True(t, handled, "emitPollResponse should emit the marked reply")
	emitted := store.getEmitted()
	require.Greater(t, len(emitted), emittedBefore)
	lastEmitted := emitted[len(emitted)-1]
	assert.Equal(t, "agent_response", lastEmitted.Subject)
	fullMessages, ok := lastEmitted.Payload.(map[string]interface{})["full_messages"].([]map[string]string)
	require.True(t, ok)
	require.NotEmpty(t, fullMessages)
	assert.Contains(t, fullMessages[0]["content"], "/help")
}

func TestSlash_MarkedMessageVisibleInConvoHistory(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	s.run()

	// The slash command text and reply are persisted to the shared convo history.
	require.Len(t, s.history, 2)
	assert.True(t, s.history[0].IsSlash)
	assert.True(t, s.history[1].IsSlash)

	// loadConvoHistory must read them back unchanged (UI read path).
	s2 := &AgentSession{ConvoID: s.ConvoID, store: store}
	s2.loadConvoHistory()
	require.Len(t, s2.history, 2)
	assert.Equal(t, "/help", s2.history[0].Content)
	assert.True(t, s2.history[0].IsSlash, "slash command text must survive loadConvoHistory")
	assert.Equal(t, RoleAgent, s2.history[1].Role)
	assert.True(t, s2.history[1].IsSlash, "slash reply must survive loadConvoHistory")
}

func TestSlash_TokenCountingSkipsSlashMessages(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())
	// Reset ContextTokens to known state.
	cs := &ConvoState{}
	cs.load(s.ConvoID, s.store)
	cs.ContextTokens = 0
	cs.persist(s.store)

	// Append a normal user message -> tokens counted.
	s.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "normal long message here"})
	// Append a slash message -> tokens NOT counted.
	s.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "/help", IsSlash: true})
	// Append a slash reply -> tokens NOT counted.
	s.appendConvoHistory(&AgentMessage{Role: RoleAgent, Content: "Available commands", IsComplete: true, IsSlash: true})

	// ContextTokens should only reflect the normal message.
	cs2 := &ConvoState{}
	cs2.load(s.ConvoID, s.store)
	assert.True(t, cs2.ContextTokens > 0, "normal message tokens should count")
	onlyNormal := s.countMessageTokens(AgentMessage{Role: RoleUser, Content: "normal long message here"})
	assert.Equal(t, onlyNormal, cs2.ContextTokens, "ContextTokens must skip slash messages")
}

func TestSlash_ShouldCompactIgnoresTrailingSlashUserMessage(t *testing.T) {
	// Agent with history_len compaction policy, threshold 3.
	store, s := newChatSlashStore(t, func(cfg *config.Config) {
		a := cfg.DataSet.Agents["a"]
		a.CompactionPolicy = &agentpkg.CompactionPolicy{
			Strategy:      "summarize",
			ThresholdType: "history_len",
			Threshold:     3,
		}
		cfg.DataSet.Agents["a"] = a
	})
	// Point the session at the compaction-enabled agent config.
	pol := *s.Agent.CompactionPolicy
	s.Agent = &config.Agent{
		Name:             s.Agent.Name,
		Driver:           s.Agent.Driver,
		Engine:           s.Agent.Engine,
		SystemPrompt:     s.Agent.SystemPrompt,
		CompactionPolicy: &pol,
	}

	// Three normal messages + a trailing slash user message. The threshold (3)
	// is reached by the normal messages, but the last non-slash message is a
	// RoleUser so compaction IS due here.
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "/model x", IsSlash: true},
	}
	assert.True(t, s.shouldCompact(), "normal-user tail triggers compaction when threshold reached")

	// Now make the trailing normal message an agent message, so the last
	// non-slash message is not a user message and compaction must NOT trigger,
	// even though a trailing slash user command is present.
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true},
		{Role: RoleAgent, Content: "a2", IsComplete: true},
		{Role: RoleUser, Content: "/help", IsSlash: true},
	}
	assert.False(t, s.shouldCompact(), "trailing slash command must not trigger compaction")

	// With no slash messages at all, the last non-slash user message governs.
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleAgent, Content: "a1", IsComplete: true},
		{Role: RoleAgent, Content: "a2", IsComplete: true},
		{Role: RoleUser, Content: "real question"},
	}
	assert.True(t, s.shouldCompact())
	_ = store
}

func TestSlash_MarkersSurvivePersistLoadAndFilterNextTurn(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	s.run()
	require.Len(t, s.history, 2)

	// Persist the session; the markers are part of the serialized history.
	s.persist(false)

	// Restore a fresh session from cache (restore path).
	restored := &AgentSession{}
	restored.setup(&dipper.Message{Labels: map[string]string{
		"agent_session_id": s.ID,
	}}, store, false)
	restored.loadConvoHistory()
	require.Len(t, restored.history, 2)
	assert.True(t, restored.history[0].IsSlash, "slash marker must survive persist+load")
	assert.True(t, restored.history[1].IsSlash, "slash reply marker must survive persist+load")

	// Next turn: append a normal user message and send to driver; the slash
	// messages must still be filtered out.
	restored.history = append(restored.history, AgentMessage{Role: RoleUser, Content: "next turn"})
	restored.sendToDriver()
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	driverHistory, ok := params["history"].([]AgentMessage)
	require.True(t, ok)
	expected := []string{"You are helpful.", "next turn"}
	require.Len(t, driverHistory, len(expected))
	for i, want := range expected {
		assert.Equal(t, want, driverHistory[i].Content, "driver history[%d]", i)
		assert.False(t, driverHistory[i].IsSlash)
	}
}

// ---------------------------------------------------------------------------
// Finding 2: slash-initiated compaction must NOT resume the model conversation
// ---------------------------------------------------------------------------

// makeCompactionResultSession builds a session with enough history for
// handleCompactionResult to run, with or without the slash-initiated flag.
func makeCompactionResultSession(t *testing.T, slashInitiated bool) (*mockStore, *AgentSession) {
	t.Helper()
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
			Strategy:           agentpkg.CompactionStrategySummarize,
			PreserveRecent:     2,
			SummarizationAgent: "summ",
		},
	}
	s := &AgentSession{store: store, Agent: &agentA, ID: "s2", ConvoID: "convo-2"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	s.SlashInitiatedCompaction = slashInitiated
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "old1"},
		{Role: RoleUser, Content: "old2"},
		{Role: RoleUser, Content: "old3"},
		{Role: RoleUser, Content: "old4"},
		{Role: RoleUser, Content: "old5"},
	}

	return store, s
}

func TestHandleCompactionResult_SlashInitiated_DoesNotSendToDriver(t *testing.T) {
	store, s := makeCompactionResultSession(t, true)

	call := AgentToolCall{
		FuncName: "ag__summ",
		Params: map[string]interface{}{
			"compaction_id": "convo-2_g1",
			"preserve":      2,
		},
	}
	got := s.handleCompactionResult(call, []map[string]interface{}{{"data": "COMPACTED SUMMARY"}})
	assert.True(t, got, "handleCompactionResult must take over the result for a compaction call")

	// The flag is cleared once the result is handled.
	assert.False(t, s.SlashInitiatedCompaction, "slash-initiated flag must be cleared after handling")

	// The model is NOT resumed: sendToDriver must not be invoked.
	assert.False(t, store.hasCall("driver:openai:send_to_model"), "slash-initiated compaction must NOT call sendToDriver")

	// The in-memory history is summary system message + preserved tail +
	// the marked confirmation reply appended by handleCompactionResult.
	require.GreaterOrEqual(t, len(s.history), 3)
	// 0: summary system message
	assert.Equal(t, RoleSystem, s.history[0].Role)
	assert.Contains(t, s.history[0].Content, "COMPACTED SUMMARY")
	// 1..2: preserved tail (old3, old4) - the last old5 is the tool-call slot
	// handled by handleCompactionResult (toolIndex = total-1).
	assert.Equal(t, "old3", s.history[len(s.history)-3].Content)

	// Last message: the confirmation reply, marked IsSlash + IsComplete.
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, last.Role)
	assert.True(t, last.IsSlash, "compaction confirmation reply must be marked IsSlash")
	assert.True(t, last.IsComplete, "compaction confirmation reply must be complete")
	assert.Contains(t, last.Content, "Conversation compacted")
}

func TestHandleCompactionResult_Automatic_StillSendsToDriver(t *testing.T) {
	store, s := makeCompactionResultSession(t, false)

	call := AgentToolCall{
		FuncName: "ag__summ",
		Params: map[string]interface{}{
			"compaction_id": "convo-2_g1",
			"preserve":      2,
		},
	}
	got := s.handleCompactionResult(call, []map[string]interface{}{{"data": "COMPACTED SUMMARY"}})
	assert.True(t, got)

	// Automatic compaction (no slash-initiated flag) still resumes the model
	// conversation - this is the regression guard for unchanged behavior.
	assert.True(t, store.hasCall("driver:openai:send_to_model"), "automatic compaction must still call sendToDriver")
	assert.False(t, s.SlashInitiatedCompaction)
}

func TestSlashCompact_Confirmation_ObservableViaHistoryAndPoll(t *testing.T) {
	store, s := makeCompactionResultSession(t, true)

	// Simulate the full slash flow: dispatch /compact (sets the flag), then the
	// summarizer returns and handleCompactionResult posts the confirmation.
	call := AgentToolCall{
		FuncName: "ag__summ",
		Params: map[string]interface{}{
			"compaction_id": "convo-2_g1",
			"preserve":      2,
		},
	}
	s.handleCompactionResult(call, []map[string]interface{}{{"data": "COMPACTED SUMMARY"}})

	// The confirmation is returned via the poll/emitPollResponse path.
	s.LastPoll = 0
	pollMsg := &dipper.Message{Labels: map[string]string{
		"resume_key":       "wf.0",
		"agent_session_id": s.ID,
	}}
	emittedBefore := len(store.getEmitted())
	handled := s.emitPollResponse(pollMsg)
	assert.True(t, handled, "emitPollResponse should emit the compaction confirmation")
	emitted := store.getEmitted()
	require.Greater(t, len(emitted), emittedBefore)
	lastEmitted := emitted[len(emitted)-1]
	assert.Equal(t, "agent_response", lastEmitted.Subject)
	fullMessages, ok := lastEmitted.Payload.(map[string]interface{})["full_messages"].([]map[string]string)
	require.True(t, ok, "expected full_messages in poll response")
	require.NotEmpty(t, fullMessages)
	assert.Contains(t, fullMessages[0]["content"], "Conversation compacted")
}

func TestSlashCompact_HandlerDispatchesWithoutSendToDriver(t *testing.T) {
	store := newMockStore(&config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"a": {
				Name:   "a",
				Driver: "openai",
				Engine: "gpt-4",
				CompactionPolicy: &agentpkg.CompactionPolicy{
					Strategy:           "summarize",
					SummarizationAgent: "summ",
					PreserveRecent:     2,
				},
			},
			"summ": {Name: "summ", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
		Drivers:   DriverConfigWithAgentEngines(),
	}})

	msg := &dipper.Message{
		Labels: map[string]string{"agent_name": "a"},
		Payload: map[string]interface{}{
			"convo_id": "convo-compact2",
			"text":     "/compact",
		},
	}
	s := &AgentSession{}
	s.setup(msg, store, false)
	// Seed a history long enough to compact.
	s.history = make([]AgentMessage, 12)
	for i := range s.history {
		s.history[i] = AgentMessage{Role: RoleUser, Content: "m" + string(rune('a'+i))}
	}

	// Invoke the slash handler directly (not via run, so we can observe the
	// return value and the emitted agent_call).
	ok := slashCompact(s, "")
	assert.True(t, ok)
	assert.True(t, s.SlashInitiatedCompaction, "flag set when compaction dispatched")
	// Dispatches the summarizer without any model invocation.
	assert.False(t, store.hasCall("driver:openai:send_to_model"), "compact handler must not call sendToDriver")
	emitted := store.getEmitted()
	require.NotEmpty(t, emitted)
	assert.Equal(t, "agent_call", emitted[0].Subject)
	// No reply posted at dispatch time (confirmation comes from the result path).
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, last.Role)
	assert.NotEmpty(t, last.ToolCalls)
}

func TestSlashCompact_HandlerNoPolicy_RepliesNoModel(t *testing.T) {
	store, s := newChatSlashStore(t, nil) // no compaction policy
	setChatText(s, AgentSessionTypeChatTurn, "/compact")

	s.run()

	assert.False(t, store.hasCall("driver:openai:send_to_model"), "no model call when compaction not configured")
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "not configured")
	assert.Empty(t, store.getEmitted())
}

// ---------------------------------------------------------------------------
// /retry
// ---------------------------------------------------------------------------

func TestSlashRetry_RegeneratesPreviousResponse(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	// Seed a conversation with a real user turn and its assistant response.
	s.history = []AgentMessage{
		{Role: RoleUser, User: "u", Content: "hello"},
		{Role: RoleAgent, Content: "hi there", IsComplete: true},
	}
	setChatText(s, AgentSessionTypeChatTurn, "/retry")

	s.run()

	// The model is invoked to regenerate the response.
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params, "sendToDriver must be invoked for /retry")
	driverHistory, ok := params["history"].([]AgentMessage)
	require.True(t, ok)

	// The history is rewound: the previous assistant response is dropped and the
	// previous real user message is re-sent. The /retry command text must NOT be
	// what is sent to the model.
	expected := []string{"You are helpful.", "hello"}
	require.Len(t, driverHistory, len(expected))
	for i, want := range expected {
		assert.Equal(t, want, driverHistory[i].Content, "driver history[%d]", i)
		assert.False(t, driverHistory[i].IsSlash)
	}

	// The in-memory history after the rewind holds the previous user message
	// (now the re-appended normal RoleUser) plus the preserved /retry marker.
	require.NotEmpty(t, s.history)
	assert.Equal(t, RoleUser, s.history[0].Role)
	assert.Equal(t, "hello", s.history[0].Content)
	assert.False(t, s.history[0].IsSlash, "re-appended user message must not be marked slash")
}

func TestSlashRetry_SkipsTrailingSlashMessagesToFindRealUserTurn(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	// A real conversation followed by a separate slash turn (e.g. /help). The
	// slash-origin messages must not be mistaken for the previous user turn.
	s.history = []AgentMessage{
		{Role: RoleUser, User: "u", Content: "hello"},
		{Role: RoleAgent, Content: "hi there", IsComplete: true},
		{Role: RoleUser, Content: "/help", IsSlash: true},
		{Role: RoleAgent, Content: "Available commands...", IsComplete: true, IsSlash: true},
	}
	setChatText(s, AgentSessionTypeChatTurn, "/retry")

	s.run()

	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	driverHistory, ok := params["history"].([]AgentMessage)
	require.True(t, ok)
	expected := []string{"You are helpful.", "hello"}
	require.Len(t, driverHistory, len(expected))
	for i, want := range expected {
		assert.Equal(t, want, driverHistory[i].Content, "driver history[%d]", i)
		assert.False(t, driverHistory[i].IsSlash)
	}
}

func TestSlashRetry_NoPriorUserMessage_RepliesErrorNoModel(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	// No real user message in history (only slash-origin entries).
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "/help", IsSlash: true},
	}
	setChatText(s, AgentSessionTypeChatTurn, "/retry")

	s.run()

	// No model invocation; a helpful marked error reply is produced instead.
	assert.False(t, store.hasCall("driver:openai:send_to_model"), "no model call when there is nothing to retry")
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "No previous user message")
}

func TestSlashRetry_EmptyHistory_RepliesErrorNoModel(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	s.history = nil
	setChatText(s, AgentSessionTypeChatTurn, "/retry")

	s.run()

	assert.False(t, store.hasCall("driver:openai:send_to_model"), "no model call on empty history")
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "No previous user message")
}

func TestSlashRetry_WorkflowChatTurn_RegeneratedResponseObservableViaPoll(t *testing.T) {
	store, s := newChatSlashStore(t, nil)
	s.history = []AgentMessage{
		{Role: RoleUser, User: "u", Content: "hello"},
		{Role: RoleAgent, Content: "old response", IsComplete: true},
	}
	setChatText(s, AgentSessionTypeChatTurn, "/retry")

	// Workflow-originated path: runTurn -> run() dispatches /retry which calls
	// sendToDriver(). No entry-specific code is required.
	s.run()
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params, "regeneration dispatched for /retry")
	driverHistory, _ := params["history"].([]AgentMessage)
	require.Len(t, driverHistory, 2)
	assert.Equal(t, "hello", driverHistory[1].Content)

	// Simulate the model returning the regenerated response (ReceiveInference ->
	// processAgentResponse -> processAgentMessage appends to history).
	respMsg := &dipper.Message{
		Labels: map[string]string{"agent_session_id": s.ID, "status": "success"},
		Payload: map[string]interface{}{
			"message": map[string]interface{}{
				"Role":       RoleAgent,
				"Content":    "regenerated response",
				"IsComplete": true,
			},
		},
	}
	s.processAgentResponse(respMsg)

	// The regenerated response is appended to the history.
	require.NotEmpty(t, s.history)
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, last.Role)
	assert.Equal(t, "regenerated response", last.Content)

	// The workflow poll path (PollInference -> emitPollResponse) surfaces the
	// regenerated response as the new agent_response. After the rewind,
	// LastPoll pointed just before the regenerated response.
	pollMsg := &dipper.Message{Labels: map[string]string{
		"resume_key":       "wf.0",
		"agent_session_id": s.ID,
	}}
	emittedBefore := len(store.getEmitted())
	handled := s.emitPollResponse(pollMsg)
	assert.True(t, handled, "emitPollResponse should emit the regenerated response")
	emitted := store.getEmitted()
	require.Greater(t, len(emitted), emittedBefore)
	lastEmitted := emitted[len(emitted)-1]
	assert.Equal(t, "agent_response", lastEmitted.Subject)
	fullMessages, ok := lastEmitted.Payload.(map[string]interface{})["full_messages"].([]map[string]string)
	require.True(t, ok, "expected full_messages in poll response")
	require.NotEmpty(t, fullMessages)
	assert.Contains(t, fullMessages[0]["content"], "regenerated response")
}
