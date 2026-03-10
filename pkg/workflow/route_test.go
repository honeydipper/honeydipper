// Copyright 2026 PayPal Inc.

//go:build !integration
// +build !integration

package workflow

import (
	"testing"

	cfg "github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/jellydator/ttlcache/v3"
	"github.com/op/go-logging"
)

func newMsg() *dipper.Message {
	return &dipper.Message{Labels: map[string]string{}, Payload: map[string]any{}}
}

// testStore is a minimal implementation of Store to satisfy methods used by tests.
type testStore struct{}

func (t *testStore) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (t *testStore) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	return nil
}

func (t *testStore) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (t *testStore) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}
func (t *testStore) GetName() string { return _testModule }

func (t *testStore) CallWithMessage(msg *dipper.Message) ([]byte, error) { return nil, nil }
func (t *testStore) CallWithMessageNoWait(msg *dipper.Message) error {
	return nil
}
func (t *testStore) GetConfig() *cfg.Config          { return &cfg.Config{} }
func (t *testStore) SendMessage(msg *dipper.Message) {}
func (t *testStore) CreateChildSession(parent *Session, wf *cfg.Workflow, msg *dipper.Message) *Session {
	return nil
}

func (t *testStore) CreateAsyncChildSession(parent *Session, wf *cfg.Workflow, msg *dipper.Message) *Session {
	return nil
}
func (t *testStore) ActivateSession(w *Session) {}
func (t *testStore) EmitResult(w *Session)      {}
func (t *testStore) StartSession(wf *cfg.Workflow, msg *dipper.Message, ctx map[string]interface{}) {
}
func (t *testStore) StartDynamicSession(spec *dipper.Message, ctx map[string]interface{}) {}
func (t *testStore) ContinueSession(ID string, msg *dipper.Message, child *Session)       {}
func (t *testStore) ResumeSession(key string, msg *dipper.Message) bool                   { return false }
func (t *testStore) GetNumSessions(getAll bool) int                                       { return 0 }
func (t *testStore) DumpSessions(cursor string) map[string]any                            { return nil }
func (t *testStore) Wait()                                                                {}
func (t *testStore) GetLogger() *logging.Logger                                           { return dipper.GetLogger(_testModule, "ERROR") }
func (s *testStore) Stop()                                                                {}
func (s *testStore) GetCache() *ttlcache.Cache[string, map[string]any] {
	return nil
}

func newSession(wf *cfg.Workflow) *Session {
	return &Session{
		Workflow:   wf,
		CurrentMsg: newMsg(),
		store:      &testStore{},
		Ctx:        map[string]interface{}{},
	}
}

func TestDetermineNextState_Default(t *testing.T) {
	s := newSession(&cfg.Workflow{})
	s.State = 42
	got := s.determineNextState()
	if got != 43 {
		t.Fatalf("expected 43 got %d", got)
	}
}

func TestDetermineNextState_AllStates(t *testing.T) {
	tests := []struct {
		name          string
		state         int
		workflow      *cfg.Workflow
		expectedState int
	}{
		{
			name:          "SessionStateElse returns SessionStateUpdate",
			state:         SessionStateElse,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateUpdate,
		},
		{
			name:          "SessionStateFailure returns SessionStateDone",
			state:         SessionStateFailure,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateDone,
		},
		{
			name:          "SessionStateError returns SessionStateDone",
			state:         SessionStateError,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateDone,
		},
		{
			name:          "SessionStateSuccess returns SessionStateDone",
			state:         SessionStateSuccess,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateDone,
		},
		{
			name:          "SessionStateCheckCondition routes to checkConditionState",
			state:         SessionStateCheckCondition,
			workflow:      &cfg.Workflow{If: []string{"false"}},
			expectedState: SessionStateDone,
		},
		{
			name:          "SessionStateCheckIteration routes to checkIterationState",
			state:         SessionStateCheckIteration,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateCheckSteps,
		},
		{
			name:          "SessionStateNextItem routes to routNextItemState",
			state:         SessionStateNextItem,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateCheckSteps,
		},
		{
			name:          "SessionStateCheckSteps routes to checkStepsState",
			state:         SessionStateCheckSteps,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateCheckAction,
		},
		{
			name:          "SessionStateCheckAction routes to checkActionState",
			state:         SessionStateCheckAction,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateUpdate,
		},
		{
			name:          "SessionStateUpdate routes to updateState",
			state:         SessionStateUpdate,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateCheckEndSteps,
		},
		{
			name:          "SessionStateCheckEndSteps routes to checkEndStepsState",
			state:         SessionStateCheckEndSteps,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateCheckEndIterations,
		},
		{
			name:          "SessionStateCheckEndIterations routes to checkEndIterationsState",
			state:         SessionStateCheckEndIterations,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateCheckEndRounds,
		},
		{
			name:          "SessionStateCheckEndRounds routes to checkEndRoundsState",
			state:         SessionStateCheckEndRounds,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateExport,
		},
		{
			name:          "SessionStateExport routes to exportState",
			state:         SessionStateExport,
			workflow:      &cfg.Workflow{},
			expectedState: SessionStateSuccess,
		},
		{
			name:          "unknown state increments by 1",
			state:         100,
			workflow:      &cfg.Workflow{},
			expectedState: 101,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSession(tt.workflow)
			s.State = tt.state
			got := s.determineNextState()
			if got != tt.expectedState {
				t.Fatalf("expected %d got %d", tt.expectedState, got)
			}
		})
	}
}

func TestRouteCheckConditionState_Cases(t *testing.T) {
	// case: condition false + else branch
	s := newSession(&cfg.Workflow{If: []string{"false"}, Else: map[string]any{"x": 1}})
	if st := s.routeCheckConditionState(); st != SessionStateElse {
		t.Fatalf("expected else state, got %d", st)
	}

	// case: condition false + no else -> done
	s = newSession(&cfg.Workflow{If: []string{"false"}})
	if st := s.routeCheckConditionState(); st != SessionStateDone {
		t.Fatalf("expected done state, got %d", st)
	}

	// case: condition true and loop -> check loop condition
	s = newSession(&cfg.Workflow{If: []string{"true"}, While: []string{"x"}})
	if st := s.routeCheckConditionState(); st != SessionStateCheckLoopCondition {
		t.Fatalf("expected check-loop state, got %d", st)
	}

	// case: condition true and not loop -> check iteration
	s = newSession(&cfg.Workflow{If: []string{"true"}})
	if st := s.routeCheckConditionState(); st != SessionStateCheckIteration {
		t.Fatalf("expected check-iteration state, got %d", st)
	}
}

func TestRouteCheckIterationState_Cases(t *testing.T) {
	// no iterate -> check steps
	s := newSession(&cfg.Workflow{})
	if st := s.routeCheckIterationState(); st != SessionStateCheckSteps {
		t.Fatalf("expected check-steps, got %d", st)
	}

	// iterate empty -> else
	s = newSession(&cfg.Workflow{Iterate: []any{}})
	if st := s.routeCheckIterationState(); st != SessionStateElse {
		t.Fatalf("expected else, got %d", st)
	}

	// iterate non-empty -> first item
	s = newSession(&cfg.Workflow{Iterate: []any{1, 2}})
	if st := s.routeCheckIterationState(); st != SessionStateFirstItem {
		t.Fatalf("expected first-item, got %d", st)
	}
}

func TestRoutNextItemState_Cases(t *testing.T) {
	s := newSession(&cfg.Workflow{})
	if st := s.routNextItemState(); st != SessionStateCheckSteps {
		t.Fatalf("expected check-steps, got %d", st)
	}

	s = newSession(&cfg.Workflow{IterateParallel: []any{1}})
	if st := s.routNextItemState(); st != SessionStateCheckAction {
		t.Fatalf("expected check-action for parallel iterate, got %d", st)
	}
}

func TestRouteCheckStepsState_Cases(t *testing.T) {
	// nil steps -> check action
	s := newSession(&cfg.Workflow{})
	if st := s.routeCheckStepsState(); st != SessionStateCheckAction {
		t.Fatalf("expected check-action, got %d", st)
	}

	// steps with items -> first step
	s = newSession(&cfg.Workflow{Steps: []cfg.Workflow{{}, {}}})
	if st := s.routeCheckStepsState(); st != SessionStateFirstStep {
		t.Fatalf("expected first-step, got %d", st)
	}

	// steps empty slice -> update
	s = newSession(&cfg.Workflow{Steps: []cfg.Workflow{}})
	if st := s.routeCheckStepsState(); st != SessionStateUpdate {
		t.Fatalf("expected update, got %d", st)
	}
}

func TestRouteCheckActionState_Cases(t *testing.T) {
	// noop when indexes zero
	s := newSession(&cfg.Workflow{})
	s.LoopCount = 0
	s.Iteration = 0
	s.Current = 0
	// ensure isNoop returns true for empty workflow
	if !s.checkIsNoop() {
		t.Fatalf("expected workflow to be noop")
	}
	if st := s.routeCheckActionState(); st != SessionStateUpdate {
		t.Fatalf("expected update for noop, got %d", st)
	}

	// first action when initial indexes and not noop
	s = newSession(&cfg.Workflow{Function: cfg.Function{Target: cfg.Action{System: "sys", Function: "f"}}})
	s.LoopCount = 0
	s.Iteration = 0
	s.Current = 0
	if s.checkIsNoop() {
		t.Fatalf("expected workflow not noop")
	}
	if st := s.routeCheckActionState(); st != SessionStateFirstAction {
		t.Fatalf("expected first-action, got %d", st)
	}

	// action when indexes non-zero
	s = newSession(&cfg.Workflow{})
	s.LoopCount = 1
	s.Iteration = 0
	s.Current = 0
	if st := s.routeCheckActionState(); st != SessionStateAction {
		t.Fatalf("expected action state, got %d", st)
	}
}

// additional route checks.
func TestRouteCheckLoopConditionState_Cases(t *testing.T) {
	s := newSession(&cfg.Workflow{})
	// no condition specified -> default true -> first round
	s.Workflow.While = []string{""} // While is truthy? but empty means false maybe; use non-empty
	s.Workflow.While = []string{"x"}
	if st := s.routeCheckLoopConditionState(); st != SessionStateFirstRound {
		t.Fatalf("expected first round when loop condition true, got %d", st)
	}
	// false path: override checkLoopCondition by setting While to empty string which is false
	s = newSession(&cfg.Workflow{})
	s.Workflow.While = []string{""}
	if st := s.routeCheckLoopConditionState(); st != SessionStateElse {
		t.Fatalf("expected else when loop condition false, got %d", st)
	}
}

func TestRouteCheckEndStepsState_Cases(t *testing.T) {
	// no steps or threads -> check end iterations
	s := newSession(&cfg.Workflow{})
	if st := s.routeCheckEndStepsState(); st != SessionStateCheckEndIterations {
		t.Fatalf("expected check-end-iterations when no steps/threads, got %d", st)
	}
	wf := &cfg.Workflow{Steps: []cfg.Workflow{{}}, Threads: []cfg.Workflow{{}}}
	s = newSession(wf)
	s.Current = 0
	if st := s.routeCheckEndStepsState(); st != SessionStateEndFirstStep {
		t.Fatalf("expected end-first-step when current 0, got %d", st)
	}
	s = newSession(wf)
	s.Current = 1
	if st := s.routeCheckEndStepsState(); st != SessionStateEndStep {
		t.Fatalf("expected end-step when current non-zero, got %d", st)
	}
}

func TestRouteEndStepState_Cases(t *testing.T) {
	// still have steps
	wf := &cfg.Workflow{Steps: []cfg.Workflow{{}, {}}, Threads: []cfg.Workflow{{}}}
	s := newSession(wf)
	s.Current = 0
	if st := s.routeEndStepState(); st != SessionStateNextStep {
		t.Fatalf("expected next-step when still steps, got %d", st)
	}
	if s.Current != 1 {
		t.Fatalf("expected current incremented, got %d", s.Current)
	}

	// beyond steps and threads -> check-end-iterations
	s = newSession(wf)
	s.Current = 2
	if st := s.routeEndStepState(); st != SessionStateCheckEndIterations {
		t.Fatalf("expected check-end-iterations when beyond both steps and threads, got %d", st)
	}

	// no steps but threads exist -> action
	wf2 := &cfg.Workflow{Steps: []cfg.Workflow{}, Threads: []cfg.Workflow{{}, {}}}
	s = newSession(wf2)
	s.Current = 0
	if st := s.routeEndStepState(); st != SessionStateAction {
		t.Fatalf("expected action when no steps but threads exist, got %d", st)
	}

	// only steps -> check end iterations when none left
	s = newSession(&cfg.Workflow{Steps: []cfg.Workflow{{}}})
	s.Current = 1
	if st := s.routeEndStepState(); st != SessionStateCheckEndIterations {
		t.Fatalf("expected check-end-iterations when none left, got %d", st)
	}
}

func TestRouteCheckEndIterationsState_Cases(t *testing.T) {
	s := newSession(&cfg.Workflow{})
	if st := s.routeCheckEndIterationsState(); st != SessionStateCheckEndRounds {
		t.Fatalf("expected check-end-rounds when not iteration, got %d", st)
	}
	s = newSession(&cfg.Workflow{Iterate: []any{1}})
	s.Iteration = 0
	if st := s.routeCheckEndIterationsState(); st != SessionStateEndFirstItem {
		t.Fatalf("expected end-first-item when iteration at zero, got %d", st)
	}
	s.Iteration = 1
	if st := s.routeCheckEndIterationsState(); st != SessionStateEndItem {
		t.Fatalf("expected end-item when iteration>0, got %d", st)
	}
}

func TestRouteEndItemState_Cases(t *testing.T) {
	wf := &cfg.Workflow{Iterate: []any{1, 2}}
	s := newSession(wf)
	s.Iteration = 0
	s.Current = 5
	if st := s.routeEndItemState(); st != SessionStateNextItem {
		t.Fatalf("expected next-item when more items, got %d", st)
	}
	if s.Iteration != 1 || s.Current != 0 {
		t.Fatalf("expected iteration 1 and current reset, got %d/%d", s.Iteration, s.Current)
	}
	s = newSession(wf)
	s.Iteration = 1
	if st := s.routeEndItemState(); st != SessionStateCheckEndRounds {
		t.Fatalf("expected check-end-rounds when last item, got %d", st)
	}
}

func TestRouteCheckEndRoundsState_Cases(t *testing.T) {
	s := newSession(&cfg.Workflow{})
	if st := s.routeCheckEndRoundsState(); st != SessionStateExport {
		t.Fatalf("expected export when not loop, got %d", st)
	}
	s = newSession(&cfg.Workflow{While: []string{"x"}})
	s.LoopCount = 0
	if st := s.routeCheckEndRoundsState(); st != SessionStateEndFirstRound {
		t.Fatalf("expected end-first-round when loopCount 0, got %d", st)
	}
	s.LoopCount = 1
	if st := s.routeCheckEndRoundsState(); st != SessionStateEndRound {
		t.Fatalf("expected end-round when loopCount>0, got %d", st)
	}
}

func TestRouteEndRoundState_Cases(t *testing.T) {
	wf := &cfg.Workflow{While: []string{"true"}}
	s := newSession(wf)
	s.LoopCount = 0
	if st := s.routeEndRoundState(); st != SessionStateNextRound {
		t.Fatalf("expected next-round when loop condition true, got %d", st)
	}
	if s.LoopCount != 1 {
		t.Fatalf("expected loopCount incremented, got %d", s.LoopCount)
	}
	if s.Iteration != 0 || s.Current != 0 {
		t.Fatalf("expected iteration/current reset, got %d/%d", s.Iteration, s.Current)
	}
	wf2 := &cfg.Workflow{While: []string{""}}
	s = newSession(wf2)
	s.LoopCount = 5
	if st := s.routeEndRoundState(); st != SessionStateExport {
		t.Fatalf("expected export when loop condition false, got %d", st)
	}
	if s.LoopCount != 6 {
		t.Fatalf("expected loopCount incremented even on export, got %d", s.LoopCount)
	}
}

func TestRouteExportState_Cases(t *testing.T) {
	// failure status -> SessionStateFailure
	s := newSession(&cfg.Workflow{})
	s.CurrentMsg.Labels["status"] = SessionStatusFailure
	if st := s.routeExportState(); st != SessionStateFailure {
		t.Fatalf("expected SessionStateFailure when status is failure, got %d", st)
	}

	// error status -> SessionStateError
	s = newSession(&cfg.Workflow{})
	s.CurrentMsg.Labels["status"] = SessionStatusError
	if st := s.routeExportState(); st != SessionStateError {
		t.Fatalf("expected SessionStateError when status is error, got %d", st)
	}

	// success status -> SessionStateSuccess
	s = newSession(&cfg.Workflow{})
	s.CurrentMsg.Labels["status"] = SessionStatusSuccess
	if st := s.routeExportState(); st != SessionStateSuccess {
		t.Fatalf("expected SessionStateSuccess when status is success, got %d", st)
	}

	// missing status label (default) -> SessionStateSuccess
	s = newSession(&cfg.Workflow{})
	// don't set status label
	if st := s.routeExportState(); st != SessionStateSuccess {
		t.Fatalf("expected SessionStateSuccess when status label is missing, got %d", st)
	}
}

func TestRouteUpdateState_Cases(t *testing.T) {
	// else branch present -> SessionStateExport
	s := newSession(&cfg.Workflow{})
	s.ElseBranch = &cfg.Workflow{}
	if st := s.routeUpdateState(); st != SessionStateExport {
		t.Fatalf("expected SessionStateExport when else branch present, got %d", st)
	}

	// failure status + on_failure==exit -> SessionStateExport
	s = newSession(&cfg.Workflow{OnFailure: "exit"})
	s.CurrentMsg.Labels["status"] = SessionStatusFailure
	if st := s.routeUpdateState(); st != SessionStateExport {
		t.Fatalf("expected SessionStateExport on failure with exit, got %d", st)
	}

	// failure status but on_failure != exit -> SessionStateCheckEndSteps (not export)
	s = newSession(&cfg.Workflow{OnFailure: "continue"})
	s.CurrentMsg.Labels["status"] = SessionStatusFailure
	if st := s.routeUpdateState(); st != SessionStateCheckEndSteps {
		t.Fatalf("expected SessionStateCheckEndSteps when failure but on_failure != exit, got %d", st)
	}

	// error status + on_error != continue -> SessionStateExport
	s = newSession(&cfg.Workflow{OnError: "stop"})
	s.CurrentMsg.Labels["status"] = SessionStatusError
	if st := s.routeUpdateState(); st != SessionStateExport {
		t.Fatalf("expected SessionStateExport on error with stop, got %d", st)
	}

	// error status + on_error == continue -> SessionStateCheckEndSteps (not export)
	s = newSession(&cfg.Workflow{OnError: "continue"})
	s.CurrentMsg.Labels["status"] = SessionStatusError
	if st := s.routeUpdateState(); st != SessionStateCheckEndSteps {
		t.Fatalf("expected SessionStateCheckEndSteps on error with continue, got %d", st)
	}

	// success status -> SessionStateCheckEndSteps (default path)
	s = newSession(&cfg.Workflow{})
	s.CurrentMsg.Labels["status"] = SessionStatusSuccess
	if st := s.routeUpdateState(); st != SessionStateCheckEndSteps {
		t.Fatalf("expected SessionStateCheckEndSteps on success, got %d", st)
	}

	// no status label (default) -> SessionStateCheckEndSteps
	s = newSession(&cfg.Workflow{})
	// don't set status label
	if st := s.routeUpdateState(); st != SessionStateCheckEndSteps {
		t.Fatalf("expected SessionStateCheckEndSteps with no status label, got %d", st)
	}
}
