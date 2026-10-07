// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"context"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/api/session"
)

// newTestSessionEngine builds an enabled session engine backed by an in-memory
// store with the given policy, for use in middleware tests.
func newTestSessionEngine(policy session.Policy, grace time.Duration) *sessionEngine {
	store := session.NewMemoryStore()
	cache := session.NewCache(store, time.Minute)
	mgr := session.NewManager(cache, policy, grace)

	return &sessionEngine{
		enabled:          true,
		idleTimeout:      policy.IdleTimeout,
		maxLifetime:      policy.MaxLifetime,
		tokenGracePeriod: grace,
		cacheTTL:         time.Minute,
		manager:          mgr,
	}
}

// registerTestSession registers a session into the store attached to a test engine.
func registerTestSession(engine *sessionEngine, sid, subject, provider string, issuedAt, lastSeen int64) {
	if engine == nil || engine.manager == nil {
		return
	}
	_ = engine.manager.Store().Register(context.Background(), session.Record{
		SID: sid, Subject: subject, Provider: provider, IssuedAt: issuedAt, LastSeen: lastSeen,
	})
}
