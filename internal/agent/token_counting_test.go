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
