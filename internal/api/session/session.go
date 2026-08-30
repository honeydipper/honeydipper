// Package session provides the Redis-backed server-side login session store
// used by the Honeydipper API auth middleware.
//
// Session identity is a daemon-minted opaque UUID (sid) that is stable across
// token rotation. It is the primary key of the session store. Because
// Honeydipper is multi-node, the store is Redis-backed so that login sessions
// can be enforced consistently across all API nodes.
package session

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound indicates no session record exists for the given sid.
	ErrNotFound = errors.New("session not found")
	// ErrUnavailable indicates the session store (Redis) could not be reached.
	// Callers should fail closed rather than allow access.
	ErrUnavailable = errors.New("session store unavailable")
)

// Record is the server-side representation of a login session.
//
// IssuedAt is preserved across token rotation so the optional absolute
// maxLifetime cap cannot be bypassed by rotating tokens. LastSeen is refreshed
// on every authenticated request (idle definition) and drives the idle timeout.
type Record struct {
	SID      string
	Subject  string
	Provider string
	// IssuedAt is the unix-seconds time the session was first minted.
	IssuedAt int64
	// LastSeen is the unix-seconds time of the most recent authenticated activity.
	LastSeen int64
	// Revoked marks the sid as revoked (a revocation tombstone).
	Revoked bool
}

// Policy carries the daemon-side session policy knobs.
type Policy struct {
	// IdleTimeout, when > 0, is the inactivity window after which the session is
	// considered idle. It is a provider re-vouch trigger, not a hard logout.
	IdleTimeout time.Duration
	// MaxLifetime, when > 0, is an absolute cap on session lifetime for
	// compliance. A sid that has exceeded MaxLifetime must never be silently
	// re-vouched; it always requires a new session.
	MaxLifetime time.Duration
	// TokenValidity is the longest token validity window in the system. It is
	// used to size record TTLs so a revocation tombstone outlives any
	// replayed-but-still-signature-valid token.
	TokenValidity time.Duration
}

// RecordTTL returns the Redis TTL for a session record.
//
// The TTL is sized so that the record (and any revocation tombstone) outlives
// the longest possible remaining token validity window after the session stops
// being touched. This prevents a GC'd entry from being resurrected by a
// replayed-but-still-signature-valid token (requirement 3).
func (p Policy) RecordTTL() time.Duration {
	ttl := p.TokenValidity
	if p.IdleTimeout > ttl {
		ttl = p.IdleTimeout
	}
	if p.MaxLifetime > ttl {
		ttl = p.MaxLifetime
	}
	ttl += p.TokenValidity
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	return ttl
}

// Store is the interface the auth middleware uses to enforce sessions.
type Store interface {
	// Register eagerly creates or updates a session. If the sid already exists,
	// IssuedAt and Revoked are preserved (so rotation cannot reset the absolute
	// cap or resurrect a revoked session) and only LastSeen is refreshed.
	Register(ctx context.Context, r Record) error
	// Touch refreshes LastSeen, lazily creating the session if the sid is
	// unknown (safety net / backfill for rollout).
	Touch(ctx context.Context, sid, subject, provider string, issuedAt int64) (Record, error)
	// Get returns the current record and a definitive result. It returns
	// ErrNotFound for an unknown sid or ErrUnavailable on a store outage.
	Get(ctx context.Context, sid string) (Record, error)
	// Revoke marks a single sid as revoked (server-side logout).
	Revoke(ctx context.Context, sid string) error
	// RevokeAllForSubject revokes every session belonging to subject (incident
	// response) and returns the revoked sids for local cache invalidation.
	RevokeAllForSubject(ctx context.Context, subject string) ([]string, error)
}
