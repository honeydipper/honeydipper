// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/daemon"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/op/go-logging"
)

const BroadcaseSubjectResult = "result"

// PersistredStore stores session using the cache driver to persist the sessions.
type PersistedStore struct {
	// Live is a flag used for draining the live sessions.
	Live sync.WaitGroup
	// StoreHelper provides the methods for workflow to access daemon.
	StoreHelper

	*logging.Logger
	nextID  int
	maxID   int
	idLock  sync.Locker
	storeID string
}

// CreateSession creates and initializes a workflow session.
func (s *PersistedStore) CreateSession(wf *config.Workflow, msg *dipper.Message, ctx map[string]interface{}) *Session {
	w := NewSession(s.GetNextID(), wf, s)
	w.Init(msg, nil, ctx)
	w.CurrentMsg.Labels["cursor"] = "0"

	s.persist(w)

	dipper.Must(s.CallNoWait("cache", "incr", map[string]any{"key": StoreSessionCounter}))
	dipper.Must(s.CallNoWait("cache", "incr", map[string]any{"key": StoreSessionCounter + ":" + s.storeID}))

	return w
}

// DetachSession detaches a session from its parent.
func (s *PersistedStore) DetachSession(w *Session) {
	w.ID = s.GetNextID()

	s.Infof("session [%s.%s] spin off session %s", w.parent.ID, w.CurrentMsg.Labels["cursor"], w.ID)

	w.Performing = []string{w.Performing[w.depth]}
	w.depth = 0
	w.EventID = w.EventID + ":" + w.ID

	msg := *w.CurrentMsg
	msg.Labels = map[string]string{}
	for k, v := range w.CurrentMsg.Labels {
		msg.Labels[k] = v
	}
	msg.Labels["sessionID"] = w.ID
	msg.Labels["cursor"] = "0"
	w.CurrentMsg = &msg
	w.parent = nil

	s.persist(w)
}

// CreateChildSession creates a child session with the given parent.
func (s *PersistedStore) CreateChildSession(parent *Session, wf *config.Workflow, msg *dipper.Message) *Session {
	w := NewSession(parent.ID, wf, s)
	w.Init(msg, parent, nil)
	if w.Workflow.Detach {
		s.DetachSession(w)
	}

	return w
}

// CreateAsyncChildSession creates a child async workflow session.
func (s *PersistedStore) CreateAsyncChildSession(parent *Session, wf *config.Workflow, msg *dipper.Message) *Session {
	w := s.CreateChildSession(parent, wf, msg)
	w.Parent = parent.ID + "." + parent.CurrentMsg.Labels["cursor"]
	s.DetachSession(w)

	return w
}

// ActivateSession activates a session to progress from its current state.
func (s *PersistedStore) ActivateSession(w *Session) {
	w.activate()
	if w.parent == nil {
		s.Live.Add(1)
		daemon.Go(func() {
			dipper.SafeExitOnError("[%s] panic when waiting for the session.", w.ID)
			defer s.Live.Done()
			defer s.persist(w)
			w.Wait()
		})
	}
}

// EmitResult emits the result of the session to a storage watched by consumers.
func (s *PersistedStore) EmitResult(w *Session) {
	w.CurrentMsg.Labels["sessionID"] = w.ID
	mesg := w.Dump()
	mesg["topic"] = "workflow"
	mesg["subject"] = BroadcaseSubjectResult
	dipper.Must(s.CallNoWait("driver:redispubsub", "send", mesg))
}

func (s *PersistedStore) uncaughtErrorHandler(w *Session, msg *dipper.Message) func(r error) {
	return func(r error) {
		if w == nil {
			w = &Session{store: s}
		}
		if w.EventID == "" {
			if msg != nil && msg.Labels != nil && msg.Labels["eventID"] != "" {
				w.EventID = msg.Labels["eventID"]
			}
		}
		if w.CurrentMsg == nil {
			w.CurrentMsg = msg
		}
		if w.CurrentMsg.Labels == nil {
			w.CurrentMsg.Labels = map[string]string{}
		}
		w.CurrentMsg.Labels["status"] = SessionStatusError
		w.CurrentMsg.Labels["reason"] = fmt.Sprintf("error when starting session: %+v", r)
		w.CurrentMsg.Labels["sessionID"] = w.ID
		w.CurrentMsg.Labels["eventID"] = w.EventID

		w.State = SessionStateDone

		if w.ID != "" {
			s.persist(w)
		}
		if w.Parent == "" && w.EventID != "" {
			if w.ID == "" {
				w.ID = s.GetNextID()
				s.persist(w)
			}
			s.EmitResult(w)
		}
	}
}

// StartSession starts a predefined workflow with a message and context variables.
func (s *PersistedStore) StartSession(wf *config.Workflow, msg *dipper.Message, ctx map[string]interface{}) {
	defer dipper.SafeExitOnError("[workflow] error when creating workflow session", s.uncaughtErrorHandler(nil, msg))

	w := s.CreateSession(wf, msg, ctx)
	defer dipper.SafeExitOnError("[workflow] error when starting workflow session", s.uncaughtErrorHandler(w, msg))
	s.ActivateSession(w)
}

// ParseDynamicWorkflow will parse a dynamic workflow execution request.
func (s *PersistedStore) ParseDynamicWorkflow(spec *dipper.Message) (*config.Workflow, *dipper.Message, map[string]interface{}) {
	wf := &config.Workflow{}
	msg := &dipper.Message{}
	ctx := map[string]interface{}{}

	dipper.Must(mapstructure.Decode(dipper.MustGetMapData(spec.Payload, "do"), wf))
	dipper.Must(mapstructure.Decode(dipper.MustGetMapData(spec.Payload, "message"), msg))
	if data, ok := dipper.GetMapData(spec.Payload, "data"); ok {
		// if data assertion fails, let it panic
		ctx = data.(map[string]interface{})
	}

	return wf, msg, ctx
}

// StartDynamicSession starts a workflow constructed from a message payload.
func (s *PersistedStore) StartDynamicSession(spec *dipper.Message, ctx map[string]interface{}) {
	defer dipper.SafeExitOnError("[workflow] error when starting dynamic workflow session", s.uncaughtErrorHandler(nil, spec))

	wf, msg, localCtx := s.ParseDynamicWorkflow(spec)
	ctx = dipper.MergeMap(ctx, localCtx)

	s.StartSession(wf, msg, ctx)
}

// ContinueSession continues a session with given dipper message.
func (s *PersistedStore) ContinueSession(sessionID string, msg *dipper.Message, child *Session) {
	defer dipper.SafeExitOnError("[workflow] error when loading workflow session %s", sessionID)
	w := s.loadSession(sessionID, msg, child)
	if w == nil {
		return
	}

	if w.State == SessionStateDone {
		panic(fmt.Errorf("%w: %s", ErrSessionTerminated, sessionID))
	}

	defer dipper.SafeExitOnError("[workflow] error when starting workflow session", s.uncaughtErrorHandler(w, msg))

	s.ActivateSession(w)
}

// ResumeSession resume a session that is in waiting state.
func (s *PersistedStore) ResumeSession(key string, msg *dipper.Message) bool {
	data, _ := s.Call("scheduler", "cancel", map[string]any{"type": "session", "key": key})
	if data == nil {
		return false
	}

	payload := dipper.DeserializeContent(data)
	s.Debugf("resuming session with key %s and payload %+v", key, payload)
	sessionID := dipper.MustGetMapDataStr(payload, "labels.sessionID")
	cursor := dipper.MustGetMapDataStr(payload, "labels.cursor")

	if msg.Labels == nil {
		msg.Labels = map[string]string{}
	}
	msg.Labels["sessionID"] = sessionID
	msg.Labels["cursor"] = cursor
	if _, ok := msg.Labels["status"]; !ok {
		msg.Labels["status"] = SessionStatusSuccess
	}

	daemon.Go(func() { s.ContinueSession(sessionID, msg, nil) })

	return true
}

// persist persists the session if it is not completed, otherwise clean up the session.
func (s *PersistedStore) persist(w *Session) {
	// Find the root session iteratively to avoid stack overflow
	root := w
	for root.parent != nil {
		root = root.parent
	}

	key := StoreSessionPrefix + root.ID
	if w.State != SessionStateInit {
		dipper.Must(s.Call("cache", "del", map[string]interface{}{
			"key": key,
		}))
	}

	var ttl time.Duration = 0
	if root.State == SessionStateDone {
		ttl = time.Hour * 24
	}

	current := root
	cursor := ""
	for {
		dipper.Must(s.Call("cache", "rpush", map[string]interface{}{
			"key":   key,
			"value": string(current.Marshal()),
			"ttl":   ttl,
		}))
		cursor = current.CurrentMsg.Labels["cursor"]
		if current.child == nil {
			break
		}
		current = current.child
	}

	if root.State == SessionStateInit {
		dipper.Must(s.Call("locker", "lock", map[string]interface{}{
			"name":   key,
			"expire": "3600s",
		}, "timeout", "3600s"))
		if root.Parent == "" {
			dipper.Must(s.Call("cache", "save", map[string]interface{}{
				"key":   StoreEventPrefix + w.EventID,
				"value": root.ID,
			}))
		}
	} else {
		dipper.Must(s.Call("locker", "unlock", map[string]interface{}{
			"name": key,
		}))
	}

	if root.State == SessionStateDone && root.Parent == "" {
		dipper.Must(s.CallNoWait("cache", "decr", map[string]any{"key": StoreSessionCounter}))
		dipper.Must(s.CallNoWait("cache", "decr", map[string]any{"key": StoreSessionCounter + ":" + s.storeID, "remove_zero": 1}))
		dipper.Must(s.Call("cache", "save", map[string]interface{}{
			"key":   StoreEventPrefix + w.EventID,
			"value": root.ID,
			"ttl":   "48h",
		}))
	}

	s.Infof("persisted session [%s.%s] depth %d state [%s]", root.ID, cursor, current.depth, SessionStates[current.State])
}

// loadSession loads the session from the cache.
func (s *PersistedStore) loadSession(sessionID string, msg *dipper.Message, child *Session) *Session {
	key := StoreSessionPrefix + sessionID

	dipper.Must(s.Call("locker", "lock", map[string]interface{}{
		"name":   key,
		"expire": "3600s",
	}, "timeout", "3600s"))
	resp := dipper.Must(s.Call("cache", "lrange", map[string]interface{}{
		"key": key,
	})).([]byte)

	stack := []*Session{}
	dipper.Must(json.Unmarshal(resp, &stack))
	if len(stack) == 0 {
		// stop further processing, let global error handler deal with it.
		s.Warningf("session %s not found in cache", sessionID)
		dipper.Must(s.Call("locker", "unlock", map[string]interface{}{
			"name": key,
		}))

		return nil
	}
	tail := stack[len(stack)-1]
	w := stack[0]
	if child != nil {
		s.Infof("session [%s.%s] loaded from cache return from [%s.%s]", w.ID, tail.CurrentMsg.Labels["cursor"],
			child.ID, child.CurrentMsg.Labels["cursor"])
	} else {
		s.Infof("session [%s.%s] loaded from cache", w.ID, tail.CurrentMsg.Labels["cursor"])
	}
	if tail.CurrentMsg.Labels["cursor"] != msg.Labels["cursor"] {
		s.Warningf("session %s ignoring mismatched cursor: expected %s, got %s",
			sessionID,
			tail.CurrentMsg.Labels["cursor"],
			msg.Labels["cursor"],
		)
		dipper.Must(s.Call("locker", "unlock", map[string]interface{}{
			"name": key,
		}))

		return nil
	}

	var current *Session
	for depth, w := range stack {
		w.depth = depth
		w.parent = current
		if current != nil {
			current.child = w
		}
		w.context, w.cancelFunc = context.WithCancel(context.Background())
		w.threads = &sync.WaitGroup{}
		w.store = s
		w.pending = true
		current = w
	}
	current.CurrentMsg = msg
	current.child = child

	return stack[0]
}

// getIDBatch reserves a batch of IDs from the locker.
func (s *PersistedStore) getIDBatch(async bool) {
	if async {
		s.idLock.Lock()
		defer s.idLock.Unlock()
	}

	s.maxID = dipper.Must(strconv.Atoi(string(dipper.Must(s.Call("locker", "getID", map[string]any{
		"name":  StoreSessionIDCounter,
		"wrap":  StoreSessionIDWrap,
		"batch": StoreSessionIDBatch,
	})).([]byte)))).(int)
	s.nextID = s.maxID - StoreSessionIDBatch + 1
}

// GetNextID returns the next available session ID from the reserved block.
func (s *PersistedStore) GetNextID() string {
	s.idLock.Lock()
	defer s.idLock.Unlock()

	if s.nextID == 0 {
		s.getIDBatch(false)
	}

	ret := strconv.Itoa(s.nextID)
	s.nextID++
	if s.nextID > s.maxID {
		s.nextID = 0
		s.maxID = 0
		daemon.Go(func() {
			dipper.SafeExitOnError("failed to pre-book batch of session IDs", func(_ any) {
			})
			s.getIDBatch(true)
		})
	}

	return ret
}

// GetLogger returns the logger  used by the session store.
func (s *PersistedStore) GetLogger() *logging.Logger {
	return s.Logger
}

// GetNumSessions returns the number of live sessions in the system.
func (s *PersistedStore) GetNumSessions(getAll bool) int {
	key := StoreSessionCounter
	if !getAll {
		key = StoreSessionCounter + ":" + s.storeID
	}
	v := dipper.Must(s.Call("cache", "load", map[string]any{"key": key}))
	if v == nil {
		return 0
	}

	if len(v.([]byte)) == 0 {
		return 0
	}

	return dipper.Must(strconv.Atoi(string(v.([]byte)))).(int)
}

// DumpSessions dumps the information of sessions for debugging.
func (s *PersistedStore) DumpSessions(cursor string) map[string]any {
	res := dipper.Must(s.Call("cache", "scan", map[string]any{"pattern": StoreSessionPrefix + "*", "cursor": cursor})).([]byte)
	data := dipper.DeserializeContent(res).(map[string]any)
	keys := data["keys"].([]any)

	ret := []map[string]any{}

	for _, key := range keys {
		buf := dipper.Must(s.Call("cache", "lrange", map[string]any{
			"key":  key,
			"stop": 0,
			"raw":  true,
		})).([]byte)
		w := &Session{}
		w.Unmarshal(buf)
		if w.Parent == "" {
			ret = append(ret, w.Dump())
		}
	}

	return map[string]any{
		"cursor":   data["cursor"],
		"sessions": ret,
	}
}

// Wait blocks until all sessions are done.
func (s *PersistedStore) Wait() {
	s.Live.Wait()
}
