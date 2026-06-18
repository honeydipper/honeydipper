// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"encoding/json"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// getMCPToolsCacheTTL
// ---------------------------------------------------------------------------

func TestGetMCPToolsCacheTTL_DefaultWhenConfigNil(t *testing.T) {
	store := newMockStore(nil)
	// Override cfg to nil to simulate missing config
	store.cfg = nil

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, MCPToolsCacheDefaultTTL, ttl)
}

func TestGetMCPToolsCacheTTL_DefaultWhenDataSetNil(t *testing.T) {
	store := newMockStore(&config.Config{})
	// Config has no DataSet

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, MCPToolsCacheDefaultTTL, ttl)
}

func TestGetMCPToolsCacheTTL_DefaultWhenDriverDataMissing(t *testing.T) {
	store := newMockStore(&config.Config{
		DataSet: &config.DataSet{
			Drivers: map[string]interface{}{},
		},
	})

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, MCPToolsCacheDefaultTTL, ttl)
}

func TestGetMCPToolsCacheTTL_CustomTTL(t *testing.T) {
	store := newMockStore(&config.Config{
		DataSet: &config.DataSet{
			Drivers: map[string]interface{}{
				"daemon": map[string]interface{}{
					"services": map[string]interface{}{
						"agent": map[string]interface{}{
							"tools_cache_ttl": "30m",
						},
					},
				},
			},
		},
	})

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, "30m", ttl)
}

func TestGetMCPToolsCacheTTL_InvalidType(t *testing.T) {
	store := newMockStore(&config.Config{
		DataSet: &config.DataSet{
			Drivers: map[string]interface{}{
				"daemon": map[string]interface{}{
					"services": map[string]interface{}{
						"agent": map[string]interface{}{
							"tools_cache_ttl": 12345, // invalid type, should be string
						},
					},
				},
			},
		},
	})

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, MCPToolsCacheDefaultTTL, ttl)
}

func TestGetMCPToolsCacheTTL_InvalidDuration(t *testing.T) {
	store := newMockStore(&config.Config{
		DataSet: &config.DataSet{
			Drivers: map[string]interface{}{
				"daemon": map[string]interface{}{
					"services": map[string]interface{}{
						"agent": map[string]interface{}{
							"tools_cache_ttl": "invalid-duration",
						},
					},
				},
			},
		},
	})

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, MCPToolsCacheDefaultTTL, ttl)
}

func TestGetMCPToolsCacheTTL_EmptyString(t *testing.T) {
	store := newMockStore(&config.Config{
		DataSet: &config.DataSet{
			Drivers: map[string]interface{}{
				"daemon": map[string]interface{}{
					"services": map[string]interface{}{
						"agent": map[string]interface{}{
							"tools_cache_ttl": "",
						},
					},
				},
			},
		},
	})

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, MCPToolsCacheDefaultTTL, ttl)
}

func TestGetMCPToolsCacheTTL_TwoHours(t *testing.T) {
	store := newMockStore(&config.Config{
		DataSet: &config.DataSet{
			Drivers: map[string]interface{}{
				"daemon": map[string]interface{}{
					"services": map[string]interface{}{
						"agent": map[string]interface{}{
							"tools_cache_ttl": "2h",
						},
					},
				},
			},
		},
	})

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, "2h", ttl)
}

func TestGetMCPToolsCacheTTL_30Minutes(t *testing.T) {
	store := newMockStore(&config.Config{
		DataSet: &config.DataSet{
			Drivers: map[string]interface{}{
				"daemon": map[string]interface{}{
					"services": map[string]interface{}{
						"agent": map[string]interface{}{
							"tools_cache_ttl": "30m",
						},
					},
				},
			},
		},
	})

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, "30m", ttl)
}

func TestGetMCPToolsCacheTTL_15Seconds(t *testing.T) {
	store := newMockStore(&config.Config{
		DataSet: &config.DataSet{
			Drivers: map[string]interface{}{
				"daemon": map[string]interface{}{
					"services": map[string]interface{}{
						"agent": map[string]interface{}{
							"tools_cache_ttl": "15s",
						},
					},
				},
			},
		},
	})

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, "15s", ttl)
}

// ---------------------------------------------------------------------------
// MCP tool filtering (Only / Excludes)
// ---------------------------------------------------------------------------

// helper to build a raw MCP list_tools response payload.
func mcpToolListRaw(tools []map[string]interface{}) []byte {
	payload := map[string]interface{}{"tools": tools}
	b, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	return b
}

// helper to build a minimal MCP tool definition entry.
func mcpTool(name, desc string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"description": desc,
		"input_schema": map[string]interface{}{
			"properties": map[string]interface{}{
				"param1": map[string]interface{}{
					"name":        "param1",
					"type":        "string",
					"description": "a param",
				},
			},
		},
	}
}

// setupMCPTestStore prepares a mock store for MCP tool tests.
// It pre-sets the MCP driver response and ensures the cache load for
// MCP tools returns empty so the code path goes through fetchMCPTools.
func setupMCPTestStore(t *testing.T, raw []byte) *mockStore {
	t.Helper()
	store := newMockStore(nil)
	store.resp["driver:mcp:list_tools"] = raw
	// Ensure cache:load returns empty for the MCP tools cache key so
	// tryCacheMCPTools fails and fetchMCPTools is called.
	store.resp["cache:load:"+MCPToolsCachePrefix+"myserver"] = []byte("")

	return store
}

func TestProcessMCPToolList_OnlyFilter(t *testing.T) {
	raw := mcpToolListRaw([]map[string]interface{}{
		mcpTool("alpha", "tool alpha"),
		mcpTool("beta", "tool beta"),
		mcpTool("gamma", "tool gamma"),
	})
	store := setupMCPTestStore(t, raw)

	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "mcp", Name: "myserver", Only: []string{"alpha", "gamma"}},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-only"}
	tools := s.BuildTools()

	assert.Len(t, tools, 3)
	assert.Contains(t, tools, "mcp__myserver__alpha")
	assert.Contains(t, tools, "mcp__myserver__gamma")
	assert.NotContains(t, tools, "mcp__myserver__beta")
}

func TestProcessMCPToolList_ExcludesFilter(t *testing.T) {
	raw := mcpToolListRaw([]map[string]interface{}{
		mcpTool("alpha", "tool alpha"),
		mcpTool("beta", "tool beta"),
		mcpTool("gamma", "tool gamma"),
	})
	store := setupMCPTestStore(t, raw)

	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "mcp", Name: "myserver", Excludes: []string{"beta"}},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-excludes"}
	tools := s.BuildTools()

	assert.Len(t, tools, 3)
	assert.Contains(t, tools, "mcp__myserver__alpha")
	assert.Contains(t, tools, "mcp__myserver__gamma")
	assert.NotContains(t, tools, "mcp__myserver__beta")
}

func TestProcessMCPToolList_BothOnlyAndExcludes(t *testing.T) {
	raw := mcpToolListRaw([]map[string]interface{}{
		mcpTool("alpha", "tool alpha"),
		mcpTool("beta", "tool beta"),
		mcpTool("gamma", "tool gamma"),
		mcpTool("delta", "tool delta"),
	})
	store := setupMCPTestStore(t, raw)

	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "mcp", Name: "myserver", Only: []string{"alpha", "beta", "gamma"}, Excludes: []string{"beta"}},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-both"}
	tools := s.BuildTools()

	// Only whitelist is applied first (alpha, beta, gamma), then Excludes removes beta.
	assert.Len(t, tools, 3)
	assert.Contains(t, tools, "mcp__myserver__alpha")
	assert.Contains(t, tools, "mcp__myserver__gamma")
	assert.NotContains(t, tools, "mcp__myserver__beta")
	assert.NotContains(t, tools, "mcp__myserver__delta")
}

func TestProcessMCPToolList_NoFilter(t *testing.T) {
	raw := mcpToolListRaw([]map[string]interface{}{
		mcpTool("alpha", "tool alpha"),
		mcpTool("beta", "tool beta"),
	})
	store := setupMCPTestStore(t, raw)

	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "mcp", Name: "myserver"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-nofilter"}
	tools := s.BuildTools()

	// All tools should be present when no filter is specified.
	assert.Len(t, tools, 3)
	assert.Contains(t, tools, "mcp__myserver__alpha")
	assert.Contains(t, tools, "mcp__myserver__beta")
}

func TestProcessMCPToolList_OnlyNonexistentAllExcluded(t *testing.T) {
	raw := mcpToolListRaw([]map[string]interface{}{
		mcpTool("alpha", "tool alpha"),
		mcpTool("beta", "tool beta"),
	})
	store := setupMCPTestStore(t, raw)

	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "mcp", Name: "myserver", Only: []string{"nonexistent"}},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-only-empty"}
	tools := s.BuildTools()

	// Only lists a tool that doesn't exist — nothing registered.
	assert.Len(t, tools, 1)
}

func TestProcessMCPToolList_ExcludesAll(t *testing.T) {
	raw := mcpToolListRaw([]map[string]interface{}{
		mcpTool("alpha", "tool alpha"),
		mcpTool("beta", "tool beta"),
	})
	store := setupMCPTestStore(t, raw)

	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "mcp", Name: "myserver", Excludes: []string{"alpha", "beta"}},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-excludes-all"}
	tools := s.BuildTools()

	// All tools excluded — nothing registered.
	assert.Len(t, tools, 1)
}

func TestProcessMCPToolList_MixedToolTypes(t *testing.T) {
	raw := mcpToolListRaw([]map[string]interface{}{
		mcpTool("mcp_tool_a", "MCP tool A"),
		mcpTool("mcp_tool_b", "MCP tool B"),
	})
	store := setupMCPTestStore(t, raw)
	store.cfg.DataSet.Systems["s1"] = makeSystemWithFunc("sys_fn")
	store.cfg.DataSet.Workflows["wf1"] = makeWorkflow("wf1", "workflow 1")

	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "system", Name: "s1"},
			{Type: "workflow", Name: "wf1"},
			{Type: "mcp", Name: "myserver", Only: []string{"mcp_tool_a"}},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-mixed"}
	tools := s.BuildTools()

	assert.Len(t, tools, 4)
	assert.Contains(t, tools, "sys_s1__sys_fn")
	assert.Contains(t, tools, "wf__wf1")
	assert.Contains(t, tools, "mcp__myserver__mcp_tool_a")
	assert.NotContains(t, tools, "mcp__myserver__mcp_tool_b")
}

func TestProcessMCPToolList_CacheHitWithFilter(t *testing.T) {
	store := newMockStore(nil)
	raw := mcpToolListRaw([]map[string]interface{}{
		mcpTool("alpha", "tool alpha"),
		mcpTool("beta", "tool beta"),
		mcpTool("gamma", "tool gamma"),
	})

	// Pre-populate cache with the full tool list (no filter was applied when caching).
	store.resp["cache:load:"+MCPToolsCachePrefix+"myserver"] = raw

	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "mcp", Name: "myserver", Only: []string{"beta"}},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-cache-filter"}
	tools := s.BuildTools()

	// Even though the cached data has all tools, the filter is applied on retrieval.
	assert.Len(t, tools, 2)
	assert.Contains(t, tools, "mcp__myserver__beta")
}

// ---------------------------------------------------------------------------
// util__hd_get_convo_url tool tests
// ---------------------------------------------------------------------------

func TestBuildTools_IncludesUtilGetConvoURL(t *testing.T) {
	store := newMockStore(nil)
	agentA := config.Agent{
		Name:   "a",
		Driver: "openai",
		Tools:  []config.AgentToolDef{},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-util-tool"}
	tools := s.BuildTools()

	require.Contains(t, tools, "util__hd_get_convo_url")
	tool := tools["util__hd_get_convo_url"]
	assert.Equal(t, "util__hd_get_convo_url", tool.Name)
	assert.Contains(t, tool.Description, "conversation page URL")
	assert.Contains(t, tool.Description, "focus page URL")
	assert.Empty(t, tool.Params)
}

func TestBuildTools_UtilGetConvoURL_AlwaysPresent(t *testing.T) {
	// Even with no other tools, util__hd_get_convo_url should be present.
	store := newMockStore(nil)
	agentA := config.Agent{
		Name:   "a",
		Driver: "openai",
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{store: store, Agent: &agentA, ID: "test-util-always"}
	tools := s.BuildTools()

	require.Contains(t, tools, "util__hd_get_convo_url")
	// Only the util tool should be present (no hd_load_skill without SkillsHeader)
	assert.Len(t, tools, 1)
}

func TestNextToolCall_UtilGetConvoURLDispatch(t *testing.T) {
	store := newMockStore(nil)
	store.uiURL = "https://honeydipper.example.com"

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Name: "test-agent", Driver: "openai"},
		ID:          "util-dispatch-session",
		ConvoID:     "convo-123",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls: []AgentToolCall{
			{FuncName: "util__hd_get_convo_url", Params: map[string]interface{}{}},
		},
	}

	s.nextToolCall()

	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "agent_continue", emitted[0].Subject)
	assert.Equal(t, "success", emitted[0].Labels["status"])
	assert.Equal(t, s.ID, emitted[0].Labels["agent_session_id"])

	// Verify the payload contains the URLs
	output, ok := dipper.GetMapData(emitted[0].Payload, "data.output")
	require.True(t, ok)
	outputMap := output.(map[string]interface{})
	assert.Equal(t, "https://honeydipper.example.com/conversations/convo-123", outputMap["convo_url"])
	assert.Equal(t, "https://honeydipper.example.com/focus/convo-123", outputMap["focus_url"])
}

func TestNextToolCall_UtilGetConvoURL_TrimTrailingSlash(t *testing.T) {
	store := newMockStore(nil)
	store.uiURL = "https://honeydipper.example.com/"

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Name: "test-agent", Driver: "openai"},
		ID:          "util-slash-session",
		ConvoID:     "convo-456",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls: []AgentToolCall{
			{FuncName: "util__hd_get_convo_url", Params: map[string]interface{}{}},
		},
	}

	s.nextToolCall()

	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	output, _ := dipper.GetMapData(emitted[0].Payload, "data.output")
	outputMap := output.(map[string]interface{})
	assert.Equal(t, "https://honeydipper.example.com/conversations/convo-456", outputMap["convo_url"])
	assert.Equal(t, "https://honeydipper.example.com/focus/convo-456", outputMap["focus_url"])
}

func TestNextToolCall_UtilGetConvoURL_MissingUIURL(t *testing.T) {
	store := newMockStore(nil)
	// uiURL is empty by default

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Name: "test-agent", Driver: "openai"},
		ID:          "util-no-url-session",
		ConvoID:     "convo-789",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls: []AgentToolCall{
			{FuncName: "util__hd_get_convo_url", Params: map[string]interface{}{}},
		},
	}

	assert.Panics(t, func() {
		s.nextToolCall()
	})
}

func TestProcessToolResult_UtilToolResult(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["s1"] = makeSystemWithFunc("fn1")

	toolCalls := []AgentToolCall{
		{FuncName: "util__hd_get_convo_url", Params: map[string]interface{}{}},
		{FuncName: "sys_s1__fn1", Params: map[string]interface{}{"param1": "v"}},
	}

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Driver: "openai"},
		ID:          "util-result-session",
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
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"output": map[string]interface{}{
					"convo_url": "https://example.com/conversations/c1",
					"focus_url": "https://example.com/focus/c1",
				},
			},
		},
	}

	s.processToolResult(msg)

	// The util result should be collected and the next tool dispatched.
	require.Len(t, s.ToolResults, 1)
	assert.Equal(t, "success", s.ToolResults[0]["status"])
	assert.Equal(t, "util__hd_get_convo_url", s.ToolResults[0]["func_name"])

	// Verify the data contains the output map
	data := s.ToolResults[0]["data"].(map[string]interface{})
	assert.Equal(t, "https://example.com/conversations/c1", data["convo_url"])
	assert.Equal(t, "https://example.com/focus/c1", data["focus_url"])

	// CurrentCall should be incremented and next tool dispatched.
	assert.Equal(t, 1, s.CurrentCall)
	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "agent_command", emitted[0].Subject)
}

func TestNextToolCall_UnknownUtilTool_Panics(t *testing.T) {
	store := newMockStore(nil)

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Name: "test-agent", Driver: "openai"},
		ID:          "util-unknown-session",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls: []AgentToolCall{
			{FuncName: "util__nonexistent_tool", Params: map[string]interface{}{}},
		},
	}

	assert.Panics(t, func() {
		s.nextToolCall()
	})
}
