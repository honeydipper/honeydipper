// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package api

import (
	"testing"
	"time"
)

func TestLoadSessionEngineDisabledByDefault(t *testing.T) {
	// No session config at all -> enabled defaults to false.
	engine := loadSessionEngine(map[string]interface{}{})
	if engine.enabled {
		t.Fatalf("expected sessions disabled when not configured")
	}
	if engine.tokenGracePeriod != DefaultSessionTokenGracePeriod {
		t.Fatalf("expected default token grace period, got %v", engine.tokenGracePeriod)
	}
}

func TestLoadSessionEngineReadsConfig(t *testing.T) {
	cfg := map[string]interface{}{
		"auth": map[string]interface{}{
			"session": map[string]interface{}{
				"enabled":          true,
				"idleTimeout":      "1h",
				"maxLifetime":      "24h",
				"tokenGracePeriod": "12h",
				"redis": map[string]interface{}{
					"addr": "redis.example:6379",
				},
			},
		},
	}

	engine := loadSessionEngine(cfg)
	if !engine.enabled {
		t.Fatalf("expected sessions enabled")
	}
	if engine.idleTimeout != time.Hour {
		t.Fatalf("expected idleTimeout 1h, got %v", engine.idleTimeout)
	}
	if engine.maxLifetime != 24*time.Hour {
		t.Fatalf("expected maxLifetime 24h, got %v", engine.maxLifetime)
	}
	if engine.tokenGracePeriod != 12*time.Hour {
		t.Fatalf("expected tokenGracePeriod 12h, got %v", engine.tokenGracePeriod)
	}
	if engine.redisAddr != "redis.example:6379" {
		t.Fatalf("expected redis addr, got %q", engine.redisAddr)
	}
	if engine.manager == nil {
		t.Fatalf("expected session manager built")
	}
}

func TestLoadSessionEngineRedisUsesRedisStore(t *testing.T) {
	cfg := map[string]interface{}{
		"auth": map[string]interface{}{
			"session": map[string]interface{}{
				"enabled": true,
				"redis": map[string]interface{}{
					"addr": "localhost:6379",
				},
			},
		},
	}
	engine := loadSessionEngine(cfg)
	if engine.redisAddr != "localhost:6379" {
		t.Fatalf("expected redis addr")
	}
	if engine.manager == nil || engine.manager.Store() == nil {
		t.Fatalf("expected session store")
	}
}
