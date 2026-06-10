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
	"github.com/stretchr/testify/assert"
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

	assert.Len(t, tools, 2)
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

	assert.Len(t, tools, 2)
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
	assert.Len(t, tools, 2)
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
	assert.Len(t, tools, 2)
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
	assert.Len(t, tools, 0)
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
	assert.Len(t, tools, 0)
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

	assert.Len(t, tools, 3)
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
	assert.Len(t, tools, 1)
	assert.Contains(t, tools, "mcp__myserver__beta")
}
