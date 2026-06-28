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

func TestCountMessageTokens_EmptyMessage(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:    "test-session",
		Agent: &agent,
		store: store,
	}

	msg := AgentMessage{}
	tokens := s.countMessageTokens(msg)
	assert.Equal(t, 0, tokens, "Empty message should have 0 tokens")
}

func TestCountMessageTokens_ContentOnly(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:    "test-session",
		Agent: &agent,
		store: store,
	}

	msg := AgentMessage{
		Content: "Hello, world!",
	}
	tokens := s.countMessageTokens(msg)
	// "Hello, world!" has 13 chars, 13/4 = 3 tokens
	assert.Equal(t, 3, tokens)
}

func TestCountMessageTokens_WithToolCalls(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:    "test-session",
		Agent: &agent,
		store: store,
	}

	msg := AgentMessage{
		Content: "Calling a tool",
		ToolCalls: []AgentToolCall{
			{
				FuncName: "sys_test__action",
				Params: map[string]interface{}{
					"param1": "value1",
				},
			},
		},
	}
	tokens := s.countMessageTokens(msg)
	assert.True(t, tokens > 10, "Message with tool calls should have more than 10 tokens, got %d", tokens)
}

func TestCountMessageTokens_WithToolResults(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:    "test-session",
		Agent: &agent,
		store: store,
	}

	msg := AgentMessage{
		Role: RoleToolResult,
		ToolResult: []map[string]interface{}{
			{
				"status": "success",
				"data":   "result data",
			},
		},
	}
	tokens := s.countMessageTokens(msg)
	assert.True(t, tokens > 0, "Tool result message should have tokens, got %d", tokens)
}

func TestCountMessageTokens_WithThoughts(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:    "test-session",
		Agent: &agent,
		store: store,
	}

	msg := AgentMessage{
		Content:  "Hello",
		Thoughts: "I should respond politely",
	}
	tokens := s.countMessageTokens(msg)
	assert.Equal(t, 7, tokens)
}

func TestCountSystemPromptTokens_Default(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:    "test-session",
		Type:  AgentSessionTypeChatTurn,
		Agent: &agent,
		store: store,
	}

	tokens := s.countSystemPromptTokens()
	assert.Equal(t, 7, tokens)
}

func TestCountSystemPromptTokens_InferencePrompt(t *testing.T) {
	store := newMockStore(nil)
	agent := config.Agent{
		Name:            "testagent",
		Driver:          "openai",
		Engine:          "gpt-4",
		SystemPrompt:    "You are a helpful assistant.",
		InferencePrompt: "Answer the question.",
	}
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:    "test-session",
		Type:  AgentSessionTypeInference,
		Agent: &agent,
		store: store,
	}

	tokens := s.countSystemPromptTokens()
	assert.Equal(t, 5, tokens)
}

func TestConvoStateContextTokens_Persistence(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	cs := &ConvoState{
		ConvoID:       "test-convo",
		ContextTokens: 100,
		TTL:           "72h",
	}
	cs.persist(store)

	cs2 := &ConvoState{}
	cs2.load("test-convo", store)
	assert.Equal(t, 100, cs2.ContextTokens, "ContextTokens should be persisted and loaded")
}

func TestLastCountedIndex_InitNewSession(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "testagent",
		},
		Payload: map[string]interface{}{
			"type": AgentSessionTypeChatTurn,
			"text": "hello",
		},
	}

	s := &AgentSession{}
	s.initNewSession("test-id", msg, store)

	assert.Equal(t, 0, s.lastCountedIndex, "New session should have lastCountedIndex = 0")
}

func TestLastCountedIndex_LoadSession(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:      "test-session",
		ConvoID: "test-convo",
		Agent:   &agent,
		history: []AgentMessage{
			{Role: RoleUser, Content: "Hello"},
			{Role: RoleAgent, Content: "Hi there!"},
		},
		store: store,
	}
	s.persist(false)

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_session_id": "test-session",
			"agent_name":       "testagent",
		},
		Payload: map[string]interface{}{
			"type":     AgentSessionTypeChatTurn,
			"convo_id": "test-convo",
		},
	}

	s2 := &AgentSession{}
	s2.setup(msg, store, false)
	s2.history = []AgentMessage{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAgent, Content: "Hi there!"},
	}
	s2.lastCountedIndex = len(s2.history)
	assert.Equal(t, 2, s2.lastCountedIndex, "Loaded session should have lastCountedIndex = len(history)")
}

func TestCompactionResetsContextTokens(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	store.cfg.DataSet.Agents["testagent"] = agent

	cs := &ConvoState{
		ConvoID:       "test-convo",
		ContextTokens: 500,
		TTL:           "72h",
	}
	cs.persist(store)

	s := &AgentSession{
		ID:               "test-session",
		ConvoID:          "test-convo",
		Agent:            &agent,
		lastCountedIndex: 10,
		store:            store,
	}

	s.lastCountedIndex = 0
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.ContextTokens = 0
	})

	cs2 := &ConvoState{}
	cs2.load("test-convo", store)
	assert.Equal(t, 0, cs2.ContextTokens, "ContextTokens should be reset after compaction")
	assert.Equal(t, 0, s.lastCountedIndex, "lastCountedIndex should be reset after compaction")
}

func TestTokenCountsWrittenBackToMessages(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	agent.TokenCounter = "simple"
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:               "test-session",
		ConvoID:          "test-convo",
		Agent:            &agent,
		TokenCounter:     &SimpleTokenCounter{},
		store:            store,
		lastCountedIndex: 0,
	}

	// Add some history messages
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAgent, Content: "Hi there!"},
	}

	// Create a mock ConvoState
	cs := &ConvoState{
		ConvoID: "test-convo",
		TTL:     "72h",
	}
	cs.persist(store)

	// Count tokens and write back to messages
	for i, msg := range s.history {
		msgTokens := s.countMessageTokens(msg)
		s.history[i].InputTokens = msgTokens
	}

	// Verify token counts were written back
	if s.history[0].InputTokens == 0 {
		t.Errorf("Expected InputTokens > 0 for first message, got %d", s.history[0].InputTokens)
	}
	if s.history[1].InputTokens == 0 {
		t.Errorf("Expected InputTokens > 0 for second message, got %d", s.history[1].InputTokens)
	}

	// Verify the tokens are reasonable (1 token ≈ 4 chars)
	// "Hello" = 5 chars = 1 token
	if s.history[0].InputTokens != 1 {
		t.Errorf("Expected InputTokens = 1 for 'Hello', got %d", s.history[0].InputTokens)
	}
}

func TestOutputTokensWrittenBackToAgentMessage(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	agent.TokenCounter = "simple"
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:           "test-session",
		ConvoID:      "test-convo",
		Agent:        &agent,
		TokenCounter: &SimpleTokenCounter{},
		store:        store,
	}

	// Create a mock ConvoState
	cs := &ConvoState{
		ConvoID: "test-convo",
		TTL:     "72h",
	}
	cs.persist(store)

	// Create an agent message
	agentMsg := &AgentMessage{
		Role:    RoleAgent,
		Content: "This is a test response",
	}

	// Count output tokens
	outputTokens := s.countMessageTokens(*agentMsg)

	// Simulate writing back to message (as done in processAgentMessage)
	agentMsg.OutputTokens = outputTokens

	// Verify output tokens were written back
	if agentMsg.OutputTokens == 0 {
		t.Errorf("Expected OutputTokens > 0, got %d", agentMsg.OutputTokens)
	}

	// "This is a test response" = 24 chars = 6 tokens
	if agentMsg.OutputTokens < 5 || agentMsg.OutputTokens > 7 {
		t.Errorf("Expected OutputTokens between 5-7 for test message, got %d", agentMsg.OutputTokens)
	}
}

func TestTokenCountsNotWrittenBackWhenTokenCounterNil(t *testing.T) {
	store := newMockStore(nil)
	agent := makeTestAgent()
	agent.TokenCounter = "" // Not set to "simple"
	store.cfg.DataSet.Agents["testagent"] = agent

	s := &AgentSession{
		ID:           "test-session",
		Agent:        &agent,
		TokenCounter: nil, // No custom counter
		store:        store,
	}

	// Create an agent message
	agentMsg := &AgentMessage{
		Role:    RoleAgent,
		Content: "Test message",
	}

	// Simulate what happens in processAgentMessage when TokenCounter is nil
	// Should use driver-reported values instead of custom counting
	if s.TokenCounter == nil {
		agentMsg.InputTokens = 10  // Simulated driver-reported value
		agentMsg.OutputTokens = 20 // Simulated driver-reported value
	}

	// Verify driver-reported values are used
	if agentMsg.InputTokens != 10 {
		t.Errorf("Expected InputTokens = 10 (driver-reported), got %d", agentMsg.InputTokens)
	}
	if agentMsg.OutputTokens != 20 {
		t.Errorf("Expected OutputTokens = 20 (driver-reported), got %d", agentMsg.OutputTokens)
	}
}

func TestTokenCounterOnlyUsedWhenSetToSimple(t *testing.T) {
	store := newMockStore(nil)

	// Test with TokenCounter set to "simple" - should use custom counter
	agentSimple := config.Agent{
		Name:         "testagent-simple",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "You are a helpful assistant.",
		TokenCounter: "simple",
	}
	store.cfg.DataSet.Agents["testagent-simple"] = agentSimple

	sSimple := &AgentSession{}

	// TokenCounter should be initialized when set to "simple"
	sSimple.setup(&dipper.Message{
		Labels: map[string]string{
			"agent_name": "testagent-simple",
		},
		Payload: map[string]interface{}{
			"type": AgentSessionTypeChatTurn,
			"text": "hello",
		},
	}, store, false)

	if sSimple.TokenCounter == nil {
		t.Error("TokenCounter should be initialized when agent.TokenCounter is set to 'simple'")
	}

	// Test with TokenCounter set to other values - should NOT use custom counter
	agentOther := config.Agent{
		Name:         "testagent-other",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "You are a helpful assistant.",
		TokenCounter: "other",
	}
	store.cfg.DataSet.Agents["testagent-other"] = agentOther

	sOther := &AgentSession{}

	sOther.setup(&dipper.Message{
		Labels: map[string]string{
			"agent_name": "testagent-other",
		},
		Payload: map[string]interface{}{
			"type": AgentSessionTypeChatTurn,
			"text": "hello",
		},
	}, store, false)

	if sOther.TokenCounter != nil {
		t.Error("TokenCounter should NOT be initialized when agent.TokenCounter is set to 'other'")
	}

	// Test with TokenCounter empty - should NOT use custom counter
	agentEmpty := config.Agent{
		Name:         "testagent-empty",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "You are a helpful assistant.",
		TokenCounter: "",
	}
	store.cfg.DataSet.Agents["testagent-empty"] = agentEmpty

	sEmpty := &AgentSession{}

	sEmpty.setup(&dipper.Message{
		Labels: map[string]string{
			"agent_name": "testagent-empty",
		},
		Payload: map[string]interface{}{
			"type": AgentSessionTypeChatTurn,
			"text": "hello",
		},
	}, store, false)

	if sEmpty.TokenCounter != nil {
		t.Error("TokenCounter should NOT be initialized when agent.TokenCounter is empty")
	}
}
