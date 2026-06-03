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
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadPreContext_HistoryNotEmpty tests that loadPreContext returns false
// when history is not empty.
func TestLoadPreContext_HistoryNotEmpty(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-1",
		Agent: &config.Agent{FileTool: "read_files", PreContext: []string{"file1.md"}},
		history: []AgentMessage{
			{Role: RoleUser, Content: "existing message"},
		},
		store: store,
	}

	result := session.loadPreContext()

	assert.False(t, result, "loadPreContext should return false when history is not empty")
	assert.Empty(t, session.ToolCalls, "no tool calls should be created")
}

// TestLoadPreContext_NoFileTool tests that loadPreContext returns false
// when FileTool is not configured.
func TestLoadPreContext_NoFileTool(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:      "test-session-2",
		Agent:   &config.Agent{FileTool: "", PreContext: []string{"file1.md"}},
		history: []AgentMessage{},
		store:   store,
	}

	result := session.loadPreContext()

	assert.False(t, result, "loadPreContext should return false when FileTool is empty")
	assert.Empty(t, session.ToolCalls, "no tool calls should be created")
}

// TestLoadPreContext_NoPreContext tests that loadPreContext returns false
// when PreContext is not configured.
func TestLoadPreContext_NoPreContext(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:      "test-session-3",
		Agent:   &config.Agent{FileTool: "read_files", PreContext: []string{}},
		history: []AgentMessage{},
		store:   store,
	}

	result := session.loadPreContext()

	assert.False(t, result, "loadPreContext should return false when PreContext is empty")
	assert.Empty(t, session.ToolCalls, "no tool calls should be created")
}

// TestLoadPreContext_WithEmptyStringsInPreContext tests that loadPreContext
// filters out empty strings from PreContext.
func TestLoadPreContext_WithEmptyStringsInPreContext(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:      "test-session-4",
		Agent:   &config.Agent{FileTool: "read_files", PreContext: []string{"", "", ""}},
		history: []AgentMessage{},
		store:   store,
	}

	result := session.loadPreContext()

	assert.False(t, result, "loadPreContext should return false when all PreContext entries are empty")
	assert.Empty(t, session.ToolCalls, "no tool calls should be created")
}

// TestLoadPreContext_Success tests that loadPreContext correctly creates
// a tool call when conditions are met.
func TestLoadPreContext_Success(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:      "test-session-5",
		Agent:   &config.Agent{FileTool: "wf__read_files", PreContext: []string{"file1.md", "file2.md"}},
		history: []AgentMessage{},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{},
		},
		store: store,
	}

	result := session.loadPreContext()

	assert.True(t, result, "loadPreContext should return true when valid config is provided")
	require.Len(t, session.ToolCalls, 1, "exactly one tool call should be created")

	toolCall := session.ToolCalls[0]
	assert.Equal(t, "wf__read_files", toolCall.FuncName)
	assert.Equal(t, []string{"file1.md", "file2.md"}, toolCall.Params["files"])
	assert.True(t, toolCall.Params["pre_context"].(bool), "pre_context param should be true")

	// Check history was NOT updated
	assert.Empty(t, session.history)
	assert.Len(t, session.ToolCalls, 1)
}

// TestLoadPreContext_MixedEmptyAndNonEmptyPreContext tests that loadPreContext
// correctly filters empty strings and processes valid entries.
func TestLoadPreContext_MixedEmptyAndNonEmptyPreContext(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:      "test-session-6",
		Agent:   &config.Agent{FileTool: "wf__read_files", PreContext: []string{"", "file1.md", "", "file2.md"}},
		history: []AgentMessage{},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello",
			},
		},
		store: store,
	}

	result := session.loadPreContext()

	assert.True(t, result, "loadPreContext should return true when at least one non-empty PreContext entry exists")
	require.Len(t, session.ToolCalls, 1)

	toolCall := session.ToolCalls[0]
	assert.Equal(t, "wf__read_files", toolCall.FuncName)
	assert.Equal(t, []string{"file1.md", "file2.md"}, toolCall.Params["files"])
}

// TestHandlePreContextResult_NotPreContext tests that handlePreContextResult
// returns false when the tool call is not a pre_context result.
func TestHandlePreContextResult_NotPreContext(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-7",
		Agent: &config.Agent{},
		store: store,
	}

	toolCall := AgentToolCall{
		FuncName: "some_tool",
		Params: map[string]interface{}{
			"pre_context": false,
		},
	}
	results := []map[string]interface{}{}

	result := session.handlePreContextResult(toolCall, results)

	assert.False(t, result, "handlePreContextResult should return false when pre_context is false")
	assert.Empty(t, session.history, "history should not be modified")
}

// TestHandlePreContextResult_MissingPreContextParam tests that handlePreContextResult
// returns false when pre_context param is missing.
func TestHandlePreContextResult_MissingPreContextParam(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-8",
		Agent: &config.Agent{},
		store: store,
	}

	toolCall := AgentToolCall{
		FuncName: "some_tool",
		Params:   map[string]interface{}{},
	}
	results := []map[string]interface{}{}

	result := session.handlePreContextResult(toolCall, results)

	assert.False(t, result, "handlePreContextResult should return false when pre_context param is missing")
	assert.Empty(t, session.history, "history should not be modified")
}

// TestHandlePreContextResult_EmptyResults tests that handlePreContextResult
// handles empty results gracefully.
func TestHandlePreContextResult_EmptyResults(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-9",
		Agent: &config.Agent{SystemPrompt: "You are helpful"},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello",
			},
		},
		store: store,
	}

	toolCall := AgentToolCall{
		FuncName: "read_files",
		Params: map[string]interface{}{
			"pre_context": true,
		},
	}
	results := []map[string]interface{}{}

	result := session.handlePreContextResult(toolCall, results)

	assert.True(t, result, "handlePreContextResult should return true for pre_context results")
	// After handling pre-context result with no content, run() is called which adds user message
	assert.Len(t, session.history, 1, "only user message should be added when pre-context result is empty")
	assert.Equal(t, RoleUser, session.history[0].Role)
}

// TestHandlePreContextResult_MissingFileContent tests that handlePreContextResult
// handles missing file_content gracefully.
func TestHandlePreContextResult_MissingFileContent(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-10",
		Agent: &config.Agent{SystemPrompt: "You are helpful"},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello",
			},
		},
		store: store,
	}

	toolCall := AgentToolCall{
		FuncName: "read_files",
		Params: map[string]interface{}{
			"pre_context": true,
		},
	}
	results := []map[string]interface{}{
		{"other_field": "value"},
	}

	result := session.handlePreContextResult(toolCall, results)

	assert.True(t, result, "handlePreContextResult should return true for pre_context results")
	// run() is called which adds user message
	assert.Len(t, session.history, 1)
	assert.Equal(t, RoleUser, session.history[0].Role)
}

// TestHandlePreContextResult_WithFileContent tests that handlePreContextResult
// correctly processes file content and adds it to history.
func TestHandlePreContextResult_WithFileContent(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-11",
		Agent: &config.Agent{SystemPrompt: "You are helpful"},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello",
			},
		},
		store: store,
	}

	toolCall := AgentToolCall{
		FuncName: "read_files",
		Params: map[string]interface{}{
			"pre_context": true,
		},
	}
	results := []map[string]interface{}{
		{
			"status": "success",
			"data": map[string]interface{}{
				"file_content": map[string]interface{}{
					"file1.md": "# File 1 Content\nThis is the content of file 1.",
					"file2.md": "# File 2 Content\nThis is the content of file 2.",
				},
			},
		},
	}

	result := session.handlePreContextResult(toolCall, results)

	assert.True(t, result, "handlePreContextResult should return true")
	assert.Greater(t, len(session.history), 0, "history should be empty")

	// Find the pre-context message in system message.
	assert.Contains(t, session.Agent.SystemPrompt, "# File 1 Content")
	assert.Contains(t, session.Agent.SystemPrompt, "# File 2 Content")
	assert.Empty(t, session.ToolCalls, "ToolCalls should be reset to empty")
	assert.Empty(t, session.ToolResults, "ToolResults should be reset to empty")
}

// TestHandlePreContextResult_SingleFile tests that handlePreContextResult
// correctly handles a single file.
func TestHandlePreContextResult_SingleFile(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-12",
		Agent: &config.Agent{SystemPrompt: "You are helpful"},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello",
			},
		},
		store: store,
	}

	toolCall := AgentToolCall{
		FuncName: "read_files",
		Params: map[string]interface{}{
			"pre_context": true,
		},
	}
	results := []map[string]interface{}{
		{
			"status": "success",
			"data": map[string]interface{}{
				"file_content": map[string]interface{}{
					"README.md": "# Project README\n\nThis is a test project.",
				},
			},
		},
	}

	result := session.handlePreContextResult(toolCall, results)

	assert.True(t, result)
	assert.Greater(t, len(session.history), 0)

	assert.Contains(t, session.Agent.SystemPrompt, PreContextHeader)
	assert.Contains(t, session.Agent.SystemPrompt, "# Project README")
}

// TestHandlePreContextResult_ResetSessionState tests that handlePreContextResult
// correctly resets session state after handling pre-context.
func TestHandlePreContextResult_ResetSessionState(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:          "test-session-13",
		Agent:       &config.Agent{SystemPrompt: "You are helpful"},
		CurrentCall: 5,
		ToolCalls:   []AgentToolCall{{FuncName: "some_tool"}},
		ToolResults: []map[string]interface{}{{"result": "data"}},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello",
			},
		},
		store: store,
	}

	toolCall := AgentToolCall{
		FuncName: "read_files",
		Params: map[string]interface{}{
			"pre_context": true,
		},
	}
	results := []map[string]interface{}{
		{
			"file_content": map[string]interface{}{
				"file.md": "content",
			},
		},
	}

	result := session.handlePreContextResult(toolCall, results)

	assert.True(t, result)
	assert.Equal(t, 0, session.CurrentCall, "CurrentCall should be reset to 0")
	assert.Nil(t, session.ToolCalls, "ToolCalls should be reset to nil")
	assert.Nil(t, session.ToolResults, "ToolResults should be reset to nil")
}

// TestHandlePreContextResult_PreContextHeaderPresent tests that the pre-context
// message always includes the expected header.
func TestHandlePreContextResult_PreContextHeaderPresent(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-14",
		Agent: &config.Agent{SystemPrompt: "You are helpful"},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello",
			},
		},
		store: store,
	}

	toolCall := AgentToolCall{
		FuncName: "read_files",
		Params: map[string]interface{}{
			"pre_context": true,
		},
	}
	results := []map[string]interface{}{
		{
			"data": map[string]interface{}{
				"file_content": map[string]interface{}{
					"doc.md": "Some documentation",
				},
			},
		},
	}

	result := session.handlePreContextResult(toolCall, results)

	assert.True(t, result)
	assert.Greater(t, len(session.history), 0)

	// Find the pre-context message in history
	assert.True(t, strings.HasSuffix(strings.TrimSuffix(session.Agent.SystemPrompt, "Some documentation\n\n"), PreContextHeader))
}

// TestHandlePreContextResult_FileContentNotMap tests that handlePreContextResult
// gracefully handles when file_content is not a map.
func TestHandlePreContextResult_FileContentNotMap(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-15",
		Agent: &config.Agent{SystemPrompt: "You are helpful"},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello",
			},
		},
		store: store,
	}

	toolCall := AgentToolCall{
		FuncName: "read_files",
		Params: map[string]interface{}{
			"pre_context": true,
		},
	}
	results := []map[string]interface{}{
		{
			"file_content": "not a map",
		},
	}

	result := session.handlePreContextResult(toolCall, results)

	assert.True(t, result, "handlePreContextResult should return true for pre_context results")
	// run() is called which adds user message
	assert.Len(t, session.history, 1)
	assert.Equal(t, RoleUser, session.history[0].Role)
}

// TestPreContextWorkflow tests the complete flow of loading and handling
// pre-context.
func TestPreContextWorkflow(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:      "test-session-workflow",
		Agent:   &config.Agent{FileTool: "wf__read_files", PreContext: []string{"setup.md", "guide.md"}, SystemPrompt: "You are helpful"},
		history: []AgentMessage{},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello",
			},
		},
		store: store,
	}

	// Step 1: Load pre-context
	loadResult := session.loadPreContext()
	assert.True(t, loadResult)
	require.Len(t, session.ToolCalls, 1)
	assert.Len(t, session.history, 0, "history should not be modified by loadPreContext")

	// Step 2: Handle pre-context result
	toolCall := session.ToolCalls[0]
	results := []map[string]interface{}{
		{
			"data": map[string]interface{}{
				"file_content": map[string]interface{}{
					"setup.md": "# Setup Instructions\nFollow these steps...",
					"guide.md": "# User Guide\nHow to use...",
				},
			},
		},
	}

	handleResult := session.handlePreContextResult(toolCall, results)
	assert.True(t, handleResult)

	assert.Contains(t, session.Agent.SystemPrompt, "# Setup Instructions")
	assert.Contains(t, session.Agent.SystemPrompt, "# User Guide")
}
