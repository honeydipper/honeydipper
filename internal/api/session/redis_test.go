// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package session

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/go-redis/redismock/v8"
)

func TestRedisStoreGet(t *testing.T) {
	client, mock := redismock.NewClientMock()
	store := NewRedisStore(client, Policy{TokenValidity: 24 * time.Hour})

	mock.ExpectHGetAll("hd-auth-session:sid-1").SetVal(map[string]string{
		"subject":   "alice",
		"provider":  "auth-github",
		"issued_at": "100",
		"last_seen": "200",
		"revoked":   "false",
	})

	r, err := store.Get(context.Background(), "sid-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if r.Subject != "alice" || r.Provider != "auth-github" || r.IssuedAt != 100 || r.LastSeen != 200 {
		t.Fatalf("unexpected record: %+v", r)
	}
	if r.SID != "sid-1" {
		t.Fatalf("expected sid set from key, got %q", r.SID)
	}
}

func TestRedisStoreGetUnknownReturnsNotFound(t *testing.T) {
	client, mock := redismock.NewClientMock()
	store := NewRedisStore(client, Policy{TokenValidity: 24 * time.Hour})

	mock.ExpectHGetAll("hd-auth-session:missing").RedisNil()

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRedisStoreRegisterWritesHashes(t *testing.T) {
	client, mock := redismock.NewClientMock()
	store := NewRedisStore(client, Policy{TokenValidity: 24 * time.Hour})

	now := time.Now().Unix()
	key := "hd-auth-session:sid-1"
	expectedHash := map[string]interface{}{
		"subject":   "alice",
		"provider":  "auth-github",
		"issued_at": strconv.FormatInt(now, 10),
		"last_seen": strconv.FormatInt(now, 10),
		"revoked":   "false",
	}

	mock.ExpectHGetAll(key).RedisNil()
	mock.ExpectTxPipeline()
	mock.ExpectHSet(key, expectedHash).SetVal(1)
	mock.ExpectExpire(key, store.policy.RecordTTL()).SetVal(true)
	mock.ExpectSAdd("hd-auth-sessions:subject:alice", "sid-1").SetVal(1)
	mock.ExpectExpire("hd-auth-sessions:subject:alice", store.policy.RecordTTL()).SetVal(true)
	mock.ExpectTxPipelineExec()

	err := store.Register(context.Background(), Record{SID: "sid-1", Subject: "alice", Provider: "auth-github", IssuedAt: now, LastSeen: now})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestRedisStoreRevokePreservesTombstone(t *testing.T) {
	client, mock := redismock.NewClientMock()
	store := NewRedisStore(client, Policy{TokenValidity: 24 * time.Hour})

	key := "hd-auth-session:sid-1"
	// Revoke loads the existing record (last_seen 1000 preserved) and rewrites
	// it with revoked=true.
	mock.ExpectHGetAll(key).SetVal(map[string]string{
		"subject":   "alice",
		"provider":  "auth-github",
		"issued_at": "1000",
		"last_seen": "1000",
		"revoked":   "false",
	})
	mock.ExpectTxPipeline()
	mock.ExpectHSet(key, map[string]interface{}{
		"subject":   "alice",
		"provider":  "auth-github",
		"issued_at": "1000",
		"last_seen": "1000",
		"revoked":   "1",
	}).SetVal(1)
	mock.ExpectExpire(key, store.policy.RecordTTL()).SetVal(true)
	mock.ExpectSAdd("hd-auth-sessions:subject:alice", "sid-1").SetVal(1)
	mock.ExpectExpire("hd-auth-sessions:subject:alice", store.policy.RecordTTL()).SetVal(true)
	mock.ExpectTxPipelineExec()

	if err := store.Revoke(context.Background(), "sid-1"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
}

func TestRedisStoreUnavailableMapsToErrUnavailable(t *testing.T) {
	client, mock := redismock.NewClientMock()
	store := NewRedisStore(client, Policy{TokenValidity: 24 * time.Hour})

	mock.ExpectHGetAll("hd-auth-session:s").SetErr(errors.New("connection refused"))

	_, err := store.Get(context.Background(), "s")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable on outage, got %v", err)
	}
}

func TestRedisStoreRevokeAllForSubjectPrunesStale(t *testing.T) {
	client, mock := redismock.NewClientMock()
	store := NewRedisStore(client, Policy{TokenValidity: 24 * time.Hour})

	// Subject index holds a live sid (has a session record) and a stale sid
	// (record already GC'd). Revoke-all must only revoke the live one and prune
	// the stale one from the index (F4).
	indexKey := "hd-auth-sessions:subject:alice"
	mock.ExpectSMembers(indexKey).SetVal([]string{"live-sid", "stale-sid"})
	mock.ExpectTxPipeline()
	mock.ExpectHExists("hd-auth-session:live-sid", "subject").SetVal(true)
	mock.ExpectHExists("hd-auth-session:stale-sid", "subject").SetVal(false)
	mock.ExpectTxPipelineExec()
	mock.ExpectSRem(indexKey, "stale-sid").SetVal(1)
	mock.ExpectTxPipeline()
	mock.ExpectHSet("hd-auth-session:live-sid", "revoked", "1").SetVal(1)
	mock.ExpectExpire("hd-auth-session:live-sid", store.policy.RecordTTL()).SetVal(true)
	mock.ExpectTxPipelineExec()

	revoked, err := store.RevokeAllForSubject(context.Background(), "alice")
	if err != nil {
		t.Fatalf("RevokeAllForSubject: %v", err)
	}
	if len(revoked) != 1 || revoked[0] != "live-sid" {
		t.Fatalf("expected only live-sid revoked, got %v", revoked)
	}
}

func TestRedisStoreRevokeAllForSubjectDropsEmptyIndex(t *testing.T) {
	client, mock := redismock.NewClientMock()
	store := NewRedisStore(client, Policy{TokenValidity: 24 * time.Hour})

	indexKey := "hd-auth-sessions:subject:bob"
	// All indexed sids are stale (records GC'd): nothing to revoke, and the now
	// empty subject index is dropped entirely.
	mock.ExpectSMembers(indexKey).SetVal([]string{"gone-sid"})
	mock.ExpectTxPipeline()
	mock.ExpectHExists("hd-auth-session:gone-sid", "subject").SetVal(false)
	mock.ExpectTxPipelineExec()
	mock.ExpectSRem(indexKey, "gone-sid").SetVal(1)
	mock.ExpectDel(indexKey).SetVal(1)

	revoked, err := store.RevokeAllForSubject(context.Background(), "bob")
	if err != nil {
		t.Fatalf("RevokeAllForSubject: %v", err)
	}
	if len(revoked) != 0 {
		t.Fatalf("expected no revoked sids, got %v", revoked)
	}
}

func TestRedisStorePruneSubject(t *testing.T) {
	client, mock := redismock.NewClientMock()
	store := NewRedisStore(client, Policy{TokenValidity: 24 * time.Hour})

	indexKey := "hd-auth-sessions:subject:carol"
	mock.ExpectSMembers(indexKey).SetVal([]string{"live", "stale"})
	mock.ExpectTxPipeline()
	mock.ExpectHExists("hd-auth-session:live", "subject").SetVal(true)
	mock.ExpectHExists("hd-auth-session:stale", "subject").SetVal(false)
	mock.ExpectTxPipelineExec()
	mock.ExpectSRem(indexKey, "stale").SetVal(1)

	if err := store.PruneSubject(context.Background(), "carol"); err != nil {
		t.Fatalf("PruneSubject: %v", err)
	}
}
