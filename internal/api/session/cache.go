// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Cache wraps a Store with a short-TTL in-memory validation cache so a full
// Redis round-trip is not needed on every request. Revocation is boundedly
// stale up to the cache TTL.
//
// On a revocation, the revoked sid(s) are immediately evicted from the local
// cache and a revoked record is cached that overrides any earlier positive
// entry for the TTL. Cross-node revocation propagation therefore lags by at
// most the cache TTL on nodes that did not observe the revoke call directly.
//
// The cache also throttles last_seen write amplification: Touch refreshes the
// in-memory LastSeen on every call but only writes through to the underlying
// store at most once per cache TTL. This bounds per-node Redis writes for a
// busy deployment while keeping idle-timeout semantics within the throttle
// window (negligible compared to typical idle timeouts of minutes/hours).
type Cache struct {
	store Store
	ttl   time.Duration
	mu    sync.Mutex
	state map[string]cacheEntry
}

type cacheEntry struct {
	record  Record
	unknown bool
	expires time.Time
	// lastStoreWrite is when the underlying store was last written for this
	// sid, used to throttle last_seen write amplification.
	lastStoreWrite time.Time
}

// NewCache wraps store with an in-memory validation cache with the given TTL.
func NewCache(store Store, ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}

	return &Cache{store: store, ttl: ttl, state: map[string]cacheEntry{}}
}

// Register delegates to the underlying store and warms the local cache.
func (c *Cache) Register(ctx context.Context, r Record) error {
	if err := c.store.Register(ctx, r); err != nil {
		return fmt.Errorf("%w", err)
	}
	c.put(r.SID, cacheEntry{record: r, expires: time.Now().Add(c.ttl), lastStoreWrite: time.Now()})

	return nil
}

// Touch refreshes LastSeen, lazily creating the session if the sid is unknown.
//
// Known, active sids have their LastSeen refreshed in the local cache on every
// call, but the underlying store is only written at most once per cache TTL to
// avoid write amplification on a busy multi-node deployment. Unknown/revoked
// sids always fall through to the store.
func (c *Cache) Touch(ctx context.Context, sid, subject, provider string, issuedAt int64) (Record, error) {
	now := time.Now()
	c.mu.Lock()
	e, ok := c.state[sid]
	if ok && !e.unknown && !e.record.Revoked && now.Before(e.expires) {
		// Refresh last_seen locally.
		e.record.LastSeen = now.Unix()
		throttled := now.Sub(e.lastStoreWrite) < c.ttl
		c.state[sid] = e
		c.mu.Unlock()
		if throttled {
			// No store write this time; the in-memory value is current enough
			// for idle semantics within the throttle window.
			return e.record, nil
		}
		// Bounded write-through: refresh the store, then warm the cache from
		// the authoritative result.
		r, err := c.store.Touch(ctx, sid, subject, provider, issuedAt)
		if err != nil {
			return r, fmt.Errorf("%w", err)
		}
		r.LastSeen = now.Unix()
		c.put(sid, cacheEntry{record: r, expires: now.Add(c.ttl), lastStoreWrite: now})

		return r, nil
	}
	c.mu.Unlock()

	// Unknown or revoked sid (or cache expired): write through to the store so
	// lazy registration / revocation handling stays correct.
	r, err := c.store.Touch(ctx, sid, subject, provider, issuedAt)
	if err != nil {
		return r, fmt.Errorf("%w", err)
	}
	c.put(sid, cacheEntry{record: r, expires: now.Add(c.ttl), lastStoreWrite: now})

	return r, nil
}

// Get returns the cached record within TTL or falls back to Redis.
func (c *Cache) Get(ctx context.Context, sid string) (Record, error) {
	if e, ok := c.peek(sid); ok {
		if e.unknown {
			return Record{}, ErrNotFound
		}

		return e.record, nil
	}

	r, err := c.store.Get(ctx, sid)
	if errors.Is(err, ErrNotFound) {
		// Cache a short negative entry to amortize repeated unknown lookups.
		c.put(sid, cacheEntry{unknown: true, expires: time.Now().Add(c.ttl)})

		return r, fmt.Errorf("%w", err)
	}
	if err == nil && r.Revoked {
		// Cache the revoked tombstone so it stays rejected for the window.
		c.put(sid, cacheEntry{record: r, expires: time.Now().Add(c.revokeTTL())})
	}

	if err != nil {
		return r, fmt.Errorf("%w", err)
	}

	return r, nil
}

// Revoke revokes the sid and immediately caches the revoked tombstone, giving
// it a TTL that outlives any remaining token validity.
func (c *Cache) Revoke(ctx context.Context, sid string) error {
	if err := c.store.Revoke(ctx, sid); err != nil {
		return fmt.Errorf("%w", err)
	}
	c.put(sid, cacheEntry{record: Record{SID: sid, Revoked: true}, expires: time.Now().Add(c.revokeTTL())})

	return nil
}

// RevokeAllForSubject revokes every session for subject and caches each revoked
// sid locally so subsequent requests on this node are rejected immediately.
func (c *Cache) RevokeAllForSubject(ctx context.Context, subject string) ([]string, error) {
	sids, err := c.store.RevokeAllForSubject(ctx, subject)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	ttl := c.revokeTTL()
	for _, sid := range sids {
		c.put(sid, cacheEntry{record: Record{SID: sid, Revoked: true}, expires: time.Now().Add(ttl)})
	}

	return sids, nil
}

// PruneSubject delegates pruning of the per-subject index to the underlying
// store so expired/GC'd sids stop accumulating in the index.
func (c *Cache) PruneSubject(ctx context.Context, subject string) error {
	if err := c.store.PruneSubject(ctx, subject); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func (c *Cache) revokeTTL() time.Duration {
	// Revoked sids must stay rejected for at least the remaining token window.
	if c.ttl < 24*time.Hour {
		return 24 * time.Hour
	}

	return c.ttl
}

func (c *Cache) peek(sid string) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.state[sid]
	if !ok {
		return cacheEntry{}, false
	}
	if time.Now().After(e.expires) {
		delete(c.state, sid)

		return cacheEntry{}, false
	}

	return e, true
}

func (c *Cache) put(sid string, e cacheEntry) {
	e.record.SID = sid
	c.mu.Lock()
	c.state[sid] = e
	c.mu.Unlock()
}
