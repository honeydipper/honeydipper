// Package session (part 2): policy evaluation manager.
//
// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package session

import (
	"context"
	"errors"
	"time"
)

// CheckResult carries the outcome of evaluating a session for a request.
type CheckResult struct {
	// Sid is the session identifier being evaluated.
	Sid string
	// Subject is the subject resolved from the store (authoritative).
	Subject string
	// OK is true when the request may proceed with this sid (policy allows).
	OK bool
	// IdleExceeded is true when the idle window has elapsed. Whether this is a
	// re-vouch (provider alive) or a hard re-auth is decided by the caller
	// based on the provider tier.
	IdleExceeded bool
	// MaxLifetimeExceeded is true when the absolute cap has been reached. This
	// is a hard re-auth - the sid must never be silently re-vouched.
	MaxLifetimeExceeded bool
	// Revoked is true when the sid has been revoked server-side.
	Revoked bool
	// StoreDown is true when the session store (Redis) is unavailable. Access
	// must fail closed.
	StoreDown bool
	// Unknown is true when the sid was not previously known and was lazily
	// registered (safety net / backfill).
	Unknown bool
	// IssuedAt is the authoritative session issuance time (unix seconds)
	// resolved from the store. It lets callers anchor token expirations to the
	// session's absolute cap even when the sid was lazily registered.
	IssuedAt int64
}

// Manager evaluates sessions against a session store and the daemon policy.
type Manager struct {
	store            Store
	policy           Policy
	tokenGracePeriod time.Duration
}

// NewManager builds a Manager around the given store and policy.
func NewManager(store Store, policy Policy, tokenGracePeriod time.Duration) *Manager {
	return &Manager{store: store, policy: policy, tokenGracePeriod: tokenGracePeriod}
}

// Store returns the underlying session store (used for logout revocation).
func (m *Manager) Store() Store {
	return m.store
}

// GraceActive reports whether the roll-out grace period is still in effect for
// pre-upgrade (sid-less) tokens.
func (m *Manager) GraceActive() bool {
	return m.tokenGracePeriod > 0
}

// CheckNoSID evaluates a pre-upgrade token that carries no sid. During the
// configured grace period such tokens are accepted on a best-effort basis;
// after the grace period they are cleanly rejected (session-expired).
func (m *Manager) CheckNoSID() CheckResult {
	if m.GraceActive() {
		return CheckResult{OK: true, Unknown: true}
	}

	return CheckResult{OK: false}
}

// Check evaluates a token carrying the given sid.
//
// This is a read-only policy evaluation (it does not refresh LastSeen). Unknown
// sids on validly-signed tokens are lazily registered as a roll-out safety net.
func (m *Manager) Check(ctx context.Context, sid, subject, provider string, issuedAt int64) CheckResult {
	rec, err := m.store.Get(ctx, sid)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			return CheckResult{Sid: sid, StoreDown: true}
		}
		if errors.Is(err, ErrNotFound) {
			// Safety net / backfill: validly-signed token carrying a sid unknown
			// to the store. Register it lazily and accept.
			now := time.Now().Unix()
			if issuedAt == 0 {
				issuedAt = now
			}
			_ = m.store.Register(ctx, Record{
				SID: sid, Subject: subject, Provider: provider,
				IssuedAt: issuedAt, LastSeen: now,
			})

			return CheckResult{Sid: sid, Subject: subject, OK: true, Unknown: true, IssuedAt: issuedAt}
		}

		return CheckResult{Sid: sid, StoreDown: true}
	}

	res := CheckResult{Sid: sid, Subject: rec.Subject, Revoked: rec.Revoked, IssuedAt: rec.IssuedAt}
	if res.Revoked {
		return res
	}

	now := time.Now().Unix()
	if m.policy.MaxLifetime > 0 && rec.IssuedAt > 0 && now-rec.IssuedAt >= int64(m.policy.MaxLifetime.Seconds()) {
		res.MaxLifetimeExceeded = true

		return res
	}
	if m.policy.IdleTimeout > 0 && rec.LastSeen > 0 && now-rec.LastSeen >= int64(m.policy.IdleTimeout.Seconds()) {
		res.IdleExceeded = true
	}

	res.OK = true

	return res
}

// Refresh refreshes LastSeen for an accepted session (idle definition: any
// authenticated request reaching an API route counts as activity).
func (m *Manager) Refresh(ctx context.Context, sid string) error {
	_, err := m.store.Touch(ctx, sid, "", "", 0)

	//nolint:wrapcheck // the underlying store already returns session-domain errors
	return err
}
