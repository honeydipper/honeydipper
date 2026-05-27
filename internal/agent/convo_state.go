// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package agent

import (
	"encoding/json"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// ConvoState cache-key prefix, stream prefix, and session-status values.
const (
	ConvoStateKeyPrefix = "convo_state:"
	// ConvoStream is the stream_hset prefix used to track convo-state changes.
	ConvoStream = "convo_stream_"
	// ConvoStreamTTL is how long each 2-hour stream block is kept in Redis.
	// Blocks are grouped in 2-hour windows; the UI queries the latest block
	// and can paginate back up to 30 days via the look_back parameter.
	ConvoStreamTTL = "720h"

	ConvoSessionStatusActive    = "active"
	ConvoSessionStatusComplete  = "complete"
	ConvoSessionStatusFailed    = "failed"
	ConvoSessionStatusCancelled = "cancelled"
)

// ConvoSessionRef is a compact record of an agent session that belongs to a conversation.
type ConvoSessionRef struct {
	SessionID string    `json:"session_id"`
	AgentName string    `json:"agent_name"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConvoState is the persisted, queryable view of a conversation.
// It tracks every agent session started in the conversation and exposes a
// Cancelled flag that active sessions poll to self-terminate.
type ConvoState struct {
	ConvoID        string            `json:"convo_id"`
	UnifiedConvoID string            `json:"unified_convo_id,omitempty"`
	Sessions       []ConvoSessionRef `json:"sessions"`
	Cancelled      bool              `json:"cancelled"`
	TTL            string            `json:"ttl"`
}

// load reads and deserialises the ConvoState for convoID from the cache.
// When no cache entry exists the receiver is left as an empty state with
// ConvoID populated; this is the normal path for a new conversation.
func (cs *ConvoState) load(convoID string, store AgentStore) {
	cs.ConvoID = convoID
	ret, err := store.Call("cache", "load", map[string]interface{}{
		"key": ConvoStateKeyPrefix + convoID,
	})
	if err != nil {
		return // new conversation – start with an empty state
	}
	if len(ret) == 0 {
		return
	}
	// Ignore unmarshal errors; the empty state set above is the safe fallback.
	_ = json.Unmarshal(ret, cs)
	cs.ConvoID = convoID // canonical key always wins
}

// persist serialises and writes the ConvoState to the cache and appends an
// entry to the convo stream so the UI can query recent state changes.
func (cs *ConvoState) persist(store AgentStore) {
	encoded := string(dipper.SerializeContent(cs))
	dipper.Must(store.Call("cache", "save", map[string]interface{}{
		"key":   ConvoStateKeyPrefix + cs.ConvoID,
		"value": encoded,
		"ttl":   cs.TTL,
	}))
	dipper.Must(store.Call("cache", "stream_hset", map[string]interface{}{
		"prefix": ConvoStream,
		"key":    cs.ConvoID,
		"value":  encoded,
		"ttl":    ConvoStreamTTL,
	}))
}

// registerSession appends a new active-session entry to the in-memory list.
// The caller is responsible for persisting the state afterwards.
func (cs *ConvoState) registerSession(sessionID, agentName, sessionType string) {
	now := time.Now()
	cs.Sessions = append(cs.Sessions, ConvoSessionRef{
		SessionID: sessionID,
		AgentName: agentName,
		Type:      sessionType,
		Status:    ConvoSessionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// updateSessionStatus finds the entry for sessionID, sets its status and
// updated timestamp in-memory.  The caller is responsible for persisting.
func (cs *ConvoState) updateSessionStatus(sessionID, status string) {
	for i := range cs.Sessions {
		if cs.Sessions[i].SessionID == sessionID {
			cs.Sessions[i].Status = status
			cs.Sessions[i].UpdatedAt = time.Now()

			return
		}
	}
}

// isSessionCancelled returns true when the session with the given ID has been
// marked as cancelled inside this ConvoState's session list.
// It is used by checkCancelled to detect turn-level cancellation without
// polluting future turns via the whole-conversation Cancelled flag.
func (cs *ConvoState) isSessionCancelled(sessionID string) bool {
	for _, sr := range cs.Sessions {
		if sr.SessionID == sessionID {
			return sr.Status == ConvoSessionStatusCancelled
		}
	}

	return false
}

// lockedConvoStateUpdate acquires the distributed lock for convoID, loads the
// latest state, calls fn with the mutable *ConvoState, persists the result,
// and then releases the lock.  It is a no-op when convoID is empty.
func lockedConvoStateUpdate(convoID string, store AgentStore, fn func(*ConvoState)) {
	if convoID == "" {
		return
	}
	lockKey := ConvoStateKeyPrefix + convoID
	dipper.Must(store.Call("locker", "lock", map[string]interface{}{
		"name":   lockKey,
		"expire": "30s",
	}))
	defer func() {
		dipper.Must(store.Call("locker", "unlock", map[string]interface{}{
			"name": lockKey,
		}))
	}()
	cs := &ConvoState{}
	cs.load(convoID, store)
	fn(cs)
	cs.persist(store)
}
