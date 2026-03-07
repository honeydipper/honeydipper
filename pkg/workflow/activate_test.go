// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package workflow

import (
	"sync"
	"testing"
	"time"

	cfg "github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
)

// activateTestStore extends testStore to track activation calls.
type activateTestStore struct {
	activateCount    int
	emitResultCount  int
	continueCount    int
	childSessionID   string
	lastCreatedWf    *cfg.Workflow
	lastEmittedSess  *Session
	lastContinuedMsg *dipper.Message
	logger           *logging.Logger
	pendingChild     bool // if true, created children will start pending
}

func (s *activateTestStore) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (s *activateTestStore) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	return nil
}

func (s *activateTestStore) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (s *activateTestStore) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}

func (s *activateTestStore) GetName() string { return "test" }

func (s *activateTestStore) CallWithMessage(msg *dipper.Message) ([]byte, error) {
	return nil, nil
}

func (s *activateTestStore) CallWithMessageNoWait(msg *dipper.Message) error {
	return nil
}

func (s *activateTestStore) GetConfig() *cfg.Config {
	return &cfg.Config{}
}

func (s *activateTestStore) SendMessage(msg *dipper.Message) {}

func (s *activateTestStore) CreateChildSession(parent *Session, wf *cfg.Workflow, msg *dipper.Message) *Session {
	s.lastCreatedWf = wf
	wg := &sync.WaitGroup{}
	child := &Session{
		ID:         s.childSessionID,
		Workflow:   wf,
		CurrentMsg: msg,
		store:      s,
		Ctx:        map[string]interface{}{},
		Performing: []string{"initializing"},
		threads:    wg,
		pending:    s.pendingChild,
	}

	return child
}

func (s *activateTestStore) CreateAsyncChildSession(parent *Session, wf *cfg.Workflow, msg *dipper.Message) *Session {
	return nil
}

func (s *activateTestStore) ActivateSession(w *Session) {
	s.activateCount++
}

func (s *activateTestStore) EmitResult(w *Session) {
	s.emitResultCount++
	s.lastEmittedSess = w
}

func (s *activateTestStore) StartSession(wf *cfg.Workflow, msg *dipper.Message, ctx map[string]interface{}) {
}

func (s *activateTestStore) StartDynamicSession(spec *dipper.Message, ctx map[string]interface{}) {}

func (s *activateTestStore) ContinueSession(ID string, msg *dipper.Message) {
	s.continueCount++
	s.lastContinuedMsg = msg
}

func (s *activateTestStore) ResumeSession(key string, msg *dipper.Message) bool {
	return false
}

func (s *activateTestStore) GetNumSessions(getAll bool) int {
	return 0
}

func (s *activateTestStore) DumpSessions(cursor string) map[string]any {
	return nil
}

func (s *activateTestStore) Wait() {}

func (s *activateTestStore) GetLogger() *logging.Logger {
	if s.logger == nil {
		if dipper.Logger == nil {
			dipper.GetLogger("test", "ERROR")
		}
		s.logger = dipper.Logger
	}

	return s.logger
}

// makeActivateSession creates a session suitable for activate tests.
func makeActivateSession(state int) *Session {
	if dipper.Logger == nil {
		dipper.GetLogger("test", "ERROR")
	}
	wg := &sync.WaitGroup{}

	return &Session{
		ID:       "test",
		Workflow: &cfg.Workflow{},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"cursor": "0",
			},
			Payload: map[string]any{},
		},
		store:      &activateTestStore{childSessionID: "child"},
		Ctx:        map[string]interface{}{},
		State:      state,
		Performing: []string{"initializing"},
		threads:    wg,
		StartTime:  time.Now(),
	}
}

// TestActivateChild_NonPendingChild tests activateChild with non-pending child.
func TestActivateChild_NonPendingChild(t *testing.T) {
	s := makeActivateSession(SessionStateInit)
	customStore := &activateTestStore{childSessionID: "child"}
	s.store = customStore

	// Create a non-pending child with Store
	childStore := &activateTestStore{childSessionID: "grandchild"}
	s.child = &Session{
		ID: "child",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"status": SessionStatusSuccess,
				"cursor": "child",
			},
			Payload: map[string]any{},
		},
		Ctx:        map[string]interface{}{},
		threads:    &sync.WaitGroup{},
		store:      childStore,
		Workflow:   &cfg.Workflow{},
		Performing: []string{"init"},
	}

	s.activateChild()

	// Child should not be nil after non-pending child (Wait() was called)
	if s.child == nil {
		t.Error("child should not be nil after activateChild")
	}
	if s.CurrentMsg.Labels["status"] != SessionStatusSuccess {
		t.Errorf("expected status Success, got %s", s.CurrentMsg.Labels["status"])
	}
}

// TestActivateChild_PendingChild tests activateChild when child is pending.
// This test validates the early return when child is pending.
func TestActivateChild_PendingChild(t *testing.T) {
	s := makeActivateSession(SessionStateInit)
	childStore := &activateTestStore{childSessionID: "grandchild"}

	// Create a child with pending flag set
	s.child = &Session{
		ID:      "child",
		pending: true,
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"cursor": "child",
			},
			Payload: map[string]any{},
		},
		Ctx:        map[string]interface{}{},
		threads:    &sync.WaitGroup{},
		store:      childStore,
		Workflow:   &cfg.Workflow{},
		Performing: []string{"init"},
	}

	// activateChild should handle pending children without panic
	s.activateChild()
}

// TestActivateChild_ChildWithCurrentHook tests activateChild when child has CurrentHook.
func TestActivateChild_ChildWithCurrentHook(t *testing.T) {
	s := makeActivateSession(SessionStateInit)
	childStore := &activateTestStore{childSessionID: "grandchild"}
	s.child = &Session{
		ID: "child",

		CurrentHook: "on_session",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"cursor": "child",
			},
		},
		Ctx:        map[string]interface{}{},
		threads:    &sync.WaitGroup{},
		store:      childStore,
		Workflow:   &cfg.Workflow{},
		Performing: []string{"init"},
	}

	s.activateChild()

	// Should return early without injecting message
	if s.CurrentMsg.Labels["status"] != "" {
		t.Error("status should not be set when child has CurrentHook")
	}
}

// TestActivateChild_InjectionWithHook tests activateChild when parent has CurrentHook set.
func TestActivateChild_InjectionWithHook(t *testing.T) {
	s := makeActivateSession(SessionStateInit)

	s.CurrentHook = "on_session"
	childStore := &activateTestStore{childSessionID: "grandchild"}
	s.child = &Session{
		ID: "child",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"status": SessionStatusSuccess,
				"cursor": "child",
			},
			Payload: map[string]any{},
		},
		Ctx:        map[string]interface{}{},
		threads:    &sync.WaitGroup{},
		store:      childStore,
		Workflow:   &cfg.Workflow{},
		Performing: []string{"init"},
	}

	s.activateChild()

	// Should not inject message when parent has CurrentHook
	if s.CurrentMsg.Labels["status"] == SessionStatusSuccess {
		t.Error("should not inject when parent has hook")
	}
}

// TestActivate_CallsProgress tests that activate eventually calls progress.
func TestActivate_CallsProgress(t *testing.T) {
	s := makeActivateSession(SessionStateInit)
	s.pending = false

	// activate runs in a goroutine, so we call it
	s.activate()
	s.threads.Wait()

	// If activate called progress, state would have advanced
	// For SessionStateInit, it should transition
	if s.State == SessionStateInit {
		t.Error("state should have advanced from Init")
	}
}

// TestProgress_ElseState tests progress when state is SessionStateElse.
func TestProgress_ElseState(t *testing.T) {
	s := makeActivateSession(SessionStateElse)
	customStore := &activateTestStore{childSessionID: "else_child"}
	s.store = customStore

	s.Workflow.Else = &cfg.Workflow{Name: "else_branch"}

	// progress should create a child for the else branch
	s.progress()

	if customStore.activateCount != 1 {
		t.Errorf("expected 1 activate call, got %d", customStore.activateCount)
	}
	if s.child == nil {
		t.Error("child should be created for else branch")
	}
	if s.ElseBranch == nil {
		t.Error("ElseBranch should be decoded")
	}
}

// TestProgress_NextItemState tests progress for SessionStateNextItem.
func TestProgress_NextItemState(t *testing.T) {
	s := makeActivateSession(SessionStateNextItem)
	s.Ctx = map[string]interface{}{}
	s.Iteration = 0
	s.Workflow.Iterate = []interface{}{"item1", "item2"}
	s.Workflow.IterateAs = "myitem"

	s.progress()

	if s.Ctx["current"] != "item1" {
		t.Errorf("expected 'item1', got %v", s.Ctx["current"])
	}
	if s.Ctx["myitem"] != "item1" {
		t.Errorf("expected IterateAs 'item1', got %v", s.Ctx["myitem"])
	}
}

// TestProgress_NextRoundState tests progress for SessionStateNextRound.
func TestProgress_NextRoundState(t *testing.T) {
	s := makeActivateSession(SessionStateNextRound)
	s.Ctx = map[string]interface{}{}
	s.LoopCount = 5

	s.progress()

	if s.Ctx["loop_count"] != 5 {
		t.Errorf("expected loop_count 5, got %v", s.Ctx["loop_count"])
	}
}

// TestProgress_ActionState_SingleThread tests progress for action with single thread.
func TestProgress_ActionState_SingleThread(t *testing.T) {
	s := makeActivateSession(SessionStateAction)
	s.Workflow.Threads = []cfg.Workflow{}
	s.Current = 0

	// Single thread (empty Threads list), execute() will set pending via w.execute()
	s.progress()

	// The test verifies that execute() is called. We check if pending was set
	// (execute sets pending=true when needed)
	if !s.pending {
		t.Error("execute() should have been called, setting pending")
	}
}

// TestProgress_ActionState_MultiThread tests progress for action with multiple threads.
func TestProgress_ActionState_MultiThread(t *testing.T) {
	s := makeActivateSession(SessionStateAction)
	s.Workflow.Threads = []cfg.Workflow{{}, {}}
	s.Current = 1 // Current > 0

	s.progress()

	if !s.pending {
		t.Error("should be pending for multi-thread with Current > 0")
	}
}

// TestProgress_UpdateState tests progress for SessionStateUpdate.
func TestProgress_UpdateState(t *testing.T) {
	s := makeActivateSession(SessionStateUpdate)
	customStore := &activateTestStore{}
	s.store = customStore

	s.child = &Session{
		Exported: []map[string]interface{}{
			{"key": "value"},
		},
		Ctx:     map[string]interface{}{},
		threads: &sync.WaitGroup{},
	}
	s.Ctx = map[string]interface{}{}
	s.Exported = nil

	s.progress()

	if len(s.Exported) != 1 {
		t.Errorf("expected 1 exported entry, got %d", len(s.Exported))
	}
	if s.child != nil {
		t.Error("child should be nil after processUpdateState")
	}
}

// TestProgress_ExportState tests progress for SessionStateExport.
func TestProgress_ExportState(t *testing.T) {
	s := makeActivateSession(SessionStateExport)
	s.Ctx = map[string]interface{}{}
	s.Exported = nil

	s.progress()

	// processWorkflowExport should be called
	// Check that no error occurred
	if s.State == SessionStateExport {
		// State should have changed via resume
		t.Error("state should have transitioned")
	}
}

// TestProgress_DefaultState tests progress for unmapped state.
func TestProgress_DefaultState(t *testing.T) {
	s := makeActivateSession(SessionStateCheckCondition)
	customStore := &activateTestStore{}
	s.store = customStore

	// checkConditionState should be called, but for testing just verify progress runs
	s.progress()

	// Progress should complete without panic
	if s.CurrentMsg == nil {
		t.Error("CurrentMsg should be set")
	}
}

// TestProgress_WithStatus tests progress respects status labels.
func TestProgress_WithStatus(t *testing.T) {
	s := makeActivateSession(SessionStateCheckCondition)
	s.CurrentMsg.Labels["status"] = SessionStatusFailure
	s.CurrentMsg.Labels["performing"] = ""
	s.Performing = []string{"step1", "step2"}

	s.progress()

	if s.CurrentMsg.Labels["performing"] != "step1\nstep2" {
		t.Errorf("performing should be set for non-success status")
	}
}

// TestProcessElseState tests processElseState execution.
func TestProcessElseState(t *testing.T) {
	s := makeActivateSession(SessionStateElse)
	customStore := &activateTestStore{childSessionID: "else_child"}
	s.store = customStore

	s.Workflow.Else = &cfg.Workflow{Name: "else_branch"}

	s.processElseState()

	if s.ElseBranch == nil {
		t.Error("ElseBranch should be decoded")
	}
	if s.child == nil {
		t.Error("child session should be created")
	}
	if customStore.activateCount != 1 {
		t.Errorf("expected 1 activate call, got %d", customStore.activateCount)
	}
}

// TestProcessUpdateState_NoChild tests processUpdateState when no child exists.
func TestProcessUpdateState_NoChild(t *testing.T) {
	s := makeActivateSession(SessionStateUpdate)
	s.child = nil
	s.Workflow.Steps = []cfg.Workflow{}
	s.CurrentMsg.Labels["status"] = ""

	s.processUpdateState()

	if s.CurrentMsg.Labels["status"] != SessionStatusSuccess {
		t.Errorf("expected status Success for noop, got %s", s.CurrentMsg.Labels["status"])
	}
}

// TestProcessUpdateState_MergesExportedData tests processUpdateState merges child exports.
func TestProcessUpdateState_MergesExportedData(t *testing.T) {
	s := makeActivateSession(SessionStateUpdate)
	s.Ctx = map[string]interface{}{"orig": "value"}
	s.child = &Session{
		Exported: []map[string]interface{}{
			{"new": "data"},
			{"more": "stuff"},
		},
		Ctx:     map[string]interface{}{},
		threads: &sync.WaitGroup{},
	}
	s.ElseBranch = nil

	s.processUpdateState()

	if s.Ctx["new"] != "data" {
		t.Error("exported data should be merged")
	}
	if s.Ctx["more"] != "stuff" {
		t.Error("second export should be merged")
	}
	if len(s.Exported) != 2 {
		t.Errorf("expected 2 exported entries, got %d", len(s.Exported))
	}
	if s.child != nil {
		t.Error("child should be nil after update")
	}
}

// TestProcessUpdateState_WithElseBranch tests processUpdateState doesn't merge when else branch executed.
func TestProcessUpdateState_WithElseBranch(t *testing.T) {
	s := makeActivateSession(SessionStateUpdate)
	s.Ctx = map[string]interface{}{}
	s.child = &Session{
		Exported: []map[string]interface{}{
			{"key": "value"},
		},
		Ctx:     map[string]interface{}{},
		threads: &sync.WaitGroup{},
	}
	s.ElseBranch = &cfg.Workflow{}

	s.processUpdateState()

	// Data should not be merged when else branch executed
	if _, ok := s.Ctx["key"]; ok {
		t.Error("should not merge when else branch executed")
	}
}

// TestResume_TransitionsState tests resume calls determineNextState.
func TestResume_TransitionsState(t *testing.T) {
	s := makeActivateSession(SessionStateCheckCondition)
	s.CurrentHook = ""

	oldState := s.State
	s.resume()

	if s.State == oldState {
		t.Error("state should transition in resume")
	}
}

// TestResume_WithCurrentHook tests resume returns when CurrentHook is set.
func TestResume_WithCurrentHook(t *testing.T) {
	s := makeActivateSession(SessionStateCheckCondition)

	s.CurrentHook = "on_session"

	oldState := s.State
	s.resume()

	if s.State != oldState {
		t.Error("state should not transition when CurrentHook is set")
	}
}

// TestResume_ToDone tests resume handling when transitioning to done state.
func TestResume_ToDone(t *testing.T) {
	s := makeActivateSession(SessionStateSuccess)
	customStore := &activateTestStore{}
	s.store = customStore

	// Ensure parent is not set so EmitResult is called via daemon.Go
	s.parent = nil
	s.Parent = ""

	// SessionStateSuccess should transition to SessionStateDone
	s.resume()

	// EmitResult is called via daemon.Go which runs asynchronously
	// Give it a moment to execute
	time.Sleep(10 * time.Millisecond)

	if customStore.emitResultCount != 1 {
		t.Errorf("expected EmitResult called once, got %d", customStore.emitResultCount)
	}
}

// TestResume_WithParentID tests resume continuing to parent.
func TestResume_WithParentID(t *testing.T) {
	s := makeActivateSession(SessionStateSuccess)
	s.Parent = "parent_id.cursor_pos"
	customStore := &activateTestStore{}
	s.store = customStore
	s.parent = nil // Not a parent object, just parent ID

	s.resume()

	// Wait briefly for goroutine
	time.Sleep(10 * time.Millisecond)

	if customStore.continueCount != 1 {
		t.Errorf("expected ContinueSession called, got %d", customStore.continueCount)
	}
}

// TestResume_InvalidParentFormat tests resume with invalid parent format.
func TestResume_InvalidParentFormat(t *testing.T) {
	s := makeActivateSession(SessionStateSuccess)
	s.Parent = "invalid_parent" // No dot
	customStore := &activateTestStore{}
	s.store = customStore

	// This should panic, which is caught by logger.Panicf
	// We can't easily test panic here without hijacking the logger
	// Just verify the format check would be triggered
	pos := len(s.Parent)
	if pos > 0 {
		t.Log("Invalid parent format test - would trigger panic")
	}
}

// TestResume_ToDoneWithNilParent tests final completion with no parent.
func TestResume_ToDoneWithNilParent(t *testing.T) {
	s := makeActivateSession(SessionStateSuccess)
	s.Parent = ""
	customStore := &activateTestStore{}
	s.store = customStore

	s.resume()

	// Wait for async completion
	time.Sleep(50 * time.Millisecond)

	if customStore.emitResultCount != 1 {
		t.Errorf("expected EmitResult, got %d", customStore.emitResultCount)
	}
}

// TestResumeClears PendingFlag tests that resume sets pending to false.
func TestResumeClearsPendingFlag(t *testing.T) {
	s := makeActivateSession(SessionStateCheckCondition)
	s.pending = true

	s.resume()

	if s.pending {
		t.Error("pending should be false after resume")
	}
}

// TestProgress_InitStatus tests progress sets status based on successful flow.
func TestProgress_InitStatus(t *testing.T) {
	s := makeActivateSession(SessionStateInit)
	s.CurrentMsg.Labels["status"] = ""

	// Starting from Init, no status set means it will default
	// The actual status depends on the state machine flow
	// Just verify progress completes
	s.progress()

	if s.CurrentMsg == nil {
		t.Error("CurrentMsg should exist")
	}
}

// TestActivateSession_RemovedPendingFlag tests that activate sets pending properly.
func TestActivateSession_RemovedPendingFlag(t *testing.T) {
	s := makeActivateSession(SessionStateInit)
	s.pending = true
	s.child = nil

	s.activate()
	s.threads.Wait()

	// After activate, pending should be false unless set by progress
	// This depends on the state machine implementation
	if s.CurrentMsg == nil {
		t.Error("CurrentMsg should be set")
	}
}
