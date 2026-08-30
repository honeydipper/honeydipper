// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package session

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreRegisterAndGet(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().Unix()
	r := Record{SID: "sid-1", Subject: "alice", Provider: "auth-github", IssuedAt: now, LastSeen: now}

	if err := store.Register(context.Background(), r); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := store.Get(context.Background(), "sid-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Subject != "alice" || got.IssuedAt != now {
		t.Fatalf("unexpected record: %+v", got)
	}
}

func TestMemoryStoreRevokePreservesIssuedAt(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().Unix()
	if err := store.Register(context.Background(), Record{SID: "s", Subject: "bob", IssuedAt: now, LastSeen: now}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := store.Revoke(context.Background(), "s"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got, err := store.Get(context.Background(), "s")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Revoked {
		t.Fatalf("expected revoked record")
	}
	if got.IssuedAt != now {
		t.Fatalf("revocation must preserve issued_at, got %d", got.IssuedAt)
	}
}

func TestMemoryStoreRegisterDoesNotResetIssuedAtOnReRegister(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().Unix()
	if err := store.Register(context.Background(), Record{SID: "s", Subject: "carol", IssuedAt: now, LastSeen: now}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Re-registration (rotation) must not reset issued_at.
	later := now + 3600
	if err := store.Register(context.Background(), Record{SID: "s", Subject: "carol", IssuedAt: later, LastSeen: later}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, _ := store.Get(context.Background(), "s")
	if got.IssuedAt != now {
		t.Fatalf("rotation must not reset issued_at, got %d want %d", got.IssuedAt, now)
	}
	if got.Revoked {
		t.Fatalf("rotation must not resurrect a record, revoked=%v", got.Revoked)
	}
}

func TestMemoryStoreRevokeAllForSubject(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().Unix()
	for _, sid := range []string{"s1", "s2"} {
		if err := store.Register(context.Background(), Record{SID: sid, Subject: "dave", IssuedAt: now, LastSeen: now}); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	if err := store.Register(context.Background(), Record{SID: "other", Subject: "eve", IssuedAt: now, LastSeen: now}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	revoked, err := store.RevokeAllForSubject(context.Background(), "dave")
	if err != nil {
		t.Fatalf("RevokeAllForSubject: %v", err)
	}
	if len(revoked) != 2 {
		t.Fatalf("expected 2 revoked sids, got %v", revoked)
	}
	for _, sid := range revoked {
		r, _ := store.Get(context.Background(), sid)
		if !r.Revoked {
			t.Fatalf("expected %s revoked", sid)
		}
	}
	// Other subject unaffected.
	o, _ := store.Get(context.Background(), "other")
	if o.Revoked {
		t.Fatalf("other subject should not be revoked")
	}
}

func TestPolicyRecordTTL(t *testing.T) {
	p := Policy{TokenValidity: 24 * time.Hour, IdleTimeout: time.Hour}
	ttl := p.RecordTTL()
	if ttl < 48*time.Hour {
		t.Fatalf("expected TTL at least 2x token validity, got %v", ttl)
	}

	p2 := Policy{TokenValidity: 24 * time.Hour}
	if ttl2 := p2.RecordTTL(); ttl2 < 48*time.Hour {
		t.Fatalf("expected default TTL, got %v", ttl2)
	}
}

// TestManagerIdleExceeded checks that an idle session is flagged but a recent
// one is not.
func TestManagerIdleExceeded(t *testing.T) {
	store := NewMemoryStore()
	now := time.Unix(time.Now().Unix(), 0)
	store.Register(context.Background(), Record{
		SID: "s", Subject: "alice", IssuedAt: now.Unix(), LastSeen: now.Add(-2 * time.Hour).Unix(),
	})

	mgr := NewManager(store, Policy{IdleTimeout: time.Hour}, 0)
	res := mgr.Check(context.Background(), "s", "alice", "auth-github", now.Unix())
	if res.IdleExceeded == false || res.OK == false {
		t.Fatalf("expected idle-exceeded but OK, got %+v", res)
	}
}

func TestManagerRefreshTouchesLastSeen(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().Unix()
	store.Register(context.Background(), Record{SID: "s", Subject: "alice", IssuedAt: now, LastSeen: now - 3600})

	mgr := NewManager(store, Policy{IdleTimeout: time.Hour}, 0)
	if err := mgr.Refresh(context.Background(), "s"); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	r, _ := store.Get(context.Background(), "s")
	if r.LastSeen <= now-3600 {
		t.Fatalf("expected last_seen refreshed, got %d", r.LastSeen)
	}
}

func TestManagerCheckLazilyRegistersUnknownSID(t *testing.T) {
	store := NewMemoryStore()
	mgr := NewManager(store, Policy{}, 0)
	now := time.Now().Unix()

	res := mgr.Check(context.Background(), "unknown-sid", "alice", "auth-github", now)
	if !res.OK {
		t.Fatalf("expected unknown sid to be accepted via lazy registration, got %+v", res)
	}
	if !res.Unknown {
		t.Fatalf("expected Unknown flag, got %+v", res)
	}
	// Now the store has it.
	if _, err := store.Get(context.Background(), "unknown-sid"); err != nil {
		t.Fatalf("expected sid to be registered, got %v", err)
	}
}

func TestManagerCheckStoreDownFailsClosed(t *testing.T) {
	store := &failingStore{}
	mgr := NewManager(store, Policy{}, 0)
	res := mgr.Check(context.Background(), "s", "alice", "auth-github", 0)
	if !res.StoreDown || res.OK {
		t.Fatalf("expected store-down fail closed, got %+v", res)
	}
}

type failingStore struct{}

func (f *failingStore) Register(ctx context.Context, r Record) error {
	return ErrUnavailable
}

func (f *failingStore) Touch(ctx context.Context, sid, subject, provider string, issuedAt int64) (Record, error) {
	return Record{}, ErrUnavailable
}

func (f *failingStore) Get(ctx context.Context, sid string) (Record, error) {
	return Record{}, ErrUnavailable
}

func (f *failingStore) Revoke(ctx context.Context, sid string) error {
	return ErrUnavailable
}

func (f *failingStore) RevokeAllForSubject(ctx context.Context, subject string) ([]string, error) {
	return nil, ErrUnavailable
}

func (f *failingStore) PruneSubject(ctx context.Context, subject string) error {
	return ErrUnavailable
}

func TestManagerCheckNoSIDGrace(t *testing.T) {
	store := NewMemoryStore()
	// Within grace.
	mgr := NewManager(store, Policy{}, time.Hour)
	if res := mgr.CheckNoSID(); !res.OK {
		t.Fatalf("expected sid-less token accepted during grace, got %+v", res)
	}
	// After grace.
	mgr2 := NewManager(store, Policy{}, 0)
	if res := mgr2.CheckNoSID(); res.OK {
		t.Fatalf("expected sid-less token rejected after grace, got %+v", res)
	}
}

func TestCacheRevokeImmediatelyInvalidatesPositiveEntry(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().Unix()
	store.Register(context.Background(), Record{SID: "s", Subject: "alice", IssuedAt: now, LastSeen: now})

	cache := NewCache(store, time.Minute)
	if _, err := cache.Get(context.Background(), "s"); err != nil {
		t.Fatalf("cache Get: %v", err)
	}
	if err := cache.Revoke(context.Background(), "s"); err != nil {
		t.Fatalf("cache Revoke: %v", err)
	}
	// The cache must return the revoked tombstone record (not ErrNotFound) so
	// the manager's lazy-registration safety net cannot resurrect the sid.
	r, err := cache.Get(context.Background(), "s")
	if err != nil {
		t.Fatalf("expected revoked tombstone record, got %v", err)
	}
	if !r.Revoked {
		t.Fatalf("expected cached record to be revoked")
	}
}

// TestManagerCheckPopulatesIssuedAt verifies Check returns the authoritative
// session issuance time (F2) for both an existing and a lazily-registered sid.
func TestManagerCheckPopulatesIssuedAt(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().Unix()
	store.Register(context.Background(), Record{SID: "s", Subject: "alice", IssuedAt: now - 3600, LastSeen: now})

	mgr := NewManager(store, Policy{}, 0)
	res := mgr.Check(context.Background(), "s", "alice", "auth-github", 0)
	if !res.OK {
		t.Fatalf("expected OK, got %+v", res)
	}
	if res.IssuedAt != now-3600 {
		t.Fatalf("expected IssuedAt from store (%d), got %d", now-3600, res.IssuedAt)
	}
}

// TestManagerCheckLazyRegistrationAnchorsIssuedAt verifies that lazy
// registration stores the provided real issuance time (not first-seen) so the
// absolute maxLifetime cap is measured from actual token issuance (F2).
func TestManagerCheckLazyRegistrationAnchorsIssuedAt(t *testing.T) {
	store := NewMemoryStore()
	mgr := NewManager(store, Policy{}, 0)
	issuedAt := time.Now().Add(-20 * time.Hour).Unix()

	res := mgr.Check(context.Background(), "unknown-sid", "alice", "auth-github", issuedAt)
	if !res.OK || !res.Unknown {
		t.Fatalf("expected lazy-accepted unknown sid, got %+v", res)
	}
	if res.IssuedAt != issuedAt {
		t.Fatalf("expected lazy registration IssuedAt anchored to %d, got %d", issuedAt, res.IssuedAt)
	}
	r, _ := store.Get(context.Background(), "unknown-sid")
	if r.IssuedAt != issuedAt {
		t.Fatalf("expected store record IssuedAt %d, got %d", issuedAt, r.IssuedAt)
	}
}

// TestCacheTouchThrottlesWrites verifies the cache only writes through to the
// store at most once per cache TTL (F3).
func TestCacheTouchThrottlesWrites(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().Unix()
	store.Register(context.Background(), Record{SID: "s", Subject: "alice", IssuedAt: now, LastSeen: now})

	cache := NewCache(store, time.Hour)

	// First Touch writes through.
	if _, err := cache.Touch(context.Background(), "s", "alice", "auth-github", now); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	firstOnDisk := store.Snapshot()["s"].LastSeen

	// A second Touch within the TTL must NOT write through: the underlying
	// store's record is untouched, only the in-memory cache value changes.
	time.Sleep(5 * time.Millisecond)
	if _, err := cache.Touch(context.Background(), "s", "alice", "auth-github", now); err != nil {
		t.Fatalf("Touch (throttled): %v", err)
	}
	secondOnDisk := store.Snapshot()["s"].LastSeen
	if secondOnDisk != firstOnDisk {
		t.Fatalf("expected throttled Touch not to write through, disk last_seen changed %d -> %d", firstOnDisk, secondOnDisk)
	}
}

// TestMemoryStorePruneSubject verifies PruneSubject removes stale sids from the
// per-subject index whose records are gone (F4).
func TestMemoryStorePruneSubject(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().Unix()
	store.Register(context.Background(), Record{SID: "s1", Subject: "alice", IssuedAt: now, LastSeen: now})
	store.Register(context.Background(), Record{SID: "s2", Subject: "alice", IssuedAt: now, LastSeen: now})

	// Simulate GC: remove s1's record entirely (as if it expired).
	// Use a helper via the internal map through Snapshot-free path: delete directly.
	store.mu.Lock()
	delete(store.records, "s1")
	store.mu.Unlock()

	if err := store.PruneSubject(context.Background(), "alice"); err != nil {
		t.Fatalf("PruneSubject: %v", err)
	}

	// s2 remains, s1 pruned from the index.
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.subjectTo["alice"]["s1"]; ok {
		t.Fatalf("expected s1 pruned from alice index")
	}
	if !store.subjectTo["alice"]["s2"] {
		t.Fatalf("expected s2 to remain in alice index")
	}
}
