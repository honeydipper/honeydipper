// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/api/session"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// Session policy constants and default values.
const (
	// DefaultSessionCacheTTL bounds the staleness of the short-TTL validation
	// cache used to avoid a Redis round-trip on every authenticated request.
	DefaultSessionCacheTTL = 30 * time.Second

	// DefaultSessionTokenGracePeriod is the roll-out grace period for
	// pre-upgrade (sid-less) tokens when not otherwise configured.
	DefaultSessionTokenGracePeriod = 24 * time.Hour

	// SessionExpiredHeader signals to the (cross-origin) UI that the presented
	// session cannot continue and it must re-authenticate.
	SessionExpiredHeader = "X-Honeydipper-Session-Expired"
)

// sessionEngine holds the parsed session policy and the live session manager.
type sessionEngine struct {
	enabled          bool
	idleTimeout      time.Duration
	maxLifetime      time.Duration
	tokenGracePeriod time.Duration
	cacheTTL         time.Duration
	manager          *session.Manager
	redisAddr        string
}

// loadSessionEngine parses the auth.session.* configuration and env overrides
// (HD_SESSION_* convention) and constructs the session manager. When no
// session config is present a local in-memory store is used so the API keeps
// working for single-node/test deployments; when configured, a Redis-backed
// store is used which is correct across multiple nodes.
func loadSessionEngine(cfg interface{}) *sessionEngine {
	engine := &sessionEngine{
		tokenGracePeriod: DefaultSessionTokenGracePeriod,
		cacheTTL:         DefaultSessionCacheTTL,
	}

	enabled, ok := envBool("HD_SESSION_ENABLED")
	if !ok {
		enabled, _ = dipper.GetMapDataBool(cfg, "auth.session.enabled")
	}
	engine.enabled = enabled

	engine.idleTimeout = parseDurationEnv(cfg, "HD_SESSION_IDLE_TIMEOUT", "auth.session.idleTimeout", 0)
	engine.maxLifetime = parseDurationEnv(cfg, "HD_SESSION_MAX_LIFETIME", "auth.session.maxLifetime", 0)
	engine.tokenGracePeriod = parseDurationEnv(
		cfg, "HD_SESSION_TOKEN_GRACE_PERIOD", "auth.session.tokenGracePeriod", DefaultSessionTokenGracePeriod)
	engine.cacheTTL = parseDurationEnv(cfg, "HD_SESSION_CACHE_TTL", "auth.session.cacheTTL", DefaultSessionCacheTTL)

	redisCfg := session.RedisConfig{}
	if v := os.Getenv("HD_SESSION_REDIS_ADDR"); v != "" {
		redisCfg.Addr = v
	} else if v, ok := dipper.GetMapDataStr(cfg, "auth.session.redis.addr"); ok {
		redisCfg.Addr = v
	}
	if v := os.Getenv("HD_SESSION_REDIS_USERNAME"); v != "" {
		redisCfg.Username = v
	} else if v, ok := dipper.GetMapDataStr(cfg, "auth.session.redis.username"); ok {
		redisCfg.Username = v
	}
	if v := os.Getenv("HD_SESSION_REDIS_PASSWORD"); v != "" {
		redisCfg.Password = v
	} else if v, ok := dipper.GetMapDataStr(cfg, "auth.session.redis.password"); ok {
		redisCfg.Password = v
	}
	if v := os.Getenv("HD_SESSION_REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			redisCfg.DB = n
		}
	} else if v, ok := dipper.GetMapDataStr(cfg, "auth.session.redis.db"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			redisCfg.DB = n
		}
	}
	engine.redisAddr = redisCfg.Addr

	policy := session.Policy{
		IdleTimeout:   engine.idleTimeout,
		MaxLifetime:   engine.maxLifetime,
		TokenValidity: 24 * time.Hour,
	}

	var store session.Store
	if redisCfg.Addr != "" {
		client := session.NewRedisClient(redisCfg)
		store = session.NewRedisStore(client, policy)
	} else {
		store = session.NewMemoryStore()
	}
	cached := session.NewCache(store, engine.cacheTTL)
	engine.manager = session.NewManager(cached, policy, engine.tokenGracePeriod)

	return engine
}

func parseDurationEnv(cfg interface{}, env, configPath string, def time.Duration) time.Duration {
	if v := os.Getenv(env); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	if v, ok := dipper.GetMapDataStr(cfg, configPath); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}

	return def
}

func envBool(name string) (bool, bool) {
	v := os.Getenv(name)
	if v == "" {
		return false, false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, false
	}

	return b, true
}

// sessionCtx is a convenience context for session store calls.
var sessionCtx = context.Background()
