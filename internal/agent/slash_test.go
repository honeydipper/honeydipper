// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"strings"
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

	return last
}

// ---------------------------------------------------------------------------
// gating tests
// ---------------------------------------------------------------------------

func TestSlashRun_ChatTurnHelp_DoesNotSendToDriver(t *testing.T) {
	store, s := newChatSlashStore(t, nil)

	s.run()

	// No model invocation; slash command text is NOT recorded as a user message.
	assert.False(t, store.hasCall("driver:openai:send_to_model"))
	require.Len(t, s.history, 1)
	last := requireLastAgentReply(t, s)
	assert.Contains(t, last.Content, "/help")
	assert.Contains(t, last.Content, "/compact")
	assert.Contains(t, last.Content, "/model")
	assert.Contains(t, last.Content, "/refresh")

	for _, m := range s.history {
		assert.NotEqual(t, RoleUser, m.Role, "slash command text must not be recorded as a user message")
	}
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
	for _, cmd := range []string{"/help", "/refresh", "/model", "/compact"} {
		assert.Contains(t, last.Content, cmd)
	}
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

	emitted := store.getEmitted()
	require.NotEmpty(t, emitted, "expected an agent_call to the summarizer")
	assert.Equal(t, "agent_call", emitted[0].Subject)

	// The slash text must NOT have been recorded as a user message: the history
	// length is unchanged from the seeded history plus the summarizer tool-call
	// entry (no extra RoleUser message was appended).
	require.NotEmpty(t, s.history)
	last := s.history[len(s.history)-1]
	assert.Equal(t, RoleAgent, last.Role)
	assert.NotEmpty(t, last.ToolCalls)
	for _, m := range s.history {
		if strings.HasPrefix(m.Content, "/compact") {
			t.Fatalf("slash text leaked into history as a user message")
		}
	}
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
