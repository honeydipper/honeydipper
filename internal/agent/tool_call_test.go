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
				"mcp": map[string]interface{}{
					"tools_cache_ttl": "30m",
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
				"mcp": map[string]interface{}{
					"tools_cache_ttl": 12345, // invalid type, should be string
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
				"mcp": map[string]interface{}{
					"tools_cache_ttl": "invalid-duration",
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
				"mcp": map[string]interface{}{
					"tools_cache_ttl": "",
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
				"mcp": map[string]interface{}{
					"tools_cache_ttl": "2h",
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
				"mcp": map[string]interface{}{
					"tools_cache_ttl": "30m",
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
				"mcp": map[string]interface{}{
					"tools_cache_ttl": "15s",
				},
			},
		},
	})

	s := &AgentSession{store: store, ID: "test-session"}
	ttl := s.getMCPToolsCacheTTL()

	assert.Equal(t, "15s", ttl)
}
