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
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Phase 1 regression: addSystemTool / addWorkflowTool / addAgentTool tolerate
// missing entities (skip + Warning, no zero-function registration, no panic)
// ---------------------------------------------------------------------------

func TestBuildTools_SystemMissing_SkipsTool(t *testing.T) {
	store := newMockStore(nil) // empty config: no systems
	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "system", Name: "missing-sys"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA
	s := &AgentSession{store: store, Agent: &agentA}

	tools := s.BuildTools()
	_, ok := tools["sys_missing-sys__anything"]
	assert.False(t, ok, "tool for a missing system must not be registered")
	// The util tool is always present.
	assert.Contains(t, tools, "util__hd_get_convo_url")
}

func TestBuildTools_WorkflowMissing_SkipsTool(t *testing.T) {
	store := newMockStore(nil) // empty config: no workflows
	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "workflow", Name: "missing-wf"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA
	s := &AgentSession{store: store, Agent: &agentA}

	tools := s.BuildTools()
	_, ok := tools["wf__missing-wf"]
	assert.False(t, ok, "tool for a missing workflow must not be registered")
}

func TestBuildTools_AgentMissing_SkipsTool(t *testing.T) {
	store := newMockStore(nil) // empty config: no agents
	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "agent", Name: "missing-ag"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA
	s := &AgentSession{store: store, Agent: &agentA}

	tools := s.BuildTools()
	_, ok := tools["ag__missing-ag"]
	assert.False(t, ok, "tool for a missing agent must not be registered")
}

// TestBuildTools_WorkflowNilMeta_Tolerated verifies a workflow with a nil or
// non-map Meta no longer panics: the tool is registered with the workflow's
// own Description and empty params.
func TestBuildTools_WorkflowNilMeta_Tolerated(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Workflows["nilmeta"] = config.Workflow{Name: "nilmeta", Description: "nil meta wf", Meta: nil}
	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "workflow", Name: "nilmeta"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA
	s := &AgentSession{store: store, Agent: &agentA}

	tools := s.BuildTools()
	tool, ok := tools["wf__nilmeta"]
	require.True(t, ok, "workflow with nil Meta must still be registered")
	assert.Equal(t, "nil meta wf", tool.Description)
	assert.Empty(t, tool.Params)
}

func TestBuildTools_WorkflowNonMapMeta_Tolerated(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Workflows["strmeta"] = config.Workflow{Name: "strmeta", Description: "str meta wf", Meta: "not-a-map"}
	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "workflow", Name: "strmeta"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA
	s := &AgentSession{store: store, Agent: &agentA}

	tools := s.BuildTools()
	tool, ok := tools["wf__strmeta"]
	require.True(t, ok, "workflow with non-map Meta must still be registered")
	assert.Equal(t, "str meta wf", tool.Description)
	assert.Empty(t, tool.Params)
}

// ---------------------------------------------------------------------------
// Phase 1 regression: processToolResult with a system removed between dispatch
// and result -> synthetic failure result, no panic, turn continues
// ---------------------------------------------------------------------------

func TestProcessToolResult_SystemRemoved_InjectsFailure(t *testing.T) {
	store := newMockStore(nil)
	// The system is NOT in the config (removed after dispatch).
	store.cfg.DataSet.Systems = map[string]config.System{}

	toolCalls := []AgentToolCall{
		{FuncName: "sys_gone__doThing", Params: map[string]interface{}{}},
		{FuncName: "wf__next"},
	}
	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Driver: "openai"},
		ID:          "sys-removed-session",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls:   toolCalls,
		history: []AgentMessage{
			{Role: RoleAgent, ToolCalls: toolCalls},
		},
	}

	msg := &dipper.Message{
		Labels: map[string]string{
			"turn_id":      "1",
			"tool_call_id": "0",
			"status":       "success", // operator says success, but system is gone
		},
		Payload: map[string]interface{}{},
	}

	// Must NOT panic.
	assert.NotPanics(t, func() { s.processToolResult(msg) })

	// A synthetic failure result was injected for the removed system.
	require.Len(t, s.ToolResults, 1)
	result := s.ToolResults[0]
	assert.Equal(t, "failure", result["status"], "missing system must inject a failure result")
	assert.Contains(t, result["reason"], "gone")
	assert.Equal(t, "sys_gone__doThing", result["func_name"])

	// The turn advanced to the next pending call (does not stall).
	assert.Equal(t, 1, s.CurrentCall)
	emitted := store.getEmitted()
	require.NotEmpty(t, emitted, "the next tool call must be dispatched so the turn continues")
	assert.Equal(t, "agent_workflow", emitted[0].Subject)
	assert.Equal(t, "next", emitted[0].Payload.(map[string]interface{})["do"].(map[string]interface{})["call_workflow"])
}

// TestProcessToolResult_SystemPresent_UsesExport verifies the normal path still
// works (a regression guard for the nil-system branch).
func TestProcessToolResult_SystemPresent_UsesExport(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["mysys"] = makeSystemWithFunc("doWork")

	toolCalls := []AgentToolCall{
		{FuncName: "sys_mysys__doWork", Params: map[string]interface{}{"param1": "v"}},
		{FuncName: "wf__next"},
	}
	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Driver: "openai"},
		ID:          "sys-present-session",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls:   toolCalls,
		history: []AgentMessage{
			{Role: RoleAgent, ToolCalls: toolCalls},
		},
	}

	msg := &dipper.Message{
		Labels: map[string]string{
			"turn_id":      "1",
			"tool_call_id": "0",
			"status":       "success",
		},
		Payload: map[string]interface{}{},
	}

	s.processToolResult(msg)
	require.Len(t, s.ToolResults, 1)
	assert.Equal(t, "success", s.ToolResults[0]["status"])
	assert.Equal(t, 1, s.CurrentCall, "turn must advance to the next pending call")
}
