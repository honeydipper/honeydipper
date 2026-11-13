// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package workflow

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	cfg "github.com/honeydipper/honeydipper/v4/internal/config"
)

const _value = "value"

// TestMarshal tests the Marshal method which converts a session to JSON bytes.
func TestMarshal(t *testing.T) {
	s := newSession(&cfg.Workflow{Name: "test"})
	//nolint:goconst
	s.ID, s.EventID = "test-id", "event-id"
	s.Ctx["key"] = _value

	data := s.Marshal()

	if data == nil {
		t.Fatal("Marshal returned nil")
	}

	if len(data) == 0 {
		t.Fatal("Marshal returned empty byte slice")
	}

	// Verify it's valid JSON
	var unmarshaled map[string]interface{}
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Marshal output is not valid JSON: %v", err)
	}
}

// TestUnmarshal tests the Unmarshal method which converts bytes back to a session.
func TestUnmarshal(t *testing.T) {
	original := newSession(&cfg.Workflow{Name: "test"})
	original.ID = "test-id"
	original.EventID = "event-id"
	original.Ctx["key"] = _value
	original.Current = 5
	original.Iteration = 3

	data := original.Marshal()

	s := &Session{}
	s.Unmarshal(data)

	if s.ID != "test-id" {
		t.Errorf("expected ID 'test-id', got '%s'", s.ID)
	}
	if s.EventID != "event-id" {
		t.Errorf("expected EventID 'event-id', got '%s'", s.EventID)
	}
	if s.Current != 5 {
		t.Errorf("expected Current 5, got %d", s.Current)
	}
	if s.Iteration != 3 {
		t.Errorf("expected Iteration 3, got %d", s.Iteration)
	}
}

// TestCancel tests the Cancel method which cancels the workflow.
func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := newSession(&cfg.Workflow{})
	s.context = ctx
	s.cancelFunc = cancel

	s.Cancel()

	// Check if context is cancelled
	select {
	case <-ctx.Done():
		// Context was cancelled successfully
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected context to be cancelled")
	}
}

// TestGetContext tests the GetContext method.
func TestGetContext(t *testing.T) {
	ctx := context.Background()
	s := newSession(&cfg.Workflow{})
	s.context = ctx

	result := s.GetContext()

	if result != ctx {
		t.Fatal("expected GetContext to return the session context")
	}
}

// TestBuildEnvData tests the buildEnvData method.
func TestBuildEnvData(t *testing.T) {
	s := newSession(&cfg.Workflow{Name: "test"})
	s.Ctx["ctx_key"] = "ctx_value"
	s.Event = map[string]interface{}{}
	s.Event["event_key"] = "event_value"
	s.CurrentMsg.Payload = map[string]interface{}{"data_key": "data_value"}
	s.CurrentMsg.Labels["label_key"] = "label_value"

	envData := s.buildEnvData()

	if envData["ctx"] == nil {
		t.Fatal("expected 'ctx' in envData")
	}
	if envData["event"] == nil {
		t.Fatal("expected 'event' in envData")
	}
	if envData["data"] == nil {
		t.Fatal("expected 'data' in envData")
	}
	if envData["labels"] == nil {
		t.Fatal("expected 'labels' in envData")
	}

	// Check metadata
	if s.Ctx["_meta_name"] != "test" {
		t.Errorf("expected _meta_name to be 'test', got %v", s.Ctx["_meta_name"])
	}
}

// TestBuildEnvData_NilPayload tests buildEnvData when payload is nil.
func TestBuildEnvData_NilPayload(t *testing.T) {
	s := newSession(&cfg.Workflow{})
	s.CurrentMsg.Payload = nil

	envData := s.buildEnvData()

	data := envData["data"].(map[string]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty data map when payload is nil")
	}
}

// TestBuildEnvData_IsHook tests buildEnvData when session is a hook.
func TestBuildEnvData_IsHook(t *testing.T) {
	s := newSession(&cfg.Workflow{Name: "test"})
	s.IsHook = true

	s.buildEnvData()

	if _, ok := s.Ctx["_meta_name"]; ok {
		t.Fatal("expected _meta_name not to be set for hook sessions")
	}
}

// TestInterpolateFunction tests the interpolateFunction method.
func TestInterpolateFunction(t *testing.T) {
	s := newSession(&cfg.Workflow{})

	f := &cfg.Function{
		Target: cfg.Action{
			System:   "test_system",
			Function: "test_function",
		},
	}

	result := s.interpolateFunction(f)

	if result.Target.System != "test_system" {
		t.Errorf("expected system 'test_system', got '%s'", result.Target.System)
	}
	if result.Target.Function != "test_function" {
		t.Errorf("expected function 'test_function', got '%s'", result.Target.Function)
	}

	// Ensure original is not modified
	if f.Target.System != "test_system" {
		t.Fatal("expected original function to remain unchanged")
	}
}

// TestSetPerforming tests the setPerforming method.
func TestSetPerforming(t *testing.T) {
	s := newSession(&cfg.Workflow{Name: "test"})
	p := make([]string, 10)
	s.Performing = &p
	s.depth = 0

	s.setPerforming("test action")

	if (*s.Performing)[0] == "" {
		t.Fatal("expected performing message to be set")
	}
}

// TestBrief_WithName tests the Brief method with workflow name.
func TestBrief_WithName(t *testing.T) {
	wf := &cfg.Workflow{Name: "my workflow"}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "my workflow" {
		t.Errorf("expected 'my workflow', got '%s'", brief)
	}
}

// TestBrief_WithDescription tests the Brief method with workflow description.
func TestBrief_WithDescription(t *testing.T) {
	wf := &cfg.Workflow{Description: "my description"}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "my description" {
		t.Errorf("expected 'my description', got '%s'", brief)
	}
}

// TestBrief_WithIteration tests the Brief method with iteration.
func TestBrief_WithIteration(t *testing.T) {
	wf := &cfg.Workflow{Iterate: []string{"a", "b"}}
	s := newSession(wf)

	brief := s.Brief()

	//nolint:goconst
	if brief != "iteration" {
		t.Errorf("expected 'iteration', got '%s'", brief)
	}
}

// TestBrief_WithIterateParallel tests the Brief method with iteration parallel.
func TestBrief_WithIterateParallel(t *testing.T) {
	wf := &cfg.Workflow{IterateParallel: []string{"a", "b"}}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "iteration" {
		t.Errorf("expected 'iteration', got '%s'", brief)
	}
}

func TestBrief_WithIterateParallelAndDescription(t *testing.T) {
	wf := &cfg.Workflow{Description: "item {{.ctx.current}}", IterateParallel: []string{"a", "b"}}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "parallel iteration" {
		t.Errorf("expected 'parallel iteration', got '%s'", brief)
	}
}

func TestBrief_ParallelIterationChildUsesDescription(t *testing.T) {
	store := &customStore{}
	parent := NewSession("parent", &cfg.Workflow{
		Description:     "item {{.ctx.current}}",
		IterateParallel: []any{"a"},
	}, store)
	parent.CurrentMsg = newMsg()
	parent.context = context.Background()
	parent.Ctx["current"] = "a"

	child := NewSession("child", &cfg.Workflow{Description: parent.Workflow.Description}, store)
	child.Init(newMsg(), parent, nil, nil, nil)

	brief := child.Brief()

	if brief != "item a" {
		t.Errorf("expected 'item a', got '%s'", brief)
	}
}

// TestBrief_WithLoop tests the Brief method with loop.
func TestBrief_WithLoop(t *testing.T) {
	wf := &cfg.Workflow{While: []string{"true"}}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "looping" {
		t.Errorf("expected 'looping', got '%s'", brief)
	}
}

// TestBrief_WithSystemFunction tests the Brief method with system function.
func TestBrief_WithSystemFunction(t *testing.T) {
	wf := &cfg.Workflow{Function: cfg.Function{Target: cfg.Action{System: "sys", Function: "func"}}}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "system func: sys.func" {
		t.Errorf("expected 'system func: sys.func', got '%s'", brief)
	}
}

// TestBrief_WithDriverFunction tests the Brief method with driver function.
func TestBrief_WithDriverFunction(t *testing.T) {
	wf := &cfg.Workflow{Function: cfg.Function{Driver: "driver", RawAction: "action"}}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "driver func: driver.action" {
		t.Errorf("expected 'driver func: driver.action', got '%s'", brief)
	}
}

// TestBrief_WithCallFunction tests the Brief method with call function.
func TestBrief_WithCallFunction(t *testing.T) {
	wf := &cfg.Workflow{CallFunction: "my_func"}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "system func: my_func" {
		t.Errorf("expected 'system func: my_func', got '%s'", brief)
	}
}

// TestBrief_WithCallDriver tests the Brief method with call driver.
func TestBrief_WithCallDriver(t *testing.T) {
	wf := &cfg.Workflow{CallDriver: "my_driver"}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "driver func: my_driver" {
		t.Errorf("expected 'driver func: my_driver', got '%s'", brief)
	}
}

// TestBrief_WithSendEvent tests the Brief method with send_event.
func TestBrief_WithSendEvent(t *testing.T) {
	wf := &cfg.Workflow{SendEvent: map[string]interface{}{"events": []interface{}{"a.b"}}}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "send event" {
		t.Errorf("expected 'send event', got '%s'", brief)
	}
}

// TestBrief_WithWorkflow tests the Brief method with workflow.
func TestBrief_WithWorkflow(t *testing.T) {
	wf := &cfg.Workflow{Workflow: "other_workflow"}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "wrapper: other_workflow" {
		t.Errorf("expected 'wrapper: other_workflow', got '%s'", brief)
	}
}

// TestBrief_WithSteps tests the Brief method with steps.
func TestBrief_WithSteps(t *testing.T) {
	wf := &cfg.Workflow{Steps: []cfg.Workflow{{Name: "step1"}}}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "steps" {
		t.Errorf("expected 'steps', got '%s'", brief)
	}
}

// TestBrief_WithThreads tests the Brief method with threads.
func TestBrief_WithThreads(t *testing.T) {
	wf := &cfg.Workflow{Threads: []cfg.Workflow{{Name: "thread1"}}}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "threads" {
		t.Errorf("expected 'threads', got '%s'", brief)
	}
}

// TestBrief_WithCases tests the Brief method with cases.
func TestBrief_WithCases(t *testing.T) {
	wf := &cfg.Workflow{Cases: map[string]interface{}{"case1": map[string]interface{}{}}}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "switch" {
		t.Errorf("expected 'switch', got '%s'", brief)
	}
}

// TestBrief_Default tests the Brief method with default (unnamed workflow).
func TestBrief_Default(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	brief := s.Brief()

	if brief != "unamed workflow" {
		t.Errorf("expected 'unamed workflow', got '%s'", brief)
	}
}

// TestIsLoop_WithWhile tests the isLoop method with while.
func TestIsLoop_WithWhile(t *testing.T) {
	wf := &cfg.Workflow{While: []string{"condition"}}
	s := newSession(wf)

	if !s.isLoop() {
		t.Fatal("expected isLoop to return true for while condition")
	}
}

// TestIsLoop_WithUntil tests the isLoop method with until.
func TestIsLoop_WithUntil(t *testing.T) {
	wf := &cfg.Workflow{Until: []string{"condition"}}
	s := newSession(wf)

	if !s.isLoop() {
		t.Fatal("expected isLoop to return true for until condition")
	}
}

// TestIsLoop_WithWhileAny tests the isLoop method with whileAny.
func TestIsLoop_WithWhileAny(t *testing.T) {
	wf := &cfg.Workflow{WhileAny: []string{"condition"}}
	s := newSession(wf)

	if !s.isLoop() {
		t.Fatal("expected isLoop to return true for whileAny condition")
	}
}

// TestIsLoop_WithUntilAll tests the isLoop method with untilAll.
func TestIsLoop_WithUntilAll(t *testing.T) {
	wf := &cfg.Workflow{UntilAll: []string{"condition"}}
	s := newSession(wf)

	if !s.isLoop() {
		t.Fatal("expected isLoop to return true for untilAll condition")
	}
}

// TestIsLoop_WithWhileMatch tests the isLoop method with whileMatch.
func TestIsLoop_WithWhileMatch(t *testing.T) {
	wf := &cfg.Workflow{WhileMatch: map[string]interface{}{"key": _value}}
	s := newSession(wf)

	if !s.isLoop() {
		t.Fatal("expected isLoop to return true for whileMatch condition")
	}
}

// TestIsLoop_WithUntilMatch tests the isLoop method with untilMatch.
func TestIsLoop_WithUntilMatch(t *testing.T) {
	wf := &cfg.Workflow{UntilMatch: map[string]interface{}{"key": _value}}
	s := newSession(wf)

	if !s.isLoop() {
		t.Fatal("expected isLoop to return true for untilMatch condition")
	}
}

// TestIsLoop_NoLoopCondition tests the isLoop method with no loop conditions.
func TestIsLoop_NoLoopCondition(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	if s.isLoop() {
		t.Fatal("expected isLoop to return false when no loop conditions are set")
	}
}

// TestIsIteration_WithIterate tests the isIteration method with iterate.
func TestIsIteration_WithIterate(t *testing.T) {
	wf := &cfg.Workflow{Iterate: []string{"a", "b"}}
	s := newSession(wf)

	if !s.isIteration() {
		t.Fatal("expected isIteration to return true for iterate")
	}
}

// TestIsIteration_WithIterateParallel tests the isIteration method with iterateParallel.
func TestIsIteration_WithIterateParallel(t *testing.T) {
	wf := &cfg.Workflow{IterateParallel: []string{"a", "b"}}
	s := newSession(wf)

	if !s.isIteration() {
		t.Fatal("expected isIteration to return true for iterateParallel")
	}
}

// TestIsIteration_NoIteration tests the isIteration method with no iteration.
func TestIsIteration_NoIteration(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	if s.isIteration() {
		t.Fatal("expected isIteration to return false when no iteration is set")
	}
}

// TestLenOfIterate_WithIterate tests the lenOfIterate method with iterate.
func TestLenOfIterate_WithIterate(t *testing.T) {
	wf := &cfg.Workflow{Iterate: []string{"a", "b", "c"}}
	s := newSession(wf)

	length := s.lenOfIterate()

	if length != 3 {
		t.Errorf("expected length 3, got %d", length)
	}
}

// TestLenOfIterate_WithIterateParallel tests the lenOfIterate method with iterateParallel.
func TestLenOfIterate_WithIterateParallel(t *testing.T) {
	wf := &cfg.Workflow{IterateParallel: []string{"a", "b"}}
	s := newSession(wf)

	length := s.lenOfIterate()

	if length != 2 {
		t.Errorf("expected length 2, got %d", length)
	}
}

// TestLenOfIterate_EmptyIterate tests the lenOfIterate method with empty iterate.
func TestLenOfIterate_EmptyIterate(t *testing.T) {
	wf := &cfg.Workflow{Iterate: []string{}}
	s := newSession(wf)

	length := s.lenOfIterate()

	if length != 0 {
		t.Errorf("expected length 0, got %d", length)
	}
}

// TestIsFunction_WithSystemFunction tests the isFunction method with driver function.
func TestIsFunction_WithSystemFunction(t *testing.T) {
	wf := &cfg.Workflow{Function: cfg.Function{Target: cfg.Action{System: "sys"}}}
	s := newSession(wf)

	if !s.isFunction() {
		t.Fatal("expected isFunction to return true for system function")
	}
}

// TestIsFunction_WithDriverFunction tests the isFunction method with driver function.
func TestIsFunction_WithDriverFunction(t *testing.T) {
	wf := &cfg.Workflow{Function: cfg.Function{Driver: "driver"}}
	s := newSession(wf)

	if !s.isFunction() {
		t.Fatal("expected isFunction to return true for driver function")
	}
}

// TestIsFunction_NoFunction tests the isFunction method with no function.
func TestIsFunction_NoFunction(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	if s.isFunction() {
		t.Fatal("expected isFunction to return false when no function is set")
	}
}

// TestWait tests the Wait method.
func TestWait(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	// Initialize threads with an empty WaitGroup
	s.threads = &sync.WaitGroup{}

	// Wait should not hang if threads WaitGroup is empty
	done := make(chan bool)
	go func() {
		s.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Wait returned successfully
	case <-time.After(1 * time.Second):
		t.Fatal("Wait() timed out")
	}
}

// TestGetStatus_Default tests the GetStatus method with default status.
func TestGetStatus_Default(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	status, _ := s.GetStatus()

	if status != SessionStatusSuccess {
		t.Errorf("expected status '%s', got '%s'", SessionStatusSuccess, status)
	}
}

// TestGetStatus_WithStatus tests the GetStatus method with status label set.
func TestGetStatus_WithStatus(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)
	s.CurrentMsg.Labels["status"] = SessionStatusFailure
	s.CurrentMsg.Labels["reason"] = "test reason"

	status, reason := s.GetStatus()

	if status != SessionStatusFailure {
		t.Errorf("expected status '%s', got '%s'", SessionStatusFailure, status)
	}
	if reason != "test reason" {
		t.Errorf("expected reason 'test reason', got '%s'", reason)
	}
}

// TestGetEventName_WithEvent tests the GetEventName method.
func TestGetEventName_WithEvent(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)
	s.Ctx["_meta_event"] = "my event"

	eventName := s.GetEventName()

	if eventName != "my event" {
		t.Errorf("expected event name 'my event', got '%s'", eventName)
	}
}

// TestGetEventName_NoEvent tests the GetEventName method when no event is set.
func TestGetEventName_NoEvent(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	eventName := s.GetEventName()

	if eventName != "" {
		t.Errorf("expected empty event name, got '%s'", eventName)
	}
}

// TestCheckIsNoop_CachedNoop tests the checkIsNoop method with cached result.
func TestCheckIsNoop_CachedNoop(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	trueVal := true
	s.IsNoop = &trueVal

	result := s.checkIsNoop()

	if !result {
		t.Fatal("expected checkIsNoop to return true")
	}
}

// TestCheckIsNoop_WithWorkflow tests the checkIsNoop method with workflow.
func TestCheckIsNoop_WithWorkflow(t *testing.T) {
	wf := &cfg.Workflow{Workflow: "other_workflow"}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for workflow")
	}
}

// TestCheckIsNoop_WithFunction tests the checkIsNoop method with function.
func TestCheckIsNoop_WithFunction(t *testing.T) {
	wf := &cfg.Workflow{Function: cfg.Function{Driver: "driver"}}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for function")
	}
}

// TestCheckIsNoop_WithCallDriver tests the checkIsNoop method with callDriver.
func TestCheckIsNoop_WithCallDriver(t *testing.T) {
	wf := &cfg.Workflow{CallDriver: "driver"}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for callDriver")
	}
}

// TestCheckIsNoop_WithCallFunction tests the checkIsNoop method with callFunction.
func TestCheckIsNoop_WithCallFunction(t *testing.T) {
	wf := &cfg.Workflow{CallFunction: "func"}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for callFunction")
	}
}

// TestCheckIsNoop_WithSendEvent tests the checkIsNoop method with send_event.
func TestCheckIsNoop_WithSendEvent(t *testing.T) {
	wf := &cfg.Workflow{SendEvent: map[string]interface{}{"events": []interface{}{"a.b"}}}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for eventbus_message")
	}
}

// TestCheckIsNoop_WithSteps tests the checkIsNoop method with steps.
func TestCheckIsNoop_WithSteps(t *testing.T) {
	wf := &cfg.Workflow{Steps: []cfg.Workflow{{Name: "step1"}}}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for steps")
	}
}

// TestCheckIsNoop_WithThreads tests the checkIsNoop method with threads.
func TestCheckIsNoop_WithThreads(t *testing.T) {
	wf := &cfg.Workflow{Threads: []cfg.Workflow{{Name: "thread1"}}}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for threads")
	}
}

// TestCheckIsNoop_WithWait tests the checkIsNoop method with wait.
func TestCheckIsNoop_WithWait(t *testing.T) {
	wf := &cfg.Workflow{Wait: "complete"}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for wait")
	}
}

// TestCheckIsNoop_WithSwitch tests the checkIsNoop method with switch.
func TestCheckIsNoop_WithSwitch(t *testing.T) {
	wf := &cfg.Workflow{Switch: "condition"}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for switch")
	}
}

// TestCheckIsNoop_WithResume tests the checkIsNoop method with resume.
func TestCheckIsNoop_WithResume(t *testing.T) {
	wf := &cfg.Workflow{Resume: "resume_point"}
	s := newSession(wf)

	result := s.checkIsNoop()

	if result {
		t.Fatal("expected checkIsNoop to return false for resume")
	}
}

// TestCheckIsNoop_Empty tests the checkIsNoop method with empty workflow (should be noop).
func TestCheckIsNoop_Empty(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	result := s.checkIsNoop()

	if !result {
		t.Fatal("expected checkIsNoop to return true for empty workflow")
	}

	// Verify it's cached
	if s.IsNoop == nil {
		t.Fatal("expected IsNoop to be cached")
	}
	if !*s.IsNoop {
		t.Fatal("expected IsNoop to be true")
	}
}

// TestIncCursor tests the incCursor method.
func TestIncCursor(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)
	s.CurrentMsg.Labels["cursor"] = "0"

	s.incCursor()

	if s.CurrentMsg.Labels["cursor"] != "1" {
		t.Errorf("expected cursor '1', got '%s'", s.CurrentMsg.Labels["cursor"])
	}

	s.incCursor()

	if s.CurrentMsg.Labels["cursor"] != "2" {
		t.Errorf("expected cursor '2', got '%s'", s.CurrentMsg.Labels["cursor"])
	}
}

// TestIncCursor_No initualCursor tests incCursor when cursor label doesn't exist.
func TestIncCursor_NoInitialCursor(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)
	// Don't set cursor label

	s.incCursor()

	if s.CurrentMsg.Labels["cursor"] != "1" {
		t.Errorf("expected cursor '1', got '%s'", s.CurrentMsg.Labels["cursor"])
	}
}

// TestDump_NotDone tests the Dump method when session is not done.
func TestDump_NotDone(t *testing.T) {
	wf := &cfg.Workflow{Description: "test workflow"}
	s := newSession(wf)
	s.State = SessionStateInit
	p := make([]string, 1)
	s.Performing = &p
	(*s.Performing)[0] = "action"
	s.Ctx["_output"] = "result"

	dump := s.Dump()

	if dump == nil {
		t.Fatal("expected Dump to return a map")
	}

	if _, ok := dump["data"]; !ok {
		t.Fatal("expected 'data' in dump")
	}

	if _, ok := dump["labels"]; !ok {
		t.Fatal("expected 'labels' in dump")
	}

	if _, ok := dump["performing"]; !ok {
		t.Fatal("expected 'performing' in dump")
	}

	data := dump["data"].(map[string]any)
	if data["description"] != "test workflow" {
		t.Errorf("expected description 'test workflow', got %v", data["description"])
	}
}

// TestDump_Done_Success tests the Dump method when session is done with success.
func TestDump_Done_Success(t *testing.T) {
	wf := &cfg.Workflow{Description: "test workflow"}
	s := newSession(wf)
	s.State = SessionStateDone
	s.CurrentMsg.Labels["status"] = SessionStatusSuccess
	s.Ctx["_output"] = "result"

	dump := s.Dump()

	if dump == nil {
		t.Fatal("expected Dump to return a map")
	}

	if _, ok := dump["performing"]; ok {
		t.Fatal("expected 'performing' not to be in dump for done success")
	}
}

// TestDump_Done_Error tests the Dump method when session is done with error.
func TestDump_Done_Error(t *testing.T) {
	wf := &cfg.Workflow{Description: "test workflow"}
	s := newSession(wf)
	s.State = SessionStateDone
	s.CurrentMsg.Labels["status"] = SessionStatusError
	s.CurrentMsg.Labels["performing"] = "action1\naction2"

	dump := s.Dump()

	if dump == nil {
		t.Fatal("expected Dump to return a map")
	}

	if _, ok := dump["performing"]; !ok {
		t.Fatal("expected 'performing' in dump for done error")
	}

	performing := dump["performing"].([]string)
	if len(performing) != 2 {
		t.Errorf("expected 2 performing items, got %d", len(performing))
	}
}

// TestDump_Done_Failure tests the Dump method when session is done with failure.
func TestDump_Done_Failure(t *testing.T) {
	wf := &cfg.Workflow{Description: "test workflow"}
	s := newSession(wf)
	s.State = SessionStateDone
	s.CurrentMsg.Labels["status"] = SessionStatusFailure
	s.CurrentMsg.Labels["performing"] = "action1\naction2"

	dump := s.Dump()

	if dump == nil {
		t.Fatal("expected Dump to return a map")
	}

	if _, ok := dump["performing"]; !ok {
		t.Fatal("expected 'performing' in dump for done failure")
	}

	performing := dump["performing"].([]string)
	if len(performing) != 2 {
		t.Errorf("expected 2 performing items, got %d", len(performing))
	}
}

// TestDump_Done_Cancelled tests the Dump method when session is done and cancelled.
func TestDump_Done_Cancelled(t *testing.T) {
	wf := &cfg.Workflow{Description: "test workflow"}
	s := newSession(wf)
	s.State = SessionStateDone
	s.Cancelled = true
	s.CurrentMsg.Labels["status"] = SessionStatusSuccess
	s.CurrentMsg.Labels["performing"] = "action1\naction2"

	dump := s.Dump()

	if dump == nil {
		t.Fatal("expected Dump to return a map")
	}

	data := dump["data"].(map[string]any)
	if data["state"] != "cancelled" {
		t.Fatalf("expected cancelled state, got %v", data["state"])
	}

	if _, ok := dump["performing"]; !ok {
		t.Fatal("expected 'performing' in dump for done cancelled session")
	}

	performing := dump["performing"].([]string)
	if len(performing) != 2 {
		t.Errorf("expected 2 performing items, got %d", len(performing))
	}
	if performing[0] != "action1" || performing[1] != "action2" {
		t.Fatalf("unexpected performing values: %+v", performing)
	}
	if data["is_noop"] != false {
		t.Fatalf("expected cancelled session not to be marked noop, got %v", data["is_noop"])
	}
}

// TestDump_Labels tests the Dump method properly copies labels.
func TestDump_Labels(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)
	s.CurrentMsg.Labels["test_label"] = "test_value"
	s.CurrentMsg.Labels["performing"] = "some_action"

	dump := s.Dump()

	labels := dump["labels"].(map[string]string)
	if labels["test_label"] != "test_value" {
		t.Errorf("expected test_label 'test_value', got '%s'", labels["test_label"])
	}

	// Verify performing label is removed
	if _, ok := labels["performing"]; ok {
		t.Fatal("expected 'performing' label to be removed from dump")
	}
}

// TestDump_WithChild tests the Dump method when there are child sessions.
func TestDump_WithChild(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)
	s.State = SessionStateInit
	p := make([]string, 2)
	s.Performing = &p
	(*s.Performing)[0] = "parent_action"

	childWf := &cfg.Workflow{}
	s.child = newSession(childWf)
	(*s.Performing)[1] = "child_action"

	dump := s.Dump()

	performing := dump["performing"].([]string)
	if performing[1] != "child_action" {
		t.Errorf("expected child performing, got %v", performing)
	}
}

// TestInterpolateFunction_NoInterpolation tests interpolateFunction with no variables to interpolate.
func TestInterpolateFunction_NoInterpolation(t *testing.T) {
	s := newSession(&cfg.Workflow{})

	f := &cfg.Function{
		Target: cfg.Action{
			System:   "fixed_system",
			Function: "fixed_function",
		},
	}

	result := s.interpolateFunction(f)

	if result.Target.System != "fixed_system" {
		t.Errorf("expected system 'fixed_system', got '%s'", result.Target.System)
	}
	if result.Target.Function != "fixed_function" {
		t.Errorf("expected function 'fixed_function', got '%s'", result.Target.Function)
	}
}

// TestBuildEnvData_EmptyLabels tests buildEnvData with nil labels.
func TestBuildEnvData_EmptyLabels(t *testing.T) {
	s := newSession(&cfg.Workflow{})
	s.CurrentMsg.Labels = nil

	envData := s.buildEnvData()

	if envData["labels"] == nil {
		t.Fatal("expected 'labels' in envData")
	}
}

// TestGetEventName_WrongType tests GetEventName when _meta_event is not a string.
func TestGetEventName_WrongType(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)
	s.Ctx["_meta_event"] = 123 // Not a string

	// This should panic or handle gracefully, but the implementation just type asserts
	// We expect it to panic based on the current implementation
	defer func() {
		if r := recover(); r == nil {
			// If no panic, that's also acceptable depending on implementation
		}
	}()

	_ = s.GetEventName()
}

// TestDump_BriefDescription tests the Dump method reads the brief correctly.
func TestDump_BriefDescription(t *testing.T) {
	wf := &cfg.Workflow{Name: "brief test"}
	s := newSession(wf)
	s.State = SessionStateDone
	s.CurrentMsg.Labels["status"] = SessionStatusSuccess

	dump := s.Dump()

	data := dump["data"].(map[string]any)
	if data["brief"] != "brief test" {
		t.Errorf("expected brief 'brief test', got %v", data["brief"])
	}
}

// TestDump_LogStream tests that Dump exposes _log_stream from Ctx as log_stream in data.
func TestDump_LogStream(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)
	s.Ctx["_log_stream"] = map[string]interface{}{
		"pod_id":   "test-pod-123",
		"provider": "podman",
	}

	dump := s.Dump()
	data := dump["data"].(map[string]any)

	logStream, ok := data["log_stream"]
	if !ok {
		t.Fatal("expected 'log_stream' in dump data")
	}

	logStreamMap, ok := logStream.(map[string]interface{})
	if !ok {
		t.Fatalf("expected log_stream to be a map, got %T", logStream)
	}

	if logStreamMap["pod_id"] != "test-pod-123" {
		t.Errorf("expected pod_id 'test-pod-123', got %v", logStreamMap["pod_id"])
	}

	if logStreamMap["provider"] != "podman" {
		t.Errorf("expected provider 'podman', got %v", logStreamMap["provider"])
	}
}

// TestDump_LogStream_Absent tests that log_stream is nil in data when unset.
func TestDump_LogStream_Absent(t *testing.T) {
	wf := &cfg.Workflow{}
	s := newSession(wf)

	dump := s.Dump()
	data := dump["data"].(map[string]any)

	if logStream, ok := data["log_stream"]; ok && logStream != nil {
		t.Errorf("expected log_stream to be nil when not set, got %v", logStream)
	}
}

// TestDump_LogStream_FromActiveChild tests that Dump can read _log_stream from active child chain.
func TestDump_LogStream_FromActiveChild(t *testing.T) {
	parent := newSession(&cfg.Workflow{})
	child := newSession(&cfg.Workflow{})
	parent.child = child

	child.Ctx["_log_stream"] = map[string]interface{}{
		"pod_id":   "child-pod-456",
		"provider": "podman",
	}

	dump := parent.Dump()
	data := dump["data"].(map[string]any)

	logStreamMap, ok := data["log_stream"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected log_stream map from child, got %T", data["log_stream"])
	}

	if logStreamMap["pod_id"] != "child-pod-456" {
		t.Errorf("expected child pod_id 'child-pod-456', got %v", logStreamMap["pod_id"])
	}
}

// TestDump_LogStream_PrefersNearestSession tests resolver picks the nearest _log_stream in chain.
func TestDump_LogStream_PrefersNearestSession(t *testing.T) {
	root := newSession(&cfg.Workflow{})
	child := newSession(&cfg.Workflow{})
	grandchild := newSession(&cfg.Workflow{})
	root.child = child
	child.child = grandchild

	child.Ctx["_log_stream"] = map[string]interface{}{
		"pod_id": "child-pod",
	}
	grandchild.Ctx["_log_stream"] = map[string]interface{}{
		"pod_id": "grandchild-pod",
	}

	dump := root.Dump()
	data := dump["data"].(map[string]any)

	logStreamMap := data["log_stream"].(map[string]interface{})
	if logStreamMap["pod_id"] != "child-pod" {
		t.Errorf("expected nearest child pod_id 'child-pod', got %v", logStreamMap["pod_id"])
	}
}

func TestDump_InteractiveOptions_FromWaitingLeaf(t *testing.T) {
	root := newSession(&cfg.Workflow{})
	root.State = SessionStateAction

	leaf := newSession(&cfg.Workflow{Wait: "5s"})
	leaf.State = SessionStateAction
	leaf.Ctx["interactive_options"] = map[string]any{
		"kind": "approval",
		"choices": []any{
			map[string]any{"label": "approve", "value": "yes"},
		},
	}
	root.child = leaf

	dump := root.Dump()
	data := dump["data"].(map[string]any)

	options, ok := data["interactive_options"].(map[string]any)
	if !ok {
		t.Fatalf("expected interactive_options map in dump, got %T", data["interactive_options"])
	}
	if options["kind"] != "approval" {
		t.Fatalf("expected interactive_options.kind approval, got %v", options["kind"])
	}
}

func TestDump_InteractiveOptions_IgnoredWhenLeafNotWaiting(t *testing.T) {
	root := newSession(&cfg.Workflow{})
	root.State = SessionStateAction

	leaf := newSession(&cfg.Workflow{})
	leaf.State = SessionStateAction
	leaf.Ctx["interactive_options"] = map[string]any{"kind": "approval"}
	root.child = leaf

	dump := root.Dump()
	data := dump["data"].(map[string]any)

	if _, ok := data["interactive_options"]; ok {
		t.Fatal("expected interactive_options to be absent when leaf is not waiting")
	}
}

func TestDump_InteractiveInteractions_AlwaysIncludedWhenPresent(t *testing.T) {
	root := newSession(&cfg.Workflow{})
	root.State = SessionStateDone

	leaf := newSession(&cfg.Workflow{})
	leaf.State = SessionStateDone
	leaf.Ctx["interactive_interactions"] = []any{
		map[string]any{
			"key":   "approve",
			"label": "Approve",
			"user":  "charles",
			"at":    "2026-05-10T12:00:00Z",
		},
	}
	root.child = leaf

	dump := root.Dump()
	data := dump["data"].(map[string]any)

	interactions, ok := data["interactive_interactions"].([]any)
	if !ok {
		t.Fatalf("expected interactive_interactions array in dump, got %T", data["interactive_interactions"])
	}
	if len(interactions) != 1 {
		t.Fatalf("expected exactly one interaction entry, got %+v", interactions)
	}
}

func TestDump_InteractiveInteractions_DedupesRootAndChildCopies(t *testing.T) {
	root := newSession(&cfg.Workflow{})
	root.State = SessionStateAction

	entry := map[string]any{
		"key":   "approve",
		"label": "Approve",
		"user":  "charles",
		"at":    "2026-05-10T12:00:00Z",
	}
	root.Ctx["interactive_interactions"] = []any{entry}

	leaf := newSession(&cfg.Workflow{Wait: "5s"})
	leaf.State = SessionStateAction
	leaf.Ctx["interactive_interactions"] = []any{entry}
	root.child = leaf

	dump := root.Dump()
	data := dump["data"].(map[string]any)

	interactions, ok := data["interactive_interactions"].([]any)
	if !ok {
		t.Fatalf("expected interactive_interactions array in dump, got %T", data["interactive_interactions"])
	}
	if len(interactions) != 1 {
		t.Fatalf("expected root/child duplicate entries to collapse to one, got %+v", interactions)
	}
}

func TestDump_RerunAvailability(t *testing.T) {
	s := newSession(&cfg.Workflow{Name: "rerunnable"})
	s.State = SessionStateDone
	s.CurrentMsg.Labels["status"] = SessionStatusSuccess

	dump := s.Dump()
	data := dump["data"].(map[string]any)
	rerun := data["rerun"].(map[string]any)
	if rerun["available"] != true {
		t.Fatalf("expected rerun to be available for root session, got %+v", rerun)
	}

	child := newSession(&cfg.Workflow{Name: "child"})
	child.parent = s
	dump = child.Dump()
	data = dump["data"].(map[string]any)
	rerun = data["rerun"].(map[string]any)
	if rerun["available"] != false {
		t.Fatalf("expected rerun to be unavailable for child session, got %+v", rerun)
	}
}

// TestLenOfIterate_ReflectValue tests lenOfIterate correctly uses reflect.
func TestLenOfIterate_ReflectValue(t *testing.T) {
	wf := &cfg.Workflow{Iterate: map[string]interface{}{"a": "1", "b": "2", "c": "3"}}
	s := newSession(wf)

	// Iterate could be a map, which should also have a length
	length := s.lenOfIterate()

	if length != 3 {
		t.Errorf("expected length 3, got %d", length)
	}
}
