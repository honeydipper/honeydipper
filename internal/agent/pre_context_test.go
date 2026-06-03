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

	result := session.loadPreContextAndSkills()

	assert.False(t, result, "loadPreContextAndSkills should return false when history is not empty")
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

	result := session.loadPreContextAndSkills()

	assert.False(t, result, "loadPreContextAndSkills should return false when FileTool is empty")
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

	result := session.loadPreContextAndSkills()

	assert.False(t, result, "loadPreContextAndSkills should return false when PreContext is empty")
	assert.Empty(t, session.ToolCalls, "no tool calls should be created")
}

// TestLoadPreContext_WithEmptyStringsInPreContext tests that loadPreContextAndSkills
// filters out empty strings from PreContext.
func TestLoadPreContext_WithEmptyStringsInPreContext(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:      "test-session-4",
		Agent:   &config.Agent{FileTool: "read_files", PreContext: []string{"", "", ""}},
		history: []AgentMessage{},
		store:   store,
	}

	result := session.loadPreContextAndSkills()

	assert.False(t, result, "loadPreContextAndSkills should return false when all PreContext entries are empty")
	assert.Empty(t, session.ToolCalls, "no tool calls should be created")
}

// TestLoadPreContext_Success tests that loadPreContextAndSkills correctly creates
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

	result := session.loadPreContextAndSkills()

	assert.True(t, result, "loadPreContextAndSkills should return true when valid config is provided")
	require.Len(t, session.ToolCalls, 1, "exactly one tool call should be created")

	toolCall := session.ToolCalls[0]
	assert.Equal(t, "wf__read_files", toolCall.FuncName)
	assert.Equal(t, []string{"file1.md", "file2.md"}, toolCall.Params["file_specs"])
	assert.True(t, toolCall.Params["pre_context"].(bool), "pre_context param should be true")

	// Check history was NOT updated
	assert.Empty(t, session.history)
	assert.Len(t, session.ToolCalls, 1)
}

// TestLoadPreContext_MixedEmptyAndNonEmptyPreContext tests that loadPreContextAndSkills
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

	result := session.loadPreContextAndSkills()

	assert.True(t, result, "loadPreContextAndSkills should return true when at least one non-empty PreContext entry exists")
	require.Len(t, session.ToolCalls, 1)

	toolCall := session.ToolCalls[0]
	assert.Equal(t, "wf__read_files", toolCall.FuncName)
	assert.Equal(t, []string{"file1.md", "file2.md"}, toolCall.Params["file_specs"])
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

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.False(t, result, "handlePreContextAndSkillsResult should return false when pre_context is false")
	assert.Empty(t, session.history, "history should not be modified")
}

// TestHandlePreContextAndSkillsResult_MissingPreContextParam tests that handlePreContextAndSkillsResult
// returns false when pre_context param is missing.
func TestHandlePreContextAndSkillsResult_MissingPreContextParam(t *testing.T) {
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

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.False(t, result, "handlePreContextAndSkillsResult should return false when pre_context param is missing")
	assert.Empty(t, session.history, "history should not be modified")
}

// TestHandlePreContextAndSkillsResult_EmptyResults tests that handlePreContextAndSkillsResult
// handles empty results gracefully.
func TestHandlePreContextAndSkillsResult_EmptyResults(t *testing.T) {
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

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.True(t, result, "handlePreContextAndSkillsResult should return true for pre_context results")
	// After handling pre-context result with no content, run() is called which adds user message
	assert.Len(t, session.history, 1, "only user message should be added when pre-context result is empty")
	assert.Equal(t, RoleUser, session.history[0].Role)
}

// TestHandlePreContextAndSkillsResult_MissingFileContent tests that handlePreContextAndSkillsResult
// handles missing file_content gracefully.
func TestHandlePreContextAndSkillsResult_MissingFileContent(t *testing.T) {
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

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.True(t, result, "handlePreContextAndSkillsResult should return true for pre_context results")
	// run() is called which adds user message
	assert.Len(t, session.history, 1)
	assert.Equal(t, RoleUser, session.history[0].Role)
}

// TestHandlePreContextAndSkillsResult_WithFileContent tests that handlePreContextAndSkillsResult
// correctly processes file content and adds it to history.
func TestHandlePreContextAndSkillsResult_WithFileContent(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID: "test-session-11",
		Agent: &config.Agent{
			SystemPrompt: "You are helpful",
			PreContext:   []string{"file1.md", "file2.md"},
		},
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
				"files": []interface{}{
					map[string]interface{}{"file_spec": "file1.md", "file_content": "# File 1 Content\nThis is the content of file 1."},
					map[string]interface{}{"file_spec": "file2.md", "file_content": "# File 2 Content\nThis is the content of file 2."},
				},
			},
		},
	}

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.True(t, result, "handlePreContextAndSkillsResult should return true")
	assert.Greater(t, len(session.history), 0, "history should be empty")

	// Find the pre-context message in system message.
	assert.Contains(t, session.Agent.SystemPrompt, "# File 1 Content")
	assert.Contains(t, session.Agent.SystemPrompt, "# File 2 Content")
	assert.Empty(t, session.ToolCalls, "ToolCalls should be reset to empty")
	assert.Empty(t, session.ToolResults, "ToolResults should be reset to empty")
}

// TestHandlePreContextAndSkillsResult_SingleFile tests that handlePreContextAndSkillsResult
// correctly handles a single file.
func TestHandlePreContextAndSkillsResult_SingleFile(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-12",
		Agent: &config.Agent{SystemPrompt: "You are helpful", PreContext: []string{"README.md"}},
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
				"files": []interface{}{
					map[string]interface{}{"file_spec": "README.md", "file_content": "# Project README\n\nThis is a test project."},
				},
			},
		},
	}

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.True(t, result)
	assert.Equal(t, 1, len(session.history), "should send the message to driver")

	assert.Contains(t, session.Agent.SystemPrompt, PreContextHeader)
	assert.Contains(t, session.Agent.SystemPrompt, "# Project README")
}

// TestHandlePreContextAndSkillsResult_ResetSessionState tests that handlePreContextAndSkillsResult
// correctly resets session state after handling pre-context.
func TestHandlePreContextAndSkillsResult_ResetSessionState(t *testing.T) {
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

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.True(t, result)
	assert.Equal(t, 0, session.CurrentCall, "CurrentCall should be reset to 0")
	assert.Nil(t, session.ToolCalls, "ToolCalls should be reset to nil")
	assert.Nil(t, session.ToolResults, "ToolResults should be reset to nil")
}

// TestHandlePreContextAndSkillsResult_PreContextHeaderPresent tests that the pre-context
// message always includes the expected header.
func TestHandlePreContextAndSkillsResult_PreContextHeaderPresent(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID:    "test-session-14",
		Agent: &config.Agent{SystemPrompt: "You are helpful", PreContext: []string{"doc.md"}},
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
				"files": []interface{}{
					map[string]interface{}{"file_spec": "doc.md", "file_content": "Some documentation"},
				},
			},
		},
	}

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.True(t, result)
	assert.Greater(t, len(session.history), 0)

	// Find the pre-context message in history
	assert.True(t, strings.HasSuffix(strings.TrimSuffix(session.Agent.SystemPrompt, "Some documentation\n\n"), PreContextHeader))
}

// TestHandlePreContextAndSkillsResult_FileContentNotMap tests that handlePreContextAndSkillsResult
// gracefully handles when file_content is not a map.
func TestHandlePreContextAndSkillsResult_FileContentNotMap(t *testing.T) {
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

	result := session.handlePreContextAndSkillsResult(toolCall, results)

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
	loadResult := session.loadPreContextAndSkills()
	assert.True(t, loadResult)
	require.Len(t, session.ToolCalls, 1)
	assert.Len(t, session.history, 0, "history should not be modified by loadPreContextAndSkills")

	// Step 2: Handle pre-context result
	toolCall := session.ToolCalls[0]
	results := []map[string]interface{}{
		{
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"file_spec": "setup.md", "file_content": "# Setup Instructions\nFollow these steps..."},
					map[string]interface{}{"file_spec": "guide.md", "file_content": "# User Guide\nHow to use..."},
				},
			},
		},
	}

	handleResult := session.handlePreContextAndSkillsResult(toolCall, results)
	assert.True(t, handleResult)

	assert.Contains(t, session.Agent.SystemPrompt, "# Setup Instructions")
	assert.Contains(t, session.Agent.SystemPrompt, "# User Guide")
}

// TestLoadPreContextAndSkills_WithSkills tests that loadPreContextAndSkills
// correctly loads both PreContext and SkillsPaths.
func TestLoadPreContextAndSkills_WithSkills(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID: "test-session-skills",
		Agent: &config.Agent{
			FileTool:    "wf__read_files",
			PreContext:  []string{"README.md"},
			SkillsPaths: []string{"skills/skill1/SKILL.md", "skills/skill2/SKILL.md"},
		},
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

	result := session.loadPreContextAndSkills()

	assert.True(t, result, "loadPreContextAndSkills should return true when skills and precontext are configured")
	require.Len(t, session.ToolCalls, 1, "exactly one tool call should be created")

	toolCall := session.ToolCalls[0]
	assert.Equal(t, "wf__read_files", toolCall.FuncName)

	// Verify that both precontext and skills are included in file_specs
	fileSpecs := toolCall.Params["file_specs"].([]string)
	assert.Len(t, fileSpecs, 3, "should load 1 precontext file and 2 skill files")
	assert.Contains(t, fileSpecs, "README.md")
	assert.Contains(t, fileSpecs, "skills/skill1/SKILL.md")
	assert.Contains(t, fileSpecs, "skills/skill2/SKILL.md")
	assert.True(t, toolCall.Params["pre_context"].(bool), "pre_context param should be true")
}

// TestLoadPreContextAndSkills_DuplicateFiles tests that loadPreContextAndSkills
// deduplicates files when both PreContext and SkillsPaths contain the same file.
func TestLoadPreContextAndSkills_DuplicateFiles(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID: "test-session-dup",
		Agent: &config.Agent{
			FileTool:    "wf__read_files",
			PreContext:  []string{"docs/guide.md", "docs/setup.md"},
			SkillsPaths: []string{"docs/guide.md", "docs/skill1/SKILL.md"},
		},
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

	result := session.loadPreContextAndSkills()

	assert.True(t, result)
	require.Len(t, session.ToolCalls, 1)

	toolCall := session.ToolCalls[0]
	fileSpecs := toolCall.Params["file_specs"].([]string)

	// Should only have 3 files (no duplicates):
	// docs/guide.md, docs/setup.md, docs/skill1/SKILL.md
	assert.Len(t, fileSpecs, 3, "duplicates should be removed")
	assert.Contains(t, fileSpecs, "docs/guide.md")
	assert.Contains(t, fileSpecs, "docs/setup.md")
	assert.Contains(t, fileSpecs, "docs/skill1/SKILL.md")
}

// TestHandlePreContextAndSkillsResult_WithSkillsAndPreContext tests that
// handlePreContextAndSkillsResult correctly processes both precontext and skill files.
func TestHandlePreContextAndSkillsResult_WithSkillsAndPreContext(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID: "test-session-combined",
		Agent: &config.Agent{
			SystemPrompt: "You are a helpful assistant",
			PreContext:   []string{"README.md"},
		},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-1",
			},
			Payload: map[string]interface{}{
				"text": "hello world",
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

	// Construct results with precontext file and skill files with YAML frontmatter
	results := []map[string]interface{}{
		{
			"status": "success",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{
						"file_spec":    "README.md",
						"file_content": "# Project README\n\nThis is the main project documentation.",
					},
					map[string]interface{}{
						"file_spec": "skills/debug/SKILL.md",
						"file_content": `---
name: debug-tool
description: Helps debug and troubleshoot issues
---
# Debug Tool Skill

Use this skill to analyze logs and debug issues.`,
					},
					map[string]interface{}{
						"file_spec": "skills/analyze/SKILL.md",
						"file_content": `---
name: analyze-code
description: Analyzes code and provides insights
---
# Code Analysis Skill

Use this skill to understand code structure and complexity.`,
					},
				},
			},
		},
	}

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.True(t, result)
	assert.Greater(t, len(session.history), 0, "history should be populated")

	// Verify precontext was added to SystemPrompt
	assert.Contains(t, session.Agent.SystemPrompt, "# Project README")
	assert.Contains(t, session.Agent.SystemPrompt, PreContextHeader)

	// Verify skills were added to SystemPrompt
	assert.Contains(t, session.Agent.SystemPrompt, SkillsHeader)
	assert.Contains(t, session.Agent.SystemPrompt, "debug-tool")
	assert.Contains(t, session.Agent.SystemPrompt, "Helps debug and troubleshoot issues")
	assert.Contains(t, session.Agent.SystemPrompt, "analyze-code")
	assert.Contains(t, session.Agent.SystemPrompt, "Analyzes code and provides insights")

	// Verify session state was reset
	assert.Empty(t, session.ToolCalls, "ToolCalls should be reset")
	assert.Empty(t, session.ToolResults, "ToolResults should be reset")
	assert.Equal(t, 0, session.CurrentCall, "CurrentCall should be reset")
}

// TestHandlePreContextAndSkillsResult_InferencePromptUpdated tests that both
// SystemPrompt and InferencePrompt are updated with precontext and skills.
func TestHandlePreContextAndSkillsResult_InferencePromptUpdated(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID: "test-session-inference",
		Agent: &config.Agent{
			SystemPrompt:    "System: You are helpful",
			InferencePrompt: "Inference: Continue the conversation",
			PreContext:      []string{"guidelines.md"},
		},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-2",
			},
			Payload: map[string]interface{}{
				"text": "test message",
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
				"files": []interface{}{
					map[string]interface{}{
						"file_spec":    "guidelines.md",
						"file_content": "Important guidelines",
					},
					map[string]interface{}{
						"file_spec": "skills/helper/SKILL.md",
						"file_content": `---
name: helper-skill
description: Provides helpful assistance
---
Helper skill content`,
					},
				},
			},
		},
	}

	result := session.handlePreContextAndSkillsResult(toolCall, results)

	assert.True(t, result)

	// Verify both prompts were updated
	assert.Contains(t, session.Agent.SystemPrompt, "Important guidelines")
	assert.Contains(t, session.Agent.SystemPrompt, "helper-skill")
	assert.Contains(t, session.Agent.InferencePrompt, "Important guidelines")
	assert.Contains(t, session.Agent.InferencePrompt, "helper-skill")
}

// TestPreContextAndSkillsWorkflow_Complete tests the complete workflow of
// loading and handling both precontext and skills.
func TestPreContextAndSkillsWorkflow_Complete(t *testing.T) {
	store := newMockStore(nil)
	session := &AgentSession{
		ID: "test-session-complete-workflow",
		Agent: &config.Agent{
			FileTool:        "wf__read_files",
			SystemPrompt:    "You are a helpful assistant",
			InferencePrompt: "Continue conversation",
			PreContext:      []string{"setup.md", "config.md"},
			SkillsPaths:     []string{"tools/search/SKILL.md", "tools/analyze/SKILL.md"},
		},
		history: []AgentMessage{},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"unified_convo_id": "convo-3",
			},
			Payload: map[string]interface{}{
				"text": "help me analyze this code",
			},
		},
		store: store,
	}

	// Step 1: Load precontext and skills
	loadResult := session.loadPreContextAndSkills()
	assert.True(t, loadResult)
	require.Len(t, session.ToolCalls, 1)

	// Verify all files are included
	fileSpecs := session.ToolCalls[0].Params["file_specs"].([]string)
	assert.Len(t, fileSpecs, 4, "should have 2 precontext + 2 skills")
	assert.Contains(t, fileSpecs, "setup.md")
	assert.Contains(t, fileSpecs, "config.md")
	assert.Contains(t, fileSpecs, "tools/search/SKILL.md")
	assert.Contains(t, fileSpecs, "tools/analyze/SKILL.md")
}
