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
	SessionID    string    `json:"session_id"`
	AgentName    string    `json:"agent_name"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ConvoState is the persisted, queryable view of a conversation.
// It tracks the first and last agent session started in the conversation and
// exposes a Cancelled flag that active sessions poll to self-terminate.
type ConvoState struct {
	ConvoID        string           `json:"convo_id"`
	UnifiedConvoID string           `json:"unified_convo_id,omitempty"`
	FirstTurn      string           `json:"first_turn,omitempty"`
	FirstSession   *ConvoSessionRef `json:"first_session,omitempty"`
	LastSession    *ConvoSessionRef `json:"last_session,omitempty"`
	Cancelled      bool             `json:"cancelled"`
	TTL            string           `json:"ttl"`
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

// registerSession records the new active session as the LastSession, and sets
// FirstSession when this is the first session in the conversation.
// The caller is responsible for persisting the state afterwards.
func (cs *ConvoState) registerSession(sessionID, agentName, sessionType string) {
	now := time.Now()
	sr := &ConvoSessionRef{
		SessionID: sessionID,
		AgentName: agentName,
		Type:      sessionType,
		Status:    ConvoSessionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if cs.FirstSession == nil {
		cs.FirstSession = sr
	}
	cs.LastSession = sr
}

// updateSessionStatus sets the status, token counts, and updated timestamp
// for the matching session in FirstSession or LastSession.
// The caller is responsible for persisting.
func (cs *ConvoState) updateSessionStatus(sessionID, status string, inputTokens, outputTokens int) {
	now := time.Now()
	if cs.FirstSession != nil && cs.FirstSession.SessionID == sessionID {
		cs.FirstSession.Status = status
		cs.FirstSession.InputTokens = inputTokens
		cs.FirstSession.OutputTokens = outputTokens
		cs.FirstSession.UpdatedAt = now
	}
	if cs.LastSession != nil && cs.LastSession.SessionID == sessionID {
		cs.LastSession.Status = status
		cs.LastSession.InputTokens = inputTokens
		cs.LastSession.OutputTokens = outputTokens
		cs.LastSession.UpdatedAt = now
	}
}

// isSessionCancelled returns true when the session with the given ID has been
// marked as cancelled in FirstSession or LastSession.
// It is used by checkCancelled to detect turn-level cancellation without
// polluting future turns via the whole-conversation Cancelled flag.
func (cs *ConvoState) isSessionCancelled(sessionID string) bool {
	if cs.FirstSession != nil && cs.FirstSession.SessionID == sessionID {
		return cs.FirstSession.Status == ConvoSessionStatusCancelled
	}
	if cs.LastSession != nil && cs.LastSession.SessionID == sessionID {
		return cs.LastSession.Status == ConvoSessionStatusCancelled
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
