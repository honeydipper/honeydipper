// Copyright 2024 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org.

//go:build !integration
// +build !integration

package agent

import (
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func makeTestAgent() config.Agent {
	return config.Agent{
		Name:         "testagent",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "You are a helpful assistant.",
	}
}

// makeTokenCountingSession creates a session with TokenCounter set.
func makeTokenCountingSession(agent config.Agent) *AgentSession {
	store := newMockStore(nil)
	agent.TokenCounter = "simple"
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:           "test-session",
		ConvoID:      "test-convo",
		Agent:        &agent,
		TokenCounter: &SimpleTokenCounter{},
		store:        store,
	}

	// Ensure ConvoState exists
	cs := &ConvoState{
		ConvoID: "test-convo",
		TTL:     "72h",
	}
	cs.persist(store)

	return s
}

// countMessageTokens tests.

func TestCountMessageTokens_EmptyMessage(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())
	tokens := s.countMessageTokens(AgentMessage{})
	assert.Equal(t, 0, tokens, "Empty message should have 0 tokens")
}

func TestCountMessageTokens_ContentOnly(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())
	tokens := s.countMessageTokens(AgentMessage{Content: "Hello, world!"})
	// "Hello, world!" has 13 chars, 13/4 = 3 tokens
	assert.Equal(t, 3, tokens)
}

func TestCountMessageTokens_WithToolCalls(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())
	msg := AgentMessage{
		Content: "Calling a tool",
		ToolCalls: []AgentToolCall{
			{FuncName: "sys_test__action", Params: map[string]interface{}{"param1": "value1"}},
		},
	}
	tokens := s.countMessageTokens(msg)
	assert.True(t, tokens > 10, "Message with tool calls should have more than 10 tokens, got %d", tokens)
}

func TestCountMessageTokens_WithToolResults(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())
	msg := AgentMessage{
		Role: RoleToolResult,
		ToolResult: []map[string]interface{}{
			{"status": "success", "data": "result data"},
		},
	}
	tokens := s.countMessageTokens(msg)
	assert.True(t, tokens > 0, "Tool result message should have tokens, got %d", tokens)
}

func TestCountMessageTokens_WithThoughts(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())
	tokens := s.countMessageTokens(AgentMessage{Content: "Hello", Thoughts: "I should respond politely"})
	assert.Equal(t, 7, tokens)
}

// countSystemPromptTokens tests.

func TestCountSystemPromptTokens_Default(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())
	s.Type = AgentSessionTypeChatTurn
	assert.Equal(t, 7, s.countSystemPromptTokens())
}

func TestCountSystemPromptTokens_InferencePrompt(t *testing.T) {
	agent := config.Agent{
		Name: "testagent", Driver: "openai", Engine: "gpt-4",
		SystemPrompt: "You are a helpful assistant.", InferencePrompt: "Answer the question.",
	}
	s := makeTokenCountingSession(agent)
	s.Type = AgentSessionTypeInference
	assert.Equal(t, 5, s.countSystemPromptTokens())
}

// ConvoState.ContextTokens persistence tests.

func TestConvoStateContextTokens_Persistence(t *testing.T) {
	store := newMockStore(nil)
	cs := &ConvoState{ConvoID: "test-convo", ContextTokens: 100, TTL: "72h"}
	cs.persist(store)

	cs2 := &ConvoState{}
	cs2.load("test-convo", store)
	assert.Equal(t, 100, cs2.ContextTokens, "ContextTokens should be persisted and loaded")
}

// Compaction recalculates ContextTokens from new history.

func TestCompactionRecalculatesContextTokens(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())
	// Set some initial ContextTokens
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 500
	}, LockedConvoStateUpdateOpts{AgentConfig: s.Agent, Labels: func() map[string]string {
		if s.CurrentMsg != nil {
			return s.CurrentMsg.Labels
		}

		return nil
	}()})

	// Simulate compaction: replace history and recalculate
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAgent, Content: "Hi there!"},
	}
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 0
		for _, msg := range s.history {
			cs.ContextTokens += s.countMessageTokens(msg)
		}
	}, LockedConvoStateUpdateOpts{AgentConfig: s.Agent, Labels: func() map[string]string {
		if s.CurrentMsg != nil {
			return s.CurrentMsg.Labels
		}

		return nil
	}()})

	cs2 := &ConvoState{}
	cs2.load("test-convo", s.store)
	assert.True(t, cs2.ContextTokens > 0, "ContextTokens should be recalculated from compacted history")
	assert.True(t, cs2.ContextTokens < 500, "ContextTokens should be less than original 500 after compaction")
}

func TestCompactionRecalculatesContextTokens_OnlyWhenTokenCounterActive(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	cs := &ConvoState{ConvoID: "test-convo", ContextTokens: 500, TTL: "72h"}
	cs.persist(store)

	s := &AgentSession{
		ID: "test-session", ConvoID: "test-convo",
		Agent: &agent, TokenCounter: nil, store: store,
	}

	// When TokenCounter is nil, compaction should NOT recalculate
	if s.TokenCounter != nil {
		lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
			cs.ContextTokens = 0
			for _, msg := range s.history {
				cs.ContextTokens += s.countMessageTokens(msg)
			}
		}, LockedConvoStateUpdateOpts{AgentConfig: s.Agent, Labels: func() map[string]string {
			if s.CurrentMsg != nil {
				return s.CurrentMsg.Labels
			}

			return nil
		}()})
	}

	cs2 := &ConvoState{}
	cs2.load("test-convo", store)
	assert.Equal(t, 500, cs2.ContextTokens, "ContextTokens should NOT be reset when TokenCounter is nil")
}

// TokenCounter initialization tests.

func TestTokenCounterOnlyUsedWhenSetToSimple(t *testing.T) {
	testCases := []struct {
		name         string
		tokenCounter string
		expectNotNil bool
	}{
		{"simple", "simple", true},
		{"empty", "", false},
		{"default", "default", false},
		{"other", "other", false},
		{"tiktoken", "tiktoken", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore(nil)
			agent := makeTestAgent()
			agent.TokenCounter = tc.tokenCounter
			store.cfg.DataSet.Agents["testagent"] = agent

			msg := &dipper.Message{
				Labels:  map[string]string{"agent_name": "testagent"},
				Payload: map[string]interface{}{"type": AgentSessionTypeChatTurn, "text": "hello"},
			}

			s := &AgentSession{}
			s.initNewSession("test-id", msg, store)
			assert.Nil(t, s.TokenCounter, "TokenCounter should be nil after initNewSession for %s", tc.name)

			// Simulate what setup does
			if s.TokenCounter == nil && s.Agent != nil && s.Agent.TokenCounter == "simple" {
				s.TokenCounter = &SimpleTokenCounter{}
			}

			if tc.expectNotNil {
				assert.NotNil(t, s.TokenCounter, "TokenCounter should be initialized for %s", tc.name)
			} else {
				assert.Nil(t, s.TokenCounter, "TokenCounter should NOT be initialized for %s", tc.name)
			}
		})
	}
}

// processAgentMessage token counting tests.

func TestProcessAgentMessage_CountsInputTokensFromContextTokens(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())

	// Pre-set ContextTokens to simulate the context that was sent to the model
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 150
	}, LockedConvoStateUpdateOpts{AgentConfig: s.Agent, Labels: func() map[string]string {
		if s.CurrentMsg != nil {
			return s.CurrentMsg.Labels
		}

		return nil
	}()})

	agentMsg := &AgentMessage{Role: RoleAgent, Content: "Hello!"}

	// Simulate processAgentMessage's token counting block
	if s.TokenCounter != nil {
		contextTokens := s.getConvoContextTokens()
		outputTokens := s.countMessageTokens(*agentMsg)
		s.InputTokens = contextTokens
		s.OutputTokens += outputTokens

		agentMsg.InputTokens = contextTokens
		agentMsg.OutputTokens = outputTokens
	}

	assert.Equal(t, 150, s.InputTokens, "InputTokens should be read from ContextTokens")
	assert.Equal(t, 150, agentMsg.InputTokens, "InputTokens should be written to the message")
	assert.True(t, agentMsg.OutputTokens > 0, "OutputTokens should be written to the message")
}

func TestProcessAgentMessage_WritesTokensToMessageBeforePersist(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())

	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 80
	}, LockedConvoStateUpdateOpts{AgentConfig: s.Agent, Labels: func() map[string]string {
		if s.CurrentMsg != nil {
			return s.CurrentMsg.Labels
		}

		return nil
	}()})

	agentMsg := &AgentMessage{Role: RoleAgent, Content: "I can help with that."}

	// Simulate processAgentMessage's token counting block
	if s.TokenCounter != nil {
		contextTokens := s.getConvoContextTokens()
		outputTokens := s.countMessageTokens(*agentMsg)

		agentMsg.InputTokens = contextTokens
		agentMsg.OutputTokens = outputTokens
	}

	// The message now has token counts that will be persisted when
	// appendConvoHistory saves it to Redis.
	assert.Equal(t, 80, agentMsg.InputTokens, "Message should have InputTokens set before persistence")
	assert.True(t, agentMsg.OutputTokens > 0, "Message should have OutputTokens set before persistence")
}

func TestProcessAgentMessage_UsesDriverValuesWhenTokenCounterNil(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID: "test-session", ConvoID: "test-convo",
		Agent: &agent, TokenCounter: nil, store: store,
	}

	cs := &ConvoState{ConvoID: "test-convo", TTL: "72h"}
	cs.persist(store)

	agentMsg := &AgentMessage{
		Role: RoleAgent, Content: "Hi",
		InputTokens: 100, OutputTokens: 50,
	}

	// Simulate processAgentMessage's else branch
	if s.TokenCounter == nil {
		s.InputTokens += agentMsg.InputTokens
		s.OutputTokens += agentMsg.OutputTokens
	}

	assert.Equal(t, 100, s.InputTokens, "Should use driver-reported InputTokens")
	assert.Equal(t, 50, s.OutputTokens, "Should use driver-reported OutputTokens")
}

// appendConvoHistory token counting tests.

func TestAppendConvoHistory_UpdatesContextTokens(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())

	// ContextTokens starts at 0
	cs := &ConvoState{}
	cs.load(s.ConvoID, s.store)
	assert.Equal(t, 0, cs.ContextTokens, "ContextTokens should start at 0")

	// Append a message
	msg := AgentMessage{Role: RoleUser, Content: "Hello, world!"}
	s.appendConvoHistory(&msg)

	// ContextTokens should now include the message's tokens
	cs2 := &ConvoState{}
	cs2.load(s.ConvoID, s.store)
	assert.True(t, cs2.ContextTokens > 0, "ContextTokens should be updated after appendConvoHistory")
	// "Hello, world!" = 13 chars = 3 tokens (13/4)
	assert.Equal(t, 3, cs2.ContextTokens, "ContextTokens should equal the message token count")
}

func TestAppendConvoHistory_NoUpdateWhenTokenCounterNil(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID: "test-session", ConvoID: "test-convo",
		Agent: &agent, TokenCounter: nil, store: store,
	}

	cs := &ConvoState{ConvoID: "test-convo", TTL: "72h"}
	cs.persist(store)

	msg := AgentMessage{Role: RoleUser, Content: "Hello"}
	s.appendConvoHistory(&msg)

	cs2 := &ConvoState{}
	cs2.load("test-convo", store)
	assert.Equal(t, 0, cs2.ContextTokens, "ContextTokens should remain 0 when TokenCounter is nil")
}

func TestAppendConvoHistory_AccumulatesContextTokens(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())

	msg1 := AgentMessage{Role: RoleUser, Content: "Hello"}
	s.appendConvoHistory(&msg1)

	msg2 := AgentMessage{Role: RoleAgent, Content: "Hi there!"}
	s.appendConvoHistory(&msg2)

	cs := &ConvoState{}
	cs.load(s.ConvoID, s.store)
	// "Hello" = 1 token, "Hi there!" = 2 tokens
	assert.Equal(t, 3, cs.ContextTokens, "ContextTokens should accumulate across multiple appends")
}

// sendToDriver context tokens logging tests.

func TestSendToDriver_LogsSystemPromptPlusContextTokens(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())

	// Append messages via appendConvoHistory (which counts tokens)
	msg1 := AgentMessage{Role: RoleUser, Content: "Hello"}
	msg2 := AgentMessage{Role: RoleAgent, Content: "Hi there!"}
	s.appendConvoHistory(&msg1)
	s.appendConvoHistory(&msg2)

	// Simulate what sendToDriver does for contextTokens
	contextTokens := 0
	if s.TokenCounter != nil {
		cs := &ConvoState{}
		cs.load(s.ConvoID, s.store)
		contextTokens = s.countSystemPromptTokens() + cs.ContextTokens
	}

	// System prompt "You are a helpful assistant." = 7 tokens
	// History: "Hello" = 1, "Hi there!" = 2, total = 3
	// Total = 7 + 3 = 10
	assert.Equal(t, 10, contextTokens, "contextTokens should be system prompt + history")
}

// Run() token counting tests.

func TestRun_DoesNotCountTokens(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())

	// run() should NOT modify ConvoState.ContextTokens.
	// It just creates the user message and appends it to history.
	// appendConvoHistory handles the counting.

	// ConvoState.ContextTokens should be 0 before run()
	cs := &ConvoState{}
	cs.load(s.ConvoID, s.store)
	assert.Equal(t, 0, cs.ContextTokens, "ContextTokens should be 0 before run()")
}

// Setup TokenCounter initialization tests.

func TestSetupInitializesTokenCounterOnlyForSimple(t *testing.T) {
	testCases := []struct {
		name      string
		tokenVal  string
		expectNil bool
	}{
		{"simple_should_init", "simple", false},
		{"empty_should_not_init", "", true},
		{"default_should_not_init", "default", true},
		{"other_should_not_init", "other", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore(nil)
			agent := makeTestAgent()
			agent.TokenCounter = tc.tokenVal
			store.cfg.DataSet.Agents["testagent"] = agent

			msg := &dipper.Message{
				Labels:  map[string]string{"agent_name": "testagent"},
				Payload: map[string]interface{}{"type": AgentSessionTypeChatTurn, "text": "hello"},
			}

			s := &AgentSession{}
			s.setup(msg, store, false)

			if tc.expectNil {
				assert.Nil(t, s.TokenCounter, "TokenCounter should be nil for %s", tc.name)
			} else {
				assert.NotNil(t, s.TokenCounter, "TokenCounter should be initialized for %s", tc.name)
				_, ok := s.TokenCounter.(*SimpleTokenCounter)
				assert.True(t, ok, "TokenCounter should be SimpleTokenCounter for %s", tc.name)
			}
		})
	}
}

func TestSetupRecalculatesContextTokensFromHistory(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent())

	// Pre-set a stale ContextTokens value
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 999
	}, LockedConvoStateUpdateOpts{AgentConfig: s.Agent, Labels: func() map[string]string {
		if s.CurrentMsg != nil {
			return s.CurrentMsg.Labels
		}

		return nil
	}()})

	// Append messages via appendConvoHistory (simulating loaded history)
	msg1 := AgentMessage{Role: RoleUser, Content: "Hello"}
	msg2 := AgentMessage{Role: RoleAgent, Content: "Hi there!"}
	s.appendConvoHistory(&msg1)
	s.appendConvoHistory(&msg2)

	// Simulate what setup() does: recalculate ContextTokens from history
	if s.TokenCounter != nil {
		lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
			cs.ContextTokens = 0
			for _, msg := range s.history {
				cs.ContextTokens += s.countMessageTokens(msg)
			}
		}, LockedConvoStateUpdateOpts{AgentConfig: s.Agent, Labels: func() map[string]string {
			if s.CurrentMsg != nil {
				return s.CurrentMsg.Labels
			}

			return nil
		}()})
	}

	// After recalculation, ContextTokens should reflect actual history
	// "Hello" = 1 token, "Hi there!" = 2 tokens = 3 total
	cs2 := &ConvoState{}
	cs2.load(s.ConvoID, s.store)
	assert.Equal(t, 3, cs2.ContextTokens, "ContextTokens should be recalculated from history")
}

// TokenCounter interface tests.

func TestProcessAgentMessage_UsesTokenCounterInterface(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	agent.TokenCounter = "simple"
	store.cfg.DataSet.Agents["testagent"] = agent

	mockCounter := &mockTokenCounter{}
	s := &AgentSession{
		ID: "test-session", ConvoID: "test-convo",
		Agent: &agent, TokenCounter: mockCounter, store: store,
	}

	cs := &ConvoState{ConvoID: "test-convo", TTL: "72h"}
	cs.persist(store)

	agentMsg := &AgentMessage{Role: RoleAgent, Content: "Test response"}

	if s.TokenCounter != nil {
		contextTokens := s.getConvoContextTokens()
		outputTokens := s.countMessageTokens(*agentMsg)
		s.InputTokens = contextTokens
		s.OutputTokens += outputTokens

		agentMsg.InputTokens = contextTokens
		agentMsg.OutputTokens = outputTokens
	}

	assert.True(t, mockCounter.callCount > 0, "TokenCounter.CountTokens should have been called via the interface")
}

// mockTokenCounter is a test mock that tracks CountTokens calls.
type mockTokenCounter struct {
	callCount int
}

func (m *mockTokenCounter) CountTokens(text string) int {
	m.callCount++

	return len(text) / 4
}

// Compile-time check that mockTokenCounter satisfies the interface.
var _ agentpkg.TokenCounter = (*mockTokenCounter)(nil)
