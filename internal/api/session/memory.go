// Package session (test double): in-memory Store implementation.
//
// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package session

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store implementation for tests and for local
// single-node operation when no Redis is configured. It mirrors the semantics
// of RedisStore so policy logic can be unit-tested without a Redis server.
type MemoryStore struct {
	mu        sync.RWMutex
	records   map[string]Record
	subjectTo map[string]map[string]bool
}

// NewMemoryStore builds an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records:   map[string]Record{},
		subjectTo: map[string]map[string]bool{},
	}
}

// Register implements Store.
func (m *MemoryStore) Register(ctx context.Context, r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.records[r.SID]; ok {
		if existing.Revoked {
			r.Revoked = true
		}
		if existing.IssuedAt > 0 {
			r.IssuedAt = existing.IssuedAt
		}
	}
	if m.subjectTo[r.Subject] == nil {
		m.subjectTo[r.Subject] = map[string]bool{}
	}
	m.subjectTo[r.Subject][r.SID] = true
	m.records[r.SID] = r

	return nil
}

// Touch implements Store.
func (m *MemoryStore) Touch(ctx context.Context, sid, subject, provider string, issuedAt int64) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.records[sid]; ok {
		if existing.Revoked {
			return existing, nil
		}
		existing.LastSeen = time.Now().Unix()
		m.records[sid] = existing

		return existing, nil
	}
	now := time.Now().Unix()
	r := Record{SID: sid, Subject: subject, Provider: provider, IssuedAt: issuedAt, LastSeen: now}
	if r.IssuedAt == 0 {
		r.IssuedAt = now
	}
	if m.subjectTo[r.Subject] == nil {
		m.subjectTo[r.Subject] = map[string]bool{}
	}
	m.subjectTo[r.Subject][sid] = true
	m.records[sid] = r

	return r, nil
}

// Get implements Store.
func (m *MemoryStore) Get(ctx context.Context, sid string) (Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.records[sid]
	if !ok {
		return Record{}, ErrNotFound
	}

	return r, nil
}

// Revoke implements Store.
func (m *MemoryStore) Revoke(ctx context.Context, sid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.records[sid]; ok {
		existing.Revoked = true
		m.records[sid] = existing

		return nil
	}
	m.records[sid] = Record{SID: sid, Revoked: true}

	return nil
}

// RevokeAllForSubject implements Store.
func (m *MemoryStore) RevokeAllForSubject(ctx context.Context, subject string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	revoked := []string{}
	for sid := range m.subjectTo[subject] {
		if existing, ok := m.records[sid]; ok {
			existing.Revoked = true
			m.records[sid] = existing
			revoked = append(revoked, sid)
		}
	}

	return revoked, nil
}

// Snapshot returns a shallow copy of all records (test helper).
func (m *MemoryStore) Snapshot() map[string]Record {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[string]Record, len(m.records))
	for k, v := range m.records {
		cp[k] = v
	}

	return cp
}
