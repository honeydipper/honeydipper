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
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisConfig carries the connection settings for the session store.
type RedisConfig struct {
	Addr     string `json:"addr"`
	Username string `json:"username"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// RedisStore is a Redis-backed Store implementation. All session records are
// keyed by sid, with a per-subject index enabling revoke-all-for-subject.
type RedisStore struct {
	client *redis.Client
	policy Policy
}

// NewRedisStore constructs a RedisStore.
func NewRedisStore(client *redis.Client, policy Policy) *RedisStore {
	return &RedisStore{client: client, policy: policy}
}

// NewRedisClient builds a go-redis client from the connection config.
func NewRedisClient(cfg RedisConfig) *redis.Client {
	opts := &redis.Options{Addr: cfg.Addr, Username: cfg.Username, Password: cfg.Password, DB: cfg.DB}
	if opts.Addr == "" {
		opts.Addr = "localhost:6379"
	}

	return redis.NewClient(opts)
}

func sidKey(sid string) string {
	return "hd-auth-session:" + sid
}

func subjectKey(subject string) string {
	return "hd-auth-sessions:subject:" + subject
}

// Register eagerly creates or updates a session.
func (s *RedisStore) Register(ctx context.Context, r Record) error {
	existing, err := s.load(ctx, r.SID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return MapErr(err)
	}
	// Preserve issued_at (never reset on rotation) and the revoked tombstone.
	if err == nil {
		if existing.Revoked {
			r.Revoked = true
		}
		if existing.IssuedAt > 0 {
			r.IssuedAt = existing.IssuedAt
		}
	}
	if r.IssuedAt == 0 {
		r.IssuedAt = time.Now().Unix()
	}
	if r.LastSeen == 0 {
		r.LastSeen = time.Now().Unix()
	}

	return s.save(ctx, r)
}

// Touch refreshes LastSeen, lazily registering an unknown-but-valid sid.
func (s *RedisStore) Touch(ctx context.Context, sid, subject, provider string, issuedAt int64) (Record, error) {
	existing, err := s.load(ctx, sid)
	if err == nil {
		if existing.Revoked {
			return existing, nil
		}
		existing.LastSeen = time.Now().Unix()
		if err := s.save(ctx, existing); err != nil {
			return existing, err
		}

		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Record{}, MapErr(err)
	}

	// Lazy registration / backfill for rollout (requirement 4).
	now := time.Now().Unix()
	r := Record{SID: sid, Subject: subject, Provider: provider, IssuedAt: issuedAt, LastSeen: now}
	if r.IssuedAt == 0 {
		r.IssuedAt = now
	}
	if err := s.save(ctx, r); err != nil {
		return Record{}, err
	}

	return r, nil
}

// Get returns the current record for a sid.
func (s *RedisStore) Get(ctx context.Context, sid string) (Record, error) {
	return s.load(ctx, sid)
}

func (s *RedisStore) load(ctx context.Context, sid string) (Record, error) {
	vals, err := s.client.HGetAll(ctx, sidKey(sid)).Result()
	if err != nil {
		return Record{}, MapErr(err)
	}
	if len(vals) == 0 {
		return Record{}, ErrNotFound
	}
	r := decodeRecord(vals)
	r.SID = sid

	return r, nil
}

func (s *RedisStore) save(ctx context.Context, r Record) error {
	pipe := s.client.TxPipeline()
	key := sidKey(r.SID)
	ttl := s.policy.RecordTTL()
	pipe.HSet(ctx, key, map[string]interface{}{
		"subject":   r.Subject,
		"provider":  r.Provider,
		"issued_at": strconv.FormatInt(r.IssuedAt, 10),
		"last_seen": strconv.FormatInt(r.LastSeen, 10),
		"revoked":   boolToStr(r.Revoked),
	})
	pipe.Expire(ctx, key, ttl)
	if r.Subject != "" {
		pipe.SAdd(ctx, subjectKey(r.Subject), r.SID)
		pipe.Expire(ctx, subjectKey(r.Subject), ttl)
	}
	_, err := pipe.Exec(ctx)

	return MapErr(err)
}

// Revoke marks a single sid as revoked, retaining the record as a tombstone
// with a TTL that outlives any remaining token validity (requirement 3).
func (s *RedisStore) Revoke(ctx context.Context, sid string) error {
	existing, err := s.load(ctx, sid)
	if err == nil {
		existing.Revoked = true
		if err := s.save(ctx, existing); err != nil {
			return MapErr(err)
		}

		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return MapErr(err)
	}

	// Unknown sid: record a bare tombstone so any replayed token for this sid
	// remains rejected for the remainder of its possible validity window.
	return MapErr(s.save(ctx, Record{SID: sid, Revoked: true, LastSeen: time.Now().Unix()}))
}

// RevokeAllForSubject revokes every session for a subject and returns the
// revoked sids so callers can invalidate local caches.
func (s *RedisStore) RevokeAllForSubject(ctx context.Context, subject string) ([]string, error) {
	key := subjectKey(subject)
	sids, err := s.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, MapErr(err)
	}
	revoked := make([]string, 0, len(sids))
	pipe := s.client.TxPipeline()
	for _, sid := range sids {
		pipe.HSet(ctx, sidKey(sid), "revoked", "1")
		pipe.Expire(ctx, sidKey(sid), s.policy.RecordTTL())
		revoked = append(revoked, sid)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, MapErr(err)
	}

	return revoked, nil
}

func decodeRecord(vals map[string]string) Record {
	r := Record{}
	r.Subject = vals["subject"]
	r.Provider = vals["provider"]
	if issuedAt, err := strconv.ParseInt(vals["issued_at"], 10, 64); err == nil {
		r.IssuedAt = issuedAt
	}
	if lastSeen, err := strconv.ParseInt(vals["last_seen"], 10, 64); err == nil {
		r.LastSeen = lastSeen
	}
	r.Revoked, _ = strconv.ParseBool(vals["revoked"])

	return r
}

func boolToStr(b bool) string {
	if b {
		return "1"
	}

	return "false"
}

// MapErr maps raw redis errors onto session errors. A store outage is surfaced
// as ErrUnavailable so callers can fail closed.
func MapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return ErrNotFound
	}

	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}
