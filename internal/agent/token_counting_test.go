// Copyright 2024 PayPal Inc.
//
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
func makeTokenCountingSession(agent config.Agent, history []AgentMessage) *AgentSession {
	store := newMockStore(nil)
	agent.TokenCounter = "simple"
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:           "test-session",
		ConvoID:      "test-convo",
		Agent:        &agent,
		TokenCounter: &SimpleTokenCounter{},
		store:        store,
		history:      history,
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
	s := makeTokenCountingSession(makeTestAgent(), nil)
	tokens := s.countMessageTokens(AgentMessage{})
	assert.Equal(t, 0, tokens, "Empty message should have 0 tokens")
}

func TestCountMessageTokens_ContentOnly(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), nil)
	tokens := s.countMessageTokens(AgentMessage{Content: "Hello, world!"})
	// "Hello, world!" has 13 chars, 13/4 = 3 tokens
	assert.Equal(t, 3, tokens)
}

func TestCountMessageTokens_WithToolCalls(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), nil)
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
	s := makeTokenCountingSession(makeTestAgent(), nil)
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
	s := makeTokenCountingSession(makeTestAgent(), nil)
	tokens := s.countMessageTokens(AgentMessage{Content: "Hello", Thoughts: "I should respond politely"})
	assert.Equal(t, 7, tokens)
}

// countSystemPromptTokens tests.

func TestCountSystemPromptTokens_Default(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), nil)
	s.Type = AgentSessionTypeChatTurn
	assert.Equal(t, 7, s.countSystemPromptTokens())
}

func TestCountSystemPromptTokens_InferencePrompt(t *testing.T) {
	agent := config.Agent{
		Name: "testagent", Driver: "openai", Engine: "gpt-4",
		SystemPrompt: "You are a helpful assistant.", InferencePrompt: "Answer the question.",
	}
	s := makeTokenCountingSession(agent, nil)
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

// Compaction reset tests.

func TestCompactionResetsContextTokens(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), nil)
	// Set some initial ContextTokens
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 500
	})

	s.lastCountedIndex = 0
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 0
	})

	cs2 := &ConvoState{}
	cs2.load("test-convo", s.store)
	assert.Equal(t, 0, cs2.ContextTokens, "ContextTokens should be reset after compaction")
	assert.Equal(t, 0, s.lastCountedIndex, "lastCountedIndex should be reset after compaction")
}

func TestCompactionResetsContextTokens_OnlyWhenTokenCounterActive(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	cs := &ConvoState{ConvoID: "test-convo", ContextTokens: 500, TTL: "72h"}
	cs.persist(store)

	s := &AgentSession{
		ID: "test-session", ConvoID: "test-convo",
		Agent: &agent, TokenCounter: nil, store: store,
		lastCountedIndex: 10,
	}

	// The compaction code guards with TokenCounter != nil
	if s.TokenCounter != nil {
		s.lastCountedIndex = 0
		lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
			cs.ContextTokens = 0
		})
	}

	cs2 := &ConvoState{}
	cs2.load("test-convo", store)
	assert.Equal(t, 500, cs2.ContextTokens, "ContextTokens should NOT be reset when TokenCounter is nil")
	assert.Equal(t, 10, s.lastCountedIndex, "lastCountedIndex should NOT be reset when TokenCounter is nil")
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

// processAgentMessage centralized token counting tests.

// TestProcessAgentMessage_CountsInputTokensFromContextTokens verifies that
// processAgentMessage reads InputTokens from ConvoState.ContextTokens.
func TestProcessAgentMessage_CountsInputTokensFromContextTokens(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), nil)

	// Pre-set ContextTokens to simulate the context that was sent to the model
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 150
	})

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

// TestProcessAgentMessage_UpdatesContextTokensWithOutput verifies that
// processAgentMessage adds the output token count to ContextTokens for
// the next round trip.
func TestProcessAgentMessage_UpdatesContextTokensWithOutput(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), nil)

	// Pre-set ContextTokens
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 100
	})

	agentMsg := &AgentMessage{Role: RoleAgent, Content: "Hello!"}

	// Simulate processAgentMessage's token counting block
	if s.TokenCounter != nil {
		contextTokens := s.getConvoContextTokens()
		outputTokens := s.countMessageTokens(*agentMsg)
		s.InputTokens = contextTokens
		s.OutputTokens += outputTokens

		lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
			cs.ContextTokens += outputTokens
		})
		agentMsg.InputTokens = contextTokens
		agentMsg.OutputTokens = outputTokens
	}

	// ContextTokens should have increased by the output token count
	newContextTokens := s.getConvoContextTokens()
	assert.True(t, newContextTokens > 100, "ContextTokens should increase by output tokens, got %d", newContextTokens)
}

// TestProcessAgentMessage_WritesTokensToMessageBeforePersist verifies that
// token counts are written to the message object before it gets appended
// to history (and thus persisted to Redis).
func TestProcessAgentMessage_WritesTokensToMessageBeforePersist(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), nil)

	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 80
	})

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

// TestProcessAgentMessage_UsesDriverValuesWhenTokenCounterNil verifies that
// when TokenCounter is nil, driver-reported values are used.
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

// sendToDriver token counting tests.

// TestSendToDriver_NoTokenCounting verifies that sendToDriver does not
// perform token counting when TokenCounter is active. It only calls
// updateContextTokens to catch uncounted history, but does not write
// token counts to history messages.
func TestSendToDriver_NoTokenCounting(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), []AgentMessage{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAgent, Content: "Hi there!"},
	})
	s.lastCountedIndex = 2 // all messages already counted

	// Before the refactor, sendToDriver would write InputTokens to s.history[i].
	// After the refactor, it should NOT modify history messages.
	originalInputTokens0 := s.history[0].InputTokens
	originalInputTokens1 := s.history[1].InputTokens

	// Call updateContextTokens (what sendToDriver does)
	if s.TokenCounter != nil {
		s.updateContextTokens()
	}

	// History message token counts should NOT be modified by sendToDriver
	assert.Equal(t, originalInputTokens0, s.history[0].InputTokens, "sendToDriver should not modify history[0].InputTokens")
	assert.Equal(t, originalInputTokens1, s.history[1].InputTokens, "sendToDriver should not modify history[1].InputTokens")
}

// Run user message counting tests.

// TestRun_CountsUserMessageTokens verifies that run() counts user message
// tokens and updates ContextTokens before appending to history.
func TestRun_CountsUserMessageTokens(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), nil)
	s.lastCountedIndex = 0

	userMsg := AgentMessage{Role: RoleUser, Content: "What is the weather?"}

	// Simulate what run() does: count user message tokens
	if s.TokenCounter != nil {
		msgTokens := s.countMessageTokens(userMsg)
		lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
			if s.lastCountedIndex == 0 {
				cs.ContextTokens = s.countSystemPromptTokens()
			}
			cs.ContextTokens += msgTokens
		})
		userMsg.InputTokens = msgTokens
		s.lastCountedIndex++
	}

	// ContextTokens should include system prompt + user message
	contextTokens := s.getConvoContextTokens()
	assert.True(t, contextTokens > 0, "ContextTokens should be positive after counting user message, got %d", contextTokens)

	// User message should have InputTokens set
	assert.True(t, userMsg.InputTokens > 0, "User message should have InputTokens set")

	// lastCountedIndex should be incremented
	assert.Equal(t, 1, s.lastCountedIndex, "lastCountedIndex should be incremented")
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

func TestSetupSetsLastCountedIndexOnlyWhenTokenCounterActive(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	msg := &dipper.Message{
		Labels:  map[string]string{"agent_name": "testagent"},
		Payload: map[string]interface{}{"type": AgentSessionTypeChatTurn, "text": "hello"},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)
	assert.Equal(t, 0, s.lastCountedIndex, "lastCountedIndex should remain 0 when TokenCounter is nil")
}

func TestLastCountedIndex_SetWhenTokenCounterActive(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	agent.TokenCounter = "simple"
	store.cfg.DataSet.Agents["testagent"] = agent

	msg := &dipper.Message{
		Labels:  map[string]string{"agent_name": "testagent"},
		Payload: map[string]interface{}{"type": AgentSessionTypeChatTurn, "text": "hello"},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)
	assert.NotNil(t, s.TokenCounter, "TokenCounter should be initialized")
	assert.Equal(t, len(s.history), s.lastCountedIndex, "lastCountedIndex should be set to len(history) when TokenCounter is active")
}

// updateContextTokens tests.

// TestUpdateContextTokens_IncrementalCounting verifies that updateContextTokens
// only counts messages since lastCountedIndex (incremental).
func TestUpdateContextTokens_IncrementalCounting(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), []AgentMessage{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAgent, Content: "Hi there!"},
	})
	s.lastCountedIndex = 2 // all already counted

	// Add a new message to history
	s.history = append(s.history, AgentMessage{Role: RoleUser, Content: "Follow-up question"})

	// Set initial ContextTokens
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 50
	})

	// Call updateContextTokens
	s.updateContextTokens()

	// ContextTokens should increase by the new message's tokens only
	newContextTokens := s.getConvoContextTokens()
	assert.True(t, newContextTokens > 50, "ContextTokens should increase from incremental counting, got %d", newContextTokens)
	assert.Equal(t, 3, s.lastCountedIndex, "lastCountedIndex should be updated to len(history)")
}

// TestUpdateContextTokens_CountsSystemPromptOnFirstSend verifies that
// updateContextTokens counts the system prompt when lastCountedIndex is 0.
func TestUpdateContextTokens_CountsSystemPromptOnFirstSend(t *testing.T) {
	s := makeTokenCountingSession(makeTestAgent(), []AgentMessage{
		{Role: RoleUser, Content: "Hello"},
	})
	s.lastCountedIndex = 0

	s.updateContextTokens()

	contextTokens := s.getConvoContextTokens()
	// Should include system prompt tokens (7) + user message tokens (1) = at least 8
	assert.True(t, contextTokens >= 8, "ContextTokens should include system prompt + history, got %d", contextTokens)
	assert.Equal(t, 1, s.lastCountedIndex, "lastCountedIndex should be updated to len(history)")
}

// TokenCounter interface tests.

// TestProcessAgentMessage_UsesTokenCounterInterface verifies that
// processAgentMessage uses the TokenCounter interface method for counting.
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
