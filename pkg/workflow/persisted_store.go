// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/daemon"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/jellydator/ttlcache/v3"
	"github.com/mitchellh/mapstructure"
	"github.com/op/go-logging"
)

const (
	// BroadcaseSubjectResult is the subject for broadcasting session result.
	BroadcaseSubjectResult = "result"
	// SessionStream is used to stream changes of the sessions for debugging.
	SessionStream = "session_stream_"
	// SessionStreamRetention is the retention period for the session stream.
	SessionStreamRetention = "48h"
)

// PersistredStore stores session using the cache driver to persist the sessions.
type PersistedStore struct {
	// StoreHelper provides the methods for workflow to access daemon.
	StoreHelper

	*logging.Logger
	nextID  int
	maxID   int
	idLock  sync.Locker
	storeID string

	lifecycleOnce sync.Once
	lifecycleMu   sync.Mutex
	lifecycleCond *sync.Cond
	activeTasks   int

	cache *ttlcache.Cache[string, map[string]any]
}

type completionCallback interface {
	OnSessionCompleted(w *Session)
}

func (s *PersistedStore) initLifecycle() {
	s.lifecycleOnce.Do(func() {
		s.lifecycleCond = sync.NewCond(&s.lifecycleMu)
	})
}

func (s *PersistedStore) taskStart() {
	s.initLifecycle()
	s.lifecycleMu.Lock()
	s.activeTasks++
	s.lifecycleMu.Unlock()
}

func (s *PersistedStore) taskDone() {
	s.initLifecycle()
	s.lifecycleMu.Lock()
	s.activeTasks--
	if s.activeTasks == 0 {
		s.lifecycleCond.Broadcast()
	}
	s.lifecycleMu.Unlock()
}

func (s *PersistedStore) waitForTasks() {
	s.initLifecycle()
	s.lifecycleMu.Lock()
	for s.activeTasks > 0 {
		s.lifecycleCond.Wait()
	}
	s.lifecycleMu.Unlock()
}

// RunAsync starts a store-owned background task and tracks it for draining.
func (s *PersistedStore) RunAsync(task func()) {
	s.taskStart()
	daemon.Go(func() {
		defer s.taskDone()
		task()
	})
}

// CreateSession creates and initializes a workflow session.
func (s *PersistedStore) CreateSession(wf *config.Workflow, msg *dipper.Message, ctx map[string]interface{}) *Session {
	return s.CreateSessionWithInitContext(wf, msg, ctx, nil, nil)
}

// CreateSessionWithInitContext creates and initializes a workflow session with separated init contexts.
func (s *PersistedStore) CreateSessionWithInitContext(
	wf *config.Workflow,
	msg *dipper.Message,
	eventCtx map[string]interface{},
	rerunCtx map[string]interface{},
	loadedContexts []string,
) *Session {
	w := NewSession(s.GetNextID(), wf, s)
	w.Init(msg, nil, eventCtx, rerunCtx, loadedContexts)
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
	frame := (*w.Performing)[w.depth]
	if w.parent != nil {
		w.parent.trimPerformingToCurrentDepth()
	}

	w.Performing = &[]string{frame}
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

	var p *Session
	for p = w.parent; p != nil && len(p.EventCtx) == 0; p = p.parent {
	}
	if p != nil {
		w.EventCtx = dipper.MustDeepCopyMap(p.EventCtx)
	}
	if len(w.Ctx) > 0 {
		w.RerunCtx = dipper.MustDeepCopyMap(w.Ctx)
	}

	w.parent = nil

	s.persist(w)
}

// CreateChildSession creates a child session with the given parent.
func (s *PersistedStore) CreateChildSession(parent *Session, wf *config.Workflow, msg *dipper.Message) *Session {
	w := NewSession(parent.ID, wf, s)
	w.Init(msg, parent, nil, nil, nil)
	if w.Workflow.Detach {
		s.DetachSession(w)
	}

	return w
}

// CreateAsyncChildSession creates a child async workflow session.
func (s *PersistedStore) CreateAsyncChildSession(parent *Session, wf *config.Workflow, msg *dipper.Message) *Session {
	m := dipper.Must(dipper.MessageCopy(msg)).(*dipper.Message)
	w := s.CreateChildSession(parent, wf, m)
	w.Parent = parent.ID + "." + parent.CurrentMsg.Labels["cursor"]
	s.DetachSession(w)
	if parent.AsyncChildren == nil {
		parent.AsyncChildren = map[string]bool{}
	}
	parent.AsyncChildren[w.ID] = true

	return w
}

// ActivateSession activates a session to progress from its current state.
func (s *PersistedStore) ActivateSession(w *Session) {
	w.activate()
	if w.parent == nil {
		s.RunAsync(func() {
			dipper.SafeExitOnError("[%s] panic when waiting for the session.", w.ID)
			defer s.persist(w)
			w.Wait()

			if !w.Paused && len(w.PendingMessages) > 0 && w.State != SessionStateDone {
				msg := w.PendingMessages[0]
				w.PendingMessages = w.PendingMessages[1:]
				s.RunAsync(func() {
					s.ContinueSession(w.ID, msg, nil)
				})
			}
		})
	}
}

// EmitResult emits the result of the session to a storage watched by consumers.
func (s *PersistedStore) EmitResult(w *Session) {
	mesg := w.Dump()
	mesg["topic"] = "workflow"
	mesg["subject"] = BroadcaseSubjectResult
	dipper.Must(s.CallNoWait("driver:redispubsub", "send", mesg))

	if cb, ok := s.StoreHelper.(completionCallback); ok {
		cb.OnSessionCompleted(w)
	}
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
		if w.CurrentMsg == nil {
			w.CurrentMsg = &dipper.Message{}
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
	s.StartSessionWithInitContext(wf, msg, ctx, nil)
}

// StartSessionWithInitContext starts a predefined workflow with separated event and rerun context variables.
func (s *PersistedStore) StartSessionWithInitContext(
	wf *config.Workflow,
	msg *dipper.Message,
	eventCtx map[string]interface{},
	rerunCtx map[string]interface{},
) {
	s.StartSessionWithInitContextHook(wf, msg, eventCtx, rerunCtx, nil, nil)
}

// StartSessionWithInitContextHook starts a predefined workflow and allows callers
// to run a callback after session creation/persistence and before activation.
func (s *PersistedStore) StartSessionWithInitContextHook(
	wf *config.Workflow,
	msg *dipper.Message,
	eventCtx map[string]interface{},
	rerunCtx map[string]interface{},
	loadedContexts []string,
	beforeActivate func(*Session),
) *Session {
	m := dipper.Must(dipper.MessageCopy(msg)).(*dipper.Message)
	defer dipper.SafeExitOnError("[workflow] error when creating workflow session", s.uncaughtErrorHandler(nil, m))

	w := s.CreateSessionWithInitContext(wf, m, eventCtx, rerunCtx, loadedContexts)
	if beforeActivate != nil {
		beforeActivate(w)
	}

	defer dipper.SafeExitOnError("[workflow] error when starting workflow session", s.uncaughtErrorHandler(w, m))
	s.ActivateSession(w)

	return w
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

	defer dipper.SafeExitOnError("[workflow] error when starting workflow session", s.uncaughtErrorHandler(w, msg))

	s.ActivateSession(w)
}

// ResumeSession resume a session that is in waiting state.
func (s *PersistedStore) ResumeSession(key string, msg *dipper.Message) bool {
	m := dipper.Must(dipper.MessageCopy(msg)).(*dipper.Message)
	data, _ := s.Call("scheduler", "cancel", map[string]any{"type": "session", "key": key})
	if data == nil {
		s.Warningf("unable to resume for key %s, no payload received from waiter.", key)

		return false
	}

	payload := dipper.DeserializeContent(data)
	s.Debugf("resuming session with key %s and payload %+v", key, payload)
	sessionID := dipper.MustGetMapDataStr(payload, "labels.sessionID")
	cursor := dipper.MustGetMapDataStr(payload, "labels.cursor")

	if m.Labels == nil {
		m.Labels = map[string]string{}
	}
	m.Labels["sessionID"] = sessionID
	m.Labels["cursor"] = cursor
	if _, ok := m.Labels["status"]; !ok {
		m.Labels["status"] = SessionStatusSuccess
	}

	s.RunAsync(func() { s.ContinueSession(sessionID, m, nil) })

	return true
}

func (s *PersistedStore) lockSessionKey(key string) {
	dipper.Must(s.Call("locker", "lock", map[string]interface{}{
		"name":   key,
		"expire": "3600s",
	}))
}

func (s *PersistedStore) unlockSessionKey(key string) {
	_, err := s.Call("locker", "unlock", map[string]interface{}{
		"name": key,
	})
	if err != nil {
		s.Debugf("failed to unlock session key %s (lock will auto-expire): %v", key, err)
	}
}

func (s *PersistedStore) loadStackForControl(sessionID string) ([]*Session, string, error) {
	key := StoreSessionPrefix + sessionID
	s.lockSessionKey(key)

	raw := dipper.Must(s.Call("cache", "lrange", map[string]interface{}{
		"key": key,
	}))
	resp, _ := raw.([]byte)

	stack := []*Session{}
	if len(resp) > 0 {
		dipper.Must(json.Unmarshal(resp, &stack))
	}
	if len(stack) == 0 {
		s.unlockSessionKey(key)

		return nil, "", ErrSessionNotFound
	}

	for i := 0; i < len(stack); i++ {
		stack[i].child = nil
		stack[i].parent = nil
		stack[i].store = s
		stack[i].depth = i
		if i > 0 {
			stack[i-1].child = stack[i]
			stack[i].parent = stack[i-1]
		}
	}

	return stack, key, nil
}

func (s *PersistedStore) controlResult(w *Session, action string) map[string]interface{} {
	return map[string]interface{}{
		"sessionID": w.ID,
		"action":    action,
		"state":     w.Dump()["data"].(map[string]any)["state"],
		"paused":    w.Paused,
		"cancelled": w.Cancelled,
	}
}

func snapshotAsyncChildren(stack []*Session) []string {
	if len(stack) == 0 {
		return nil
	}
	seen := map[string]bool{}
	children := []string{}
	for _, frame := range stack {
		for sessionID := range frame.AsyncChildren {
			if sessionID == "" || seen[sessionID] {
				continue
			}
			seen[sessionID] = true
			children = append(children, sessionID)
		}
	}
	if len(children) == 0 {
		return nil
	}

	return children
}

func isIgnorableChildControlErr(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrSessionTerminated)
}

func hasInteractiveOptionKey(optionKey string, options any) bool {
	switch opts := options.(type) {
	case map[string]any:
		_, ok := opts[optionKey]

		return ok
	case []any:
		for _, item := range opts {
			option, ok := item.(map[string]any)
			if !ok {
				continue
			}
			k, _ := dipper.GetMapDataStr(option, "key")
			if k == optionKey {
				return true
			}
		}
	}

	return false
}

func (s *PersistedStore) resolveInteractiveMessage(optionKey string, messages any) (*dipper.Message, error) {
	selected := any(nil)
	found := false

	switch opts := messages.(type) {
	case map[string]any:
		selected, found = opts[optionKey]
	case []any:
		for _, item := range opts {
			option, ok := item.(map[string]any)
			if !ok {
				continue
			}
			k, _ := dipper.GetMapDataStr(option, "key")
			if k == optionKey {
				selected = option
				found = true

				break
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("%w: %s", ErrInteractiveOptionNotFound, optionKey)
	}

	msg := &dipper.Message{}
	if selectedMap, ok := selected.(map[string]any); ok {
		if rawMsg, hasRaw := selectedMap["message"]; hasRaw {
			dipper.Must(mapstructure.Decode(rawMsg, msg))
		} else if _, hasPayload := selectedMap["payload"]; hasPayload {
			dipper.Must(mapstructure.Decode(selectedMap, msg))
		} else {
			msg.Payload = selectedMap
		}
	} else {
		msg.Payload = map[string]any{"value": selected}
	}

	if msg.Payload == nil {
		msg.Payload = map[string]any{}
	}
	if payload, ok := msg.Payload.(map[string]any); ok {
		if _, exists := payload["key"]; !exists {
			payload["key"] = optionKey
		}
	}

	return msg, nil
}

func resolveInteractiveOptionInfo(optionKey string, options any) map[string]any {
	findFromOption := func(option map[string]any) map[string]any {
		ret := map[string]any{}
		label, _ := dipper.GetMapDataStr(option, "label")
		if strings.TrimSpace(label) == "" {
			label, _ = dipper.GetMapDataStr(option, "title")
		}
		if strings.TrimSpace(label) != "" {
			ret["label"] = label
		}
		if style, ok := dipper.GetMapDataStr(option, "style"); ok && strings.TrimSpace(style) != "" {
			ret["style"] = style
		}

		return ret
	}

	switch opts := options.(type) {
	case map[string]any:
		if option, ok := opts[optionKey].(map[string]any); ok {
			return findFromOption(option)
		}
	case []any:
		for _, item := range opts {
			option, ok := item.(map[string]any)
			if !ok {
				continue
			}
			k, _ := dipper.GetMapDataStr(option, "key")
			if k != optionKey {
				continue
			}

			return findFromOption(option)
		}
	}

	return map[string]any{}
}

func buildInteractiveSelectionEntry(key string, actor string, optionInfo map[string]any) map[string]any {
	entry := map[string]any{
		"key": key,
		"at":  time.Now().UTC().Format(time.RFC3339Nano),
	}
	if actor = strings.TrimSpace(actor); actor != "" {
		entry["user"] = actor
	}
	if label, ok := dipper.GetMapDataStr(optionInfo, "label"); ok && strings.TrimSpace(label) != "" {
		entry["label"] = label
	}
	if style, ok := dipper.GetMapDataStr(optionInfo, "style"); ok && strings.TrimSpace(style) != "" {
		entry["style"] = style
	}

	return entry
}

func recordInteractiveSelection(target map[string]any, entry map[string]any) {
	if target == nil || entry == nil {
		return
	}

	raw := target["interactive_interactions"]
	if raw == nil {
		target["interactive_interactions"] = []any{dipper.MustDeepCopyMap(entry)}
		delete(target, "interactive_interaction")

		return
	}

	history, ok := raw.([]any)
	if !ok {
		target["interactive_interactions"] = []any{dipper.MustDeepCopyMap(entry)}
		delete(target, "interactive_interaction")

		return
	}

	target["interactive_interactions"] = append(history, dipper.MustDeepCopyMap(entry))
	delete(target, "interactive_interaction")
}

type controlStackResult struct {
	root      *Session
	children  []string
	resumeMsg *dipper.Message
}

func (s *PersistedStore) pauseSingleStack(sessionID string) (*controlStackResult, error) {
	stack, key, err := s.loadStackForControl(sessionID)
	if err != nil {
		return nil, err
	}

	root := stack[0]
	if root.State == SessionStateDone {
		s.unlockSessionKey(key)

		return nil, ErrSessionTerminated
	}
	if root.State == SessionStateInit {
		s.unlockSessionKey(key)

		return nil, fmt.Errorf("%w: session not yet controllable", ErrWorkflowError)
	}

	for _, frame := range stack {
		frame.Paused = true
		frame.Cancelled = false
	}
	children := snapshotAsyncChildren(stack)
	s.persist(root)

	return &controlStackResult{root: root, children: children}, nil
}

func (s *PersistedStore) resumeSingleStack(sessionID string) (*controlStackResult, error) {
	stack, key, err := s.loadStackForControl(sessionID)
	if err != nil {
		return nil, err
	}

	root := stack[0]
	if root.State == SessionStateDone {
		s.unlockSessionKey(key)

		return nil, ErrSessionTerminated
	}
	if root.State == SessionStateInit {
		s.unlockSessionKey(key)

		return nil, fmt.Errorf("%w: session not yet controllable", ErrWorkflowError)
	}

	for _, frame := range stack {
		frame.Paused = false
	}
	children := snapshotAsyncChildren(stack)
	var resumeMsg *dipper.Message
	if len(root.PendingMessages) > 0 {
		resumeMsg = root.PendingMessages[0]
		root.PendingMessages = root.PendingMessages[1:]
	}

	s.persist(root)

	return &controlStackResult{root: root, children: children, resumeMsg: resumeMsg}, nil
}

func (s *PersistedStore) cancelSingleStack(sessionID string, reason string) (*controlStackResult, error) {
	stack, key, err := s.loadStackForControl(sessionID)
	if err != nil {
		return nil, err
	}

	root := stack[0]
	tail := stack[len(stack)-1]
	if root.State == SessionStateDone {
		s.unlockSessionKey(key)

		return nil, ErrSessionTerminated
	}
	if root.State == SessionStateInit {
		s.unlockSessionKey(key)

		return nil, fmt.Errorf("%w: session not yet controllable", ErrWorkflowError)
	}

	for _, frame := range stack {
		frame.Cancelled = true
		frame.Paused = false
	}
	if reason != "" {
		root.CancelReason = reason
	}
	children := snapshotAsyncChildren(stack)
	s.persist(root)

	resumeMsg := dipper.Must(dipper.MessageCopy(tail.CurrentMsg)).(*dipper.Message)

	return &controlStackResult{root: root, children: children, resumeMsg: resumeMsg}, nil
}

// PauseSession pauses a session by ID at the next safe checkpoint.
func (s *PersistedStore) PauseSession(sessionID string) (map[string]interface{}, error) {
	queue := []string{sessionID}
	seen := map[string]bool{}
	var rootResult *controlStackResult

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true

		result, err := s.pauseSingleStack(currentID)
		if err != nil {
			if currentID != sessionID && isIgnorableChildControlErr(err) {
				continue
			}

			return nil, err
		}
		if currentID == sessionID {
			rootResult = result
		}
		for _, childID := range result.children {
			if !seen[childID] {
				queue = append(queue, childID)
			}
		}
	}

	if rootResult == nil {
		return nil, ErrSessionNotFound
	}

	return s.controlResult(rootResult.root, "pause"), nil
}

// ResumeSessionByID resumes a paused session by ID.
func (s *PersistedStore) ResumeSessionByID(sessionID string) (map[string]interface{}, error) {
	queue := []string{sessionID}
	seen := map[string]bool{}
	var rootResult *controlStackResult

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true

		result, err := s.resumeSingleStack(currentID)
		if err != nil {
			if currentID != sessionID && isIgnorableChildControlErr(err) {
				continue
			}

			return nil, err
		}
		if currentID == sessionID {
			rootResult = result
		}

		msg := result.resumeMsg
		rootID := result.root.ID
		if msg != nil {
			s.RunAsync(func() {
				s.ContinueSession(rootID, msg, nil)
			})
		}

		for _, childID := range result.children {
			if !seen[childID] {
				queue = append(queue, childID)
			}
		}
	}

	if rootResult == nil {
		return nil, ErrSessionNotFound
	}

	return s.controlResult(rootResult.root, "resume"), nil
}

// InteractSessionByID resumes a waiting session using a keyed interactive option.
func (s *PersistedStore) InteractSessionByID(sessionID string, key string, actor string) (map[string]interface{}, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrInteractiveOptionNotFound
	}

	stack, lockKey, err := s.loadStackForControl(sessionID)
	if err != nil {
		return nil, err
	}

	root := stack[0]
	tail := stack[len(stack)-1]
	if root.State == SessionStateDone {
		s.unlockSessionKey(lockKey)

		return nil, ErrSessionTerminated
	}
	if root.State == SessionStateInit {
		s.unlockSessionKey(lockKey)

		return nil, fmt.Errorf("%w: session not yet controllable", ErrWorkflowError)
	}
	if tail.Workflow == nil || tail.Workflow.Wait == "" {
		s.unlockSessionKey(lockKey)

		return nil, ErrSessionNotInteractive
	}
	if tail.Ctx == nil {
		s.unlockSessionKey(lockKey)

		return nil, ErrSessionNotInteractive
	}

	options, ok := tail.Ctx["interactive_options"]
	if !ok || options == nil {
		s.unlockSessionKey(lockKey)

		return nil, ErrSessionNotInteractive
	}
	if !hasInteractiveOptionKey(key, options) {
		s.unlockSessionKey(lockKey)

		return nil, fmt.Errorf("%w: %s", ErrInteractiveOptionNotFound, key)
	}

	messages, ok := tail.Ctx["interactive_messages"]
	if !ok || messages == nil {
		s.unlockSessionKey(lockKey)

		return nil, fmt.Errorf("%w: %s", ErrInteractiveOptionNotFound, key)
	}

	msg, err := s.resolveInteractiveMessage(key, messages)
	if err != nil {
		s.unlockSessionKey(lockKey)

		return nil, err
	}

	optionInfo := resolveInteractiveOptionInfo(key, options)
	entry := buildInteractiveSelectionEntry(key, actor, optionInfo)
	if tail.Ctx == nil {
		tail.Ctx = map[string]any{}
	}
	recordInteractiveSelection(tail.Ctx, entry)
	if root != tail {
		if root.Ctx == nil {
			root.Ctx = map[string]any{}
		}
		recordInteractiveSelection(root.Ctx, entry)
	}
	s.persist(root)

	resumeKey := tail.ID + "." + tail.CurrentMsg.Labels["cursor"]
	if customResumeKey, ok := tail.Ctx["resume_key"]; ok && customResumeKey != nil {
		if v, ok := customResumeKey.(string); ok && strings.TrimSpace(v) != "" {
			resumeKey = v
		}
	}

	s.unlockSessionKey(lockKey)
	if !s.ResumeSession(resumeKey, msg) {
		return nil, ErrSessionNotFound
	}

	return s.controlResult(root, "interact"), nil
}

// CancelSessionByID marks a session as cancelled and schedules it to converge to terminal state.
func (s *PersistedStore) CancelSessionByID(sessionID string, reason string) (map[string]interface{}, error) {
	queue := []string{sessionID}
	seen := map[string]bool{}
	var rootResult *controlStackResult

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true

		result, err := s.cancelSingleStack(currentID, reason)
		if err != nil {
			if currentID != sessionID && isIgnorableChildControlErr(err) {
				continue
			}

			return nil, err
		}
		if currentID == sessionID {
			rootResult = result
		}

		msg := result.resumeMsg
		rootID := result.root.ID
		s.RunAsync(func() {
			s.ContinueSession(rootID, msg, nil)
		})

		for _, childID := range result.children {
			if !seen[childID] {
				queue = append(queue, childID)
			}
		}
	}

	if rootResult == nil {
		return nil, ErrSessionNotFound
	}

	return s.controlResult(rootResult.root, "cancel"), nil
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
		var savedPerforming *[]string
		var savedEvent map[string]interface{}
		var savedOriginalWorkflow *config.Workflow
		if current != root {
			savedPerforming = current.Performing
			savedEvent = current.Event
			savedOriginalWorkflow = current.OriginalWorkflow
			current.Performing = nil
			current.Event = nil
			current.OriginalWorkflow = nil
		}
		dipper.Must(s.Call("cache", "rpush", map[string]interface{}{
			"key":   key,
			"value": string(current.Marshal()),
			"ttl":   ttl,
		}))
		if current != root {
			current.Performing = savedPerforming
			current.Event = savedEvent
			current.OriginalWorkflow = savedOriginalWorkflow
		}
		cursor = current.CurrentMsg.Labels["cursor"]
		if current.child == nil {
			break
		}
		current = current.child
	}

	dipper.Must(s.Call("cache", "stream_hset", map[string]interface{}{
		"prefix": SessionStream,
		"key":    w.ID,
		"value":  string(dipper.SerializeContent(w.Dump())),
	}))

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
		s.unlockSessionKey(key)
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
	}))

	raw := dipper.Must(s.Call("cache", "lrange", map[string]interface{}{
		"key": key,
	}))
	resp, _ := raw.([]byte)

	stack := []*Session{}
	if len(resp) > 0 {
		dipper.Must(json.Unmarshal(resp, &stack))
	}
	if len(stack) == 0 {
		// stop further processing, let global error handler deal with it.
		s.Warningf("session %s not found in cache", sessionID)
		s.unlockSessionKey(key)

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
		s.unlockSessionKey(key)

		return nil
	}

	if stack[0].State == SessionStateDone {
		s.Warningf("session is already done: %s", sessionID)
		s.unlockSessionKey(key)

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

	// Share the Performing stack from the root with all sessions.
	for _, session := range stack {
		session.Performing = stack[0].Performing
		session.Event = stack[0].Event
	}

	if child != nil && current.AsyncChildren != nil {
		delete(current.AsyncChildren, child.ID)
		if len(current.AsyncChildren) == 0 {
			current.AsyncChildren = nil
		}
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
		s.RunAsync(func() {
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
func (s *PersistedStore) DumpSessions(lookBack int, asOf string) []byte {
	return dipper.Must(s.Call("cache", "stream_hvals", map[string]any{
		"prefix":    SessionStream,
		"look_back": lookBack,
		"asOf":      asOf,
	})).([]byte)
}

// Wait blocks until all sessions are done.
func (s *PersistedStore) Wait() {
	s.waitForTasks()
}

// Stop blocks until all sessions are done and shut down the cache.
func (s *PersistedStore) Stop() {
	s.Wait()
	if s.cache != nil {
		s.cache.Stop()
	}
}

// GetCache returns the cache instance used by the session store.
func (s *PersistedStore) GetCache() *ttlcache.Cache[string, map[string]any] {
	return s.cache
}
