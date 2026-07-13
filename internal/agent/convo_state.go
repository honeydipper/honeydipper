package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// ConvoState cache-key prefix, stream prefix, and session-status values.
const (
	ConvoStateKeyPrefix = "convo_state:"
	// ConvoTurnLockPrefix is the distributed-lock key prefix used to serialise
	// chat turns on the same conversation. Only one turn may execute at a time
	// per conversation; subsequent turns block until the current one finishes.
	// The locker driver polls every 100ms while the key is held, so no
	// additional busy-wait is needed.
	ConvoTurnLockPrefix = "convo_turn:"
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
	SessionID    string `json:"session_id"`
	AgentName    string `json:"agent_name"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	ErrorReason  string `json:"error_reason,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	TotalTokens  int    `json:"total_tokens,omitempty"`
	// ContextTokens tracks the token count of the current history including
	// system message, persisted for incremental counting across round trips.
	ContextTokens int       `json:"context_tokens,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ConvoState is the persisted, queryable view of a conversation.
// It tracks the first and last agent session started in the conversation and
// exposes a Cancelled flag that active sessions poll to self-terminate.
// Generation tracks how many times the conversation has been compacted;
// each compaction archives the previous history under <ConvoID>_g<N>.
type ConvoState struct {
	ConvoID        string           `json:"convo_id"`
	UnifiedConvoID string           `json:"unified_convo_id,omitempty"`
	FirstTurn      string           `json:"first_turn,omitempty"`
	FirstSession   *ConvoSessionRef `json:"first_session,omitempty"`
	LastSession    *ConvoSessionRef `json:"last_session,omitempty"`
	ActiveSession  *ConvoSessionRef `json:"active_session,omitempty"`
	Cancelled      bool             `json:"cancelled"`
	// TotalTokens accumulates the sum of input+output tokens for
	// completed/failed sessions that belonged to this conversation.
	TotalTokens int `json:"total_tokens,omitempty"`
	// ContextTokens tracks the token count of the current history including
	// system message, persisted for incremental counting across round trips.
	ContextTokens  int      `json:"context_tokens,omitempty"`
	Generation     int      `json:"generation"`
	ArchivedConvos []string `json:"archived_convos,omitempty"`
	TTL            string   `json:"ttl"`

	Agent  *config.Agent     `json:"agent,omitempty"`
	Skills map[string]string `json:"skills,omitempty"`
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

// registerSession records the new active session and optionally updates
// LastSession. When registering child or unified sessions set updateLast
// to false so UI-visible LastSession isn't overwritten by internal workers.
// The caller is responsible for persisting the state afterwards.
func (cs *ConvoState) registerSession(sessionID, agentName, sessionType string, updateLast bool) {
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
	if updateLast {
		cs.LastSession = sr
	}
	// Track the currently active session separately so control operations
	// (cancel) can target the active worker without affecting UI-visible
	// First/Last session semantics.
	cs.ActiveSession = sr
}

// updateSessionStatus sets the status, token counts, and updated timestamp
// for the matching session in FirstSession or LastSession.
// The caller is responsible for persisting.
func (cs *ConvoState) updateSessionStatus(sessionID, status string, reason string, inputTokens, outputTokens int, totalTokens int) {
	now := time.Now()
	// Avoid double-adding when multiple refs point to the same session
	// (First/Last/Active). Track which session IDs we've already accounted
	// for in this update call.
	accounted := false

	updateRef := func(ref *ConvoSessionRef, updateActive bool) {
		if ref == nil || ref.SessionID != sessionID {
			return
		}
		ref.InputTokens = inputTokens
		ref.OutputTokens = outputTokens
		ref.TotalTokens = totalTokens
		ref.UpdatedAt = now

		if ref.Status != ConvoSessionStatusComplete && ref.Status != ConvoSessionStatusFailed {
			if status == ConvoSessionStatusComplete || status == ConvoSessionStatusFailed {
				if !accounted && !updateActive {
					cs.TotalTokens += totalTokens
					accounted = true
				}
			}
		}

		ref.Status = status
		ref.ErrorReason = reason
	}

	updateRef(cs.FirstSession, false)
	updateRef(cs.LastSession, false)
	updateRef(cs.ActiveSession, true)
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
	if cs.ActiveSession != nil && cs.ActiveSession.SessionID == sessionID {
		return cs.ActiveSession.Status == ConvoSessionStatusCancelled
	}

	return false
}

// archiveConvo copies the current conversation history to an archived key
// suffixed with _g<N> where N is the incremented generation number.
// It increments cs.Generation and returns the archived key name.
// The caller is responsible for persisting the updated ConvoState.
//
// The archived key follows the pattern: convo_history:<ConvoID>_g<N>
// This allows the UI to discover archived generations by convention.
func (cs *ConvoState) archiveConvo(store AgentStore) (string, error) {
	// Read the current conversation history.
	ret, err := store.Call("cache", "lrange", map[string]interface{}{
		"key": ConvoHistoryKeyPrefix + cs.ConvoID,
	})
	if err != nil {
		return "", fmt.Errorf("archiveConvo: %w", err)
	}

	// Increment generation before building the key so the first
	// archive gets _g1, the second _g2, etc.
	cs.Generation++
	archivedKey := cs.ConvoID + "_g" + strconv.Itoa(cs.Generation)

	// Only copy if there are entries in the current history.
	if len(ret) > 0 {
		var messages []json.RawMessage
		if err := json.Unmarshal(ret, &messages); err != nil {
			// Roll back the generation increment on parse failure.
			cs.Generation--

			return "", fmt.Errorf("archiveConvo: %w", err)
		}

		convoTTL, _ := time.ParseDuration(ConvoStreamTTL)
		fullKey := ConvoHistoryKeyPrefix + archivedKey
		for _, msg := range messages {
			_, err := store.Call("cache", "rpush", map[string]interface{}{
				"key":   fullKey,
				"value": string(msg),
				"ttl":   float64(convoTTL),
			})
			if err != nil {
				// Best-effort: try to push remaining messages.
				// If individual pushes fail, the archive is partial.
				// The generation is already incremented so the next
				// compaction will use _g<N+1> — the partial archive
				// remains discoverable with a best-effort copy.
				continue
			}
		}
	}

	// Record the archived generation in the ConvoState so callers can expose
	// the list of archived generations to clients. The caller is responsible
	// for persisting the ConvoState (lockedConvoStateUpdate will persist
	// after the fn returns when used).
	cs.ArchivedConvos = append(cs.ArchivedConvos, archivedKey)

	return archivedKey, nil
}

// lockedConvoStateUpdate acquires the distributed lock for convoID, loads the
// latest state, calls fn with the mutable *ConvoState, persists the result,
// and then releases the lock.  It is a no-op when convoID is empty.
// Optional agentConfig and labels parameters allow configurable lock expiration:
// - agentConfig.TurnLockTimeout is used if set (fallback to AgentSessionDefaultTurnLockExpire)
// - labels["timeout"] overrides everything if present.
func lockedConvoStateUpdate(convoID string, store AgentStore, fn func(*ConvoState), opts ...LockedConvoStateUpdateOpts) {
	if convoID == "" {
		return
	}
	// Determine lock expiration with same priority as turn lock:
	// 1. Default: AgentSessionDefaultTurnLockExpire ("1h")
	// 2. Agent config: agentConfig.TurnLockTimeout (if provided and non-empty)
	// 3. Message label: labels["timeout"] (if provided and non-empty)
	expire := AgentSessionDefaultTurnLockExpire
	if len(opts) > 0 {
		if opts[0].AgentConfig != nil && opts[0].AgentConfig.TurnLockTimeout != "" {
			expire = opts[0].AgentConfig.TurnLockTimeout
		}
		if opts[0].Labels != nil {
			if timeoutLabel := opts[0].Labels["timeout"]; timeoutLabel != "" {
				expire = timeoutLabel
			}
		}
	}
	lockKey := ConvoStateKeyPrefix + convoID
	dipper.Must(store.Call("locker", "lock", map[string]interface{}{
		"name":   lockKey,
		"expire": expire,
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

// LockedConvoStateUpdateOpts holds optional parameters for lockedConvoStateUpdate.
type LockedConvoStateUpdateOpts struct {
	AgentConfig *config.Agent
	Labels      map[string]string
}
