// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/stretchr/testify/assert"
)

// TestCheckCancelled_ActiveSessionNotCancelled verifies that the active session
// is not considered cancelled.
func TestCheckCancelled_ActiveSessionNotCancelled(t *testing.T) {
	store := newMockStore(nil)
	cfg := store.GetConfig()
	cfg.DataSet.Agents["test-agent"] = config.Agent{Name: "test-agent"}

	// Create a ConvoState with session-1 as ActiveSession
	cs := &ConvoState{
		ConvoID:       "convo-1",
		FirstSession:  &ConvoSessionRef{SessionID: "session-1", Status: ConvoSessionStatusActive},
		LastSession:   &ConvoSessionRef{SessionID: "session-1", Status: ConvoSessionStatusActive},
		ActiveSession: &ConvoSessionRef{SessionID: "session-1", Status: ConvoSessionStatusActive},
	}
	cs.persist(store)

	// Create session matching ActiveSession
	s := &AgentSession{
		store:   store,
		Agent:   &config.Agent{Name: "test-agent"},
		ID:      "session-1",
		ConvoID: "convo-1",
	}

	// Should NOT be cancelled (it's the active session)
	assert.False(t, s.checkCancelled())
}

// TestCheckCancelled_NonActiveSessionIsCancelled verifies that a session that
// is not the active session is considered cancelled.
func TestCheckCancelled_NonActiveSessionIsCancelled(t *testing.T) {
	store := newMockStore(nil)
	cfg := store.GetConfig()
	cfg.DataSet.Agents["test-agent"] = config.Agent{Name: "test-agent"}

	// Create a ConvoState where session-2 is the active session
	cs := &ConvoState{
		ConvoID:       "convo-1",
		FirstSession:  &ConvoSessionRef{SessionID: "session-1", Status: ConvoSessionStatusComplete},
		LastSession:   &ConvoSessionRef{SessionID: "session-2", Status: ConvoSessionStatusActive},
		ActiveSession: &ConvoSessionRef{SessionID: "session-2", Status: ConvoSessionStatusActive},
	}
	cs.persist(store)

	// Create session-1 (no longer active)
	s := &AgentSession{
		store:   store,
		Agent:   &config.Agent{Name: "test-agent"},
		ID:      "session-1",
		ConvoID: "convo-1",
	}

	// Should be cancelled (it's not the active session)
	assert.True(t, s.checkCancelled())
}

// TestCheckCancelled_ExplicitCancelledSession verifies that an explicitly
// cancelled session is detected as cancelled.
func TestCheckCancelled_ExplicitCancelledSession(t *testing.T) {
	store := newMockStore(nil)
	cfg := store.GetConfig()
	cfg.DataSet.Agents["test-agent"] = config.Agent{Name: "test-agent"}

	// Create a ConvoState where session-1 is explicitly cancelled
	cs := &ConvoState{
		ConvoID:       "convo-1",
		FirstSession:  &ConvoSessionRef{SessionID: "session-1", Status: ConvoSessionStatusCancelled},
		LastSession:   &ConvoSessionRef{SessionID: "session-2", Status: ConvoSessionStatusActive},
		ActiveSession: &ConvoSessionRef{SessionID: "session-2", Status: ConvoSessionStatusActive},
	}
	cs.persist(store)

	// Create session-1 (explicitly cancelled)
	s := &AgentSession{
		store:   store,
		Agent:   &config.Agent{Name: "test-agent"},
		ID:      "session-1",
		ConvoID: "convo-1",
	}

	// Should be cancelled (explicitly marked as cancelled)
	assert.True(t, s.checkCancelled())
}
