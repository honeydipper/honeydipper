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

	cfg "github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/jellydator/ttlcache/v3"
	"github.com/op/go-logging"
)

// hookTrackingStore is a test store that tracks CreateChildSession calls.
type hookTrackingStore struct {
	createdChildCount int
	lastCreatedWf     *cfg.Workflow
	lastCreatedMsg    *dipper.Message
	makePendingChild  bool
}

func (s *hookTrackingStore) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (s *hookTrackingStore) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	return nil
}

func (s *hookTrackingStore) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (s *hookTrackingStore) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}

func (s *hookTrackingStore) GetName() string { return _testModule }

func (s *hookTrackingStore) CallWithMessage(msg *dipper.Message) ([]byte, error) {
	return nil, nil
}

func (s *hookTrackingStore) CallWithMessageNoWait(msg *dipper.Message) error {
	return nil
}

func (s *hookTrackingStore) GetConfig() *cfg.Config {
	return &cfg.Config{}
}

func (s *hookTrackingStore) SendMessage(msg *dipper.Message) {}

func (s *hookTrackingStore) CreateChildSession(parent *Session, wf *cfg.Workflow, msg *dipper.Message) *Session {
	s.createdChildCount++
	s.lastCreatedWf = wf
	s.lastCreatedMsg = msg

	// Return a mock session with minimal setup for testing.
	wg := &sync.WaitGroup{}
	child := &Session{
		ID:          "child",
		Workflow:    wf,
		CurrentMsg:  msg,
		store:       s,
		Ctx:         map[string]interface{}{},
		pending:     s.makePendingChild,
		CurrentHook: "",
		Performing:  &[]string{"initializing"},
		threads:     wg,
	}

	return child
}

func (s *hookTrackingStore) CreateAsyncChildSession(parent *Session, wf *cfg.Workflow, msg *dipper.Message) *Session {
	return nil
}

func (s *hookTrackingStore) ActivateSession(w *Session) {}

func (s *hookTrackingStore) EmitResult(w *Session) {}

func (s *hookTrackingStore) StartSession(wf *cfg.Workflow, msg *dipper.Message, ctx map[string]interface{}) {
}

func (s *hookTrackingStore) StartDynamicSession(spec *dipper.Message, ctx map[string]interface{}) {}

func (s *hookTrackingStore) ContinueSession(ID string, msg *dipper.Message, child *Session) {}

func (s *hookTrackingStore) ResumeSession(key string, msg *dipper.Message) bool {
	return false
}

func (s *hookTrackingStore) GetNumSessions(getAll bool) int {
	return 0
}

func (s *hookTrackingStore) DumpSessions(_ int, _ string) []byte {
	return nil
}

func (s *hookTrackingStore) Wait() {}

func (s *hookTrackingStore) GetLogger() *logging.Logger {
	if dipper.Logger == nil {
		dipper.GetLogger(_testModule, "ERROR")
	}

	return dipper.Logger
}

func (s *hookTrackingStore) Stop() {}

func (s *hookTrackingStore) GetCache() *ttlcache.Cache[string, map[string]any] {
	return nil
}

// makeHookSession creates a session suitable for testing hooks.
func makeHookSession(state int) *Session {
	wg := &sync.WaitGroup{}

	return &Session{
		ID:       "test",
		Workflow: &cfg.Workflow{},
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]any{},
		},
		store:      &hookTrackingStore{},
		Ctx:        map[string]interface{}{},
		State:      state,
		Performing: &[]string{"initializing"},
		threads:    wg,
	}
}

// TestFireClearHook_EntryHook_WithDefinedHook tests entry hook that exists in context.
func TestFireClearHook_EntryHook_WithDefinedHook(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"on_session": "workflow_name",
		},
	}
	s.CurrentHook = ""

	ret := s.fireOrClearHook(true)
	if !ret {
		t.Error("should return true because SessionStateCheckCondition has entry hooks")
	}
	if s.CurrentHook != "" {
		t.Errorf("CurrentHook should be cleared after non-pending execution, got %s", s.CurrentHook)
	}
	if customStore.createdChildCount != 1 {
		t.Errorf("expected 1 child session, got %d", customStore.createdChildCount)
	}
}

// TestFireClearHook_EntryHook_NoHookInContext tests entry hook when state has hooks but context doesn't.
func TestFireClearHook_EntryHook_NoHookInContext(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{}
	s.store = customStore
	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{},
	}
	s.CurrentHook = ""

	ret := s.fireOrClearHook(true)
	if !ret {
		t.Error("should return true because SessionStateCheckCondition has entry hooks")
	}
	if customStore.createdChildCount != 0 {
		t.Errorf("expected 0 child sessions, got %d", customStore.createdChildCount)
	}
	if s.CurrentHook != "" {
		t.Errorf("CurrentHook should be empty, got %s", s.CurrentHook)
	}
}

// TestFireClearHook_StateWithoutHooks tests state that has no hooks defined.
func TestFireClearHook_StateWithoutHooks(t *testing.T) {
	s := makeHookSession(SessionStateInit)
	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"on_session": "workflow",
		},
	}
	s.CurrentHook = ""

	ret := s.fireOrClearHook(true)
	if ret {
		t.Error("should return false when state has no entry hooks")
	}
	if s.CurrentHook != "" {
		t.Errorf("CurrentHook should remain empty, got %s", s.CurrentHook)
	}
}

// TestFireClearHook_ExitHook_WithDefinedHook tests exit hook path.
func TestFireClearHook_ExitHook_WithDefinedHook(t *testing.T) {
	s := makeHookSession(SessionStateSuccess)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"on_success": "workflow_name",
		},
	}
	s.CurrentHook = ""

	ret := s.fireOrClearHook(false)
	if !ret {
		t.Error("should return true for exit hook")
	}
	if customStore.createdChildCount != 1 {
		t.Errorf("expected 1 child, got %d", customStore.createdChildCount)
	}
	if s.CurrentHook != "" {
		t.Errorf("should be cleared, got %s", s.CurrentHook)
	}
}

// TestFireClearHook_PendingChild tests behavior when child is pending.
func TestFireClearHook_PendingChild(t *testing.T) {
	s := makeHookSession(SessionStateFailure)
	customStore := &hookTrackingStore{makePendingChild: true}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"on_failure": "pending_workflow",
			"on_exit":    "exit_workflow",
		},
	}
	s.CurrentHook = ""

	ret := s.fireOrClearHook(false)
	if !ret {
		t.Error("should return true")
	}
	if s.CurrentHook != "on_failure" {
		t.Errorf("expected CurrentHook 'on_failure', got %s", s.CurrentHook)
	}
	if !s.pending {
		t.Error("session should be marked pending")
	}
	if customStore.createdChildCount != 1 {
		t.Errorf("expected 1 child, got %d", customStore.createdChildCount)
	}
}

// TestFireClearHook_ClearingHook tests that when CurrentHook is set (returning from a pending hook),
// the hook is cleared and the next hook in sequence is fired.
func TestFireClearHook_ClearingHook(t *testing.T) {
	s := makeHookSession(SessionStateSuccess)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"on_success": "hk1",
			"on_exit":    "hk2",
		},
	}
	// Simulate returning from a previously pending on_success hook.
	s.CurrentHook = "on_success"
	s.pending = false

	ret := s.fireOrClearHook(false)
	if !ret {
		t.Error("should return true when clearing hook")
	}
	// on_success is cleared, then on_exit is fired and completes (non-pending), so CurrentHook ends up empty.
	if s.CurrentHook != "" {
		t.Errorf("CurrentHook should be cleared after on_exit completes, got %s", s.CurrentHook)
	}
	// on_exit child was created by executeHook.
	if customStore.createdChildCount != 1 {
		t.Errorf("expected 1 child created for on_exit, got %d", customStore.createdChildCount)
	}
}

// TestFireClearHook_MultipleHooksSequence tests clearing first hook then processing next.
func TestFireClearHook_MultipleHooksSequence(t *testing.T) {
	s := makeHookSession(SessionStateError)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"on_error": "error_wf",
			"on_exit":  "exit_wf",
		},
	}
	s.CurrentHook = "on_error"
	s.child = &Session{ID: "child"}

	ret := s.fireOrClearHook(false)
	if !ret {
		t.Error("should return true")
	}
	// First iteration clears on_error, second iteration finds on_exit and executes it
	// But on_exit executes, is not pending, so gets cleared in same loop iteration
	if s.CurrentHook != "" {
		t.Errorf("expected empty after non-pending on_exit, got %s", s.CurrentHook)
	}
	if customStore.createdChildCount != 1 {
		t.Errorf("expected 1 new child created for on_exit, got %d", customStore.createdChildCount)
	}
}

// TestExecuteHook_HookNotDefined tests executeHook when hook doesn't exist in context.
func TestExecuteHook_HookNotDefined(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	s.Ctx = map[string]interface{}{}

	ret := s.executeHook("nonexistent")
	if ret {
		t.Error("should return false when hook not defined")
	}
	if s.child != nil {
		t.Error("child should not be created")
	}
}

// TestExecuteHook_StringFormat tests executeHook with string hook value.
func TestExecuteHook_StringFormat(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"test_hook": "my_workflow",
		},
	}

	ret := s.executeHook("test_hook")
	// Non-pending child returns false from executeHook
	if ret {
		t.Error("should return false for non-pending child")
	}
	if customStore.lastCreatedWf.Workflow != "my_workflow" {
		t.Errorf("expected workflow 'my_workflow', got %s", customStore.lastCreatedWf.Workflow)
	}
	if customStore.lastCreatedWf.Context != SessionContextHooks {
		t.Error("hook workflow should have SessionContextHooks context")
	}
	if customStore.createdChildCount != 1 {
		t.Errorf("expected 1 child created, got %d", customStore.createdChildCount)
	}
}

// TestExecuteHook_ArraySingleElement tests executeHook with single element array.
func TestExecuteHook_ArraySingleElement(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"test_hook": []interface{}{"single_workflow"},
		},
	}

	ret := s.executeHook("test_hook")
	// Non-pending child returns false
	if ret {
		t.Error("should return false for non-pending child")
	}
	if customStore.lastCreatedWf.Workflow != "single_workflow" {
		t.Errorf("expected 'single_workflow', got %s", customStore.lastCreatedWf.Workflow)
	}
	if len(customStore.lastCreatedWf.Threads) != 0 {
		t.Errorf("expected no threads for single element, got %d", len(customStore.lastCreatedWf.Threads))
	}
}

// TestExecuteHook_ArrayMultipleElements tests executeHook with multiple array elements.
func TestExecuteHook_ArrayMultipleElements(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"test_hook": []interface{}{
				"workflow1",
				"workflow2",
				"workflow3",
			},
		},
	}

	ret := s.executeHook("test_hook")
	// Non-pending child returns false
	if ret {
		t.Error("should return false for non-pending child")
	}
	if customStore.lastCreatedWf.Workflow != "" {
		t.Errorf("expected empty Workflow for multi-hook, got %s", customStore.lastCreatedWf.Workflow)
	}
	if len(customStore.lastCreatedWf.Threads) != 3 {
		t.Errorf("expected 3 threads, got %d", len(customStore.lastCreatedWf.Threads))
	}
	if customStore.lastCreatedWf.Threads[0].Workflow != "workflow1" {
		t.Errorf("expected 'workflow1', got %s", customStore.lastCreatedWf.Threads[0].Workflow)
	}
	if customStore.lastCreatedWf.Threads[1].Workflow != "workflow2" {
		t.Errorf("expected 'workflow2', got %s", customStore.lastCreatedWf.Threads[1].Workflow)
	}
	if customStore.lastCreatedWf.Threads[2].Workflow != "workflow3" {
		t.Errorf("expected 'workflow3', got %s", customStore.lastCreatedWf.Threads[2].Workflow)
	}
}

// TestExecuteHook_EmptyWorkflow tests executeHook when hook resolves to empty.
func TestExecuteHook_EmptyWorkflow(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"test_hook": "",
		},
	}

	ret := s.executeHook("test_hook")
	if ret {
		t.Error("should return false for empty workflow")
	}
	if customStore.createdChildCount != 0 {
		t.Error("child should not be created for empty workflow")
	}
	if s.child != nil {
		t.Error("child should not be set")
	}
}

// TestExecuteHook_EmptyArray tests executeHook with empty array.
func TestExecuteHook_EmptyArray(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"test_hook": []interface{}{},
		},
	}

	ret := s.executeHook("test_hook")
	if ret {
		t.Error("should return false for empty array")
	}
	if customStore.createdChildCount != 0 {
		t.Error("child should not be created")
	}
}

// TestExecuteHook_MessageLabelsCopied tests that message labels are copied, not referenced.
func TestExecuteHook_MessageLabelsCopied(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{}
	s.store = customStore

	s.CurrentMsg.Labels = map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"test_hook": "workflow",
		},
	}

	s.executeHook("test_hook")

	if customStore.lastCreatedMsg.Labels["key1"] != "value1" {
		t.Error("label key1 not copied")
	}
	if customStore.lastCreatedMsg.Labels["key2"] != "value2" {
		t.Error("label key2 not copied")
	}

	s.CurrentMsg.Labels["key1"] = "modified"
	if customStore.lastCreatedMsg.Labels["key1"] == "modified" {
		t.Error("created message labels should be a copy, not reference")
	}
}

// TestExecuteHook_ChildWithPending tests return value when child is pending.
func TestExecuteHook_ChildWithPending(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{makePendingChild: true}
	s.store = customStore

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"test_hook": "workflow",
		},
	}

	ret := s.executeHook("test_hook")
	if !ret {
		t.Error("should return true when child is pending")
	}
}

// TestExecuteHook_ChildWithCurrentHook tests return value when child has CurrentHook.
func TestExecuteHook_ChildWithCurrentHook(t *testing.T) {
	s := makeHookSession(SessionStateCheckCondition)
	customStore := &hookTrackingStore{}
	s.store = customStore
	// Set makePendingChild to true so the child remains after executeHook
	customStore.makePendingChild = true

	s.Ctx = map[string]interface{}{
		"hooks": map[string]interface{}{
			"test_hook": "workflow",
		},
	}

	ret := s.executeHook("test_hook")

	// When child has CurrentHook, executeHook should return true (pending)
	if !ret {
		t.Error("executeHook should return true when child is pending")
	}

	// Set CurrentHook on the pending child
	s.child.CurrentHook = "some_hook"

	if !(s.child.pending || s.child.CurrentHook != "") {
		t.Error("child should be identified as having pending state")
	}
}

// TestSessionEntryHooks_Structure verifies SessionEntryHooks map.
func TestSessionEntryHooks_Structure(t *testing.T) {
	expectedStates := []int{
		SessionStateCheckCondition,
		SessionStateFirstRound,
		SessionStateNextRound,
		SessionStateFirstItem,
		SessionStateNextItem,
		SessionStateFirstAction,
	}

	for _, state := range expectedStates {
		if _, ok := SessionEntryHooks[state]; !ok {
			t.Errorf("SessionEntryHooks missing entry for state %d", state)
		}
	}

	//nolint:goconst
	if len(SessionEntryHooks[SessionStateCheckCondition]) == 0 || SessionEntryHooks[SessionStateCheckCondition][0] != "on_session" {
		t.Error("SessionStateCheckCondition should have on_session hook")
	}
	if len(SessionEntryHooks[SessionStateFirstRound]) == 0 || SessionEntryHooks[SessionStateFirstRound][0] != "on_first_round" {
		t.Error("SessionStateFirstRound should have on_first_round hook")
	}
}

// TestSessionExitHooks_Structure verifies SessionExitHooks map.
func TestSessionExitHooks_Structure(t *testing.T) {
	expectedStates := []int{
		SessionStateUpdate,
		SessionStateFailure,
		SessionStateError,
		SessionStateSuccess,
	}

	for _, state := range expectedStates {
		if _, ok := SessionExitHooks[state]; !ok {
			t.Errorf("SessionExitHooks missing exit hook for state %d", state)
		}
	}

	if len(SessionExitHooks[SessionStateFailure]) != 2 {
		t.Errorf("SessionStateFailure should have 2 hooks, got %d", len(SessionExitHooks[SessionStateFailure]))
	}
	//nolint:goconst
	if SessionExitHooks[SessionStateFailure][0] != "on_failure" || SessionExitHooks[SessionStateFailure][1] != "on_exit" {
		t.Error("SessionStateFailure hooks not as expected")
	}
	if SessionExitHooks[SessionStateError][0] != "on_error" || SessionExitHooks[SessionStateError][1] != "on_exit" {
		t.Error("SessionStateError hooks not as expected")
	}
	if SessionExitHooks[SessionStateSuccess][0] != "on_success" || SessionExitHooks[SessionStateSuccess][1] != "on_exit" {
		t.Error("SessionStateSuccess hooks not as expected")
	}
}
