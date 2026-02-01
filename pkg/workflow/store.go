// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/op/go-logging"
)

const (
	// StoreSessionIdCounter is the counter used for reserving unique session IDs.
	StoreSessionIDCounter = "session"
	// StoreSessionPrefix is the prefix for persisted session stacks.
	StoreSessionPrefix = "stack:"
	// StoreEvenPrefix is the prefix for mapping events to persisted session stacks.
	StoreEventPrefix = "event:"
	// StoreSessionIdWrap is the number when reached causes the session ID counter to reset.
	StoreSessionIDWrap = 10000000000
	// StoreSessionIdBatch is the number of IDs to reserve each time.
	StoreSessionIDBatch = 100
	// StoreSessionCounter is the name for the counter to track live sessions.
	StoreSessionCounter = "counter:session"
)

var (
	// ErrSessionNotFound is an error when looking up a session that doesn't exist.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionTerminated is an error when resuming or continuing a terminated session.
	ErrSessionTerminated = errors.New("session already terminated")
)

// StoreHelper represents objects that offers functions for sessions and the store to use.
type StoreHelper interface {
	dipper.RPCCaller
	GetConfig() *config.Config
	SendMessage(msg *dipper.Message)
}

// Store is an object that manages the sessions.
type Store interface {
	StoreHelper

	// CreateChildSession creates a child session with the given parent.
	CreateChildSession(parent *Session, wf *config.Workflow, msg *dipper.Message) *Session
	// CreateAsyncChildSession creates a child session that is detached from the parent.
	CreateAsyncChildSession(parent *Session, wf *config.Workflow, msg *dipper.Message) *Session
	// ActivateSession activates a session to progress from its current state.
	ActivateSession(w *Session)
	// EmitResult emits the result of the session to a storage watched by consumers.
	EmitResult(w *Session)

	// StartSession starts a predefined workflow with a message and context variables.
	StartSession(wf *config.Workflow, msg *dipper.Message, ctx map[string]interface{})
	// StartDynamicSession starts a workflow constructed from a message payload.
	StartDynamicSession(spec *dipper.Message, ctx map[string]interface{})
	// ContinueSession continues a session with given dipper message.
	ContinueSession(ID string, msg *dipper.Message)
	// ResumeSession resumes a session that is in waiting state.
	ResumeSession(key string, msg *dipper.Message) bool

	// GetNumSessions returns the number of live sessions in the system.
	GetNumSessions(getAll bool) int
	// DumpSessions dumps the information of sessions for debugging.
	DumpSessions(cursor string) map[string]any
	// Wait waits for a session to be done.
	Wait()

	// GetLogger returns the logger  used by the session store.
	GetLogger() *logging.Logger
}

// NewStore initialize the session store.
func NewStore(helper StoreHelper) Store {
	s := &PersistedStore{
		StoreHelper: helper,
		Logger:      dipper.GetLogger("workflow", "INFO"),
		storeID:     dipper.GetIP(),
		idLock:      &sync.Mutex{},
	}

	return s
}

// GetStoredSession gets a stored session from the cache.
func GetStoredSession(caller dipper.RPCCaller, sessionID string) *Session {
	key := StoreSessionPrefix + sessionID
	resp := dipper.Must(caller.Call("cache", "lrange", map[string]interface{}{
		"key":  key,
		"stop": 0,
		"raw":  true,
	}))

	// callers may return a nil slice (typed []byte) which is not equal to nil
	// when stored in an interface.  Guard against empty payloads so we don't
	// panic when attempting to unmarshal.
	b := resp.([]byte)
	if len(b) == 0 {
		return nil
	}

	w := &Session{}
	w.Unmarshal(b)

	return w
}

// Wait waits for a session to be done.
func Wait(ctx context.Context, caller dipper.RPCCaller, eventID string, key string) *Session {
	for {
		sessionID := string(dipper.Must(caller.Call("cache", "load", map[string]any{"key": StoreEventPrefix + eventID})).([]byte))
		w := GetStoredSession(caller, sessionID)
		if w == nil {
			panic(fmt.Errorf("%w for event: %s", ErrSessionNotFound, eventID))
		}

		if w.State == SessionStateDone {
			return w
		}
		_, err := caller.Call("driver:redispubsub", "expect", map[string]interface{}{
			"topic":   "workflow",
			"subject": BroadcaseSubjectResult,
			"match": map[string]any{
				"labels": map[string]any{
					"sessionID": sessionID,
				},
			},
		}, "timeout", "10s", "key", key)
		if err != nil && !errors.Is(err, dipper.ErrTimeout) {
			panic(err)
		}
		select {
		case <-ctx.Done():
			panic(ctx.Err())
		default:
		}
	}
}
