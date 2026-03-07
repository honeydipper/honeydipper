//go:build !integration
// +build !integration

package workflow

import (
	"sync"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
)

const _testModule = "test"

// execTestStore implements the Store interface with hooks for inspecting calls.
type execTestStore struct {
	lastSentMessages []*dipper.Message
	lastCallFeature  string
	lastCallMethod   string
	lastCallParams   interface{}
	lastCallLabels   []string
	resumeReturn     bool
	workflows        map[string]config.Workflow

	createChildCalls int
	lastCreatedWf    *config.Workflow
	activateCalls    int
	asyncCreateCalls int
	lastAsyncChild   *Session

	// if true, any session created via CreateChildSession will have pending=true
	childMakePending bool
}

func (s *execTestStore) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	s.lastCallFeature = feature
	s.lastCallMethod = method
	s.lastCallParams = params
	s.lastCallLabels = labelsKV

	return nil, nil
}

func (s *execTestStore) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	return nil
}

func (s *execTestStore) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (s *execTestStore) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}
func (s *execTestStore) GetName() string                                     { return _testModule }
func (s *execTestStore) CallWithMessage(msg *dipper.Message) ([]byte, error) { return nil, nil }
func (s *execTestStore) CallWithMessageNoWait(msg *dipper.Message) error     { return nil }
func (s *execTestStore) GetConfig() *config.Config {
	return &config.Config{DataSet: &config.DataSet{Workflows: s.workflows}}
}

func (s *execTestStore) SendMessage(msg *dipper.Message) {
	s.lastSentMessages = append(s.lastSentMessages, msg)
}

func (s *execTestStore) CreateChildSession(parent *Session, wf *config.Workflow, msg *dipper.Message) *Session {
	s.createChildCalls++
	s.lastCreatedWf = wf
	wg := &sync.WaitGroup{}
	child := &Session{ID: "child", Workflow: wf, CurrentMsg: msg, store: s, threads: wg, Ctx: map[string]interface{}{}}
	if s.childMakePending {
		child.pending = true
	}

	return child
}

func (s *execTestStore) CreateAsyncChildSession(parent *Session, wf *config.Workflow, msg *dipper.Message) *Session {
	s.asyncCreateCalls++
	wg := &sync.WaitGroup{}
	child := &Session{ID: "async", Workflow: wf, CurrentMsg: msg, store: s, threads: wg, Ctx: map[string]interface{}{}}
	s.lastAsyncChild = child

	return child
}
func (s *execTestStore) ActivateSession(w *Session) { s.activateCalls++ }
func (s *execTestStore) EmitResult(w *Session)      {}
func (s *execTestStore) StartSession(wf *config.Workflow, msg *dipper.Message, ctx map[string]interface{}) {
}
func (s *execTestStore) StartDynamicSession(spec *dipper.Message, ctx map[string]interface{}) {}
func (s *execTestStore) ContinueSession(ID string, msg *dipper.Message)                       {}
func (s *execTestStore) ResumeSession(key string, msg *dipper.Message) bool {
	return s.resumeReturn
}
func (s *execTestStore) GetNumSessions(getAll bool) int            { return 0 }
func (s *execTestStore) DumpSessions(cursor string) map[string]any { return nil }
func (s *execTestStore) Wait()                                     {}
func (s *execTestStore) GetLogger() *logging.Logger {
	if dipper.Logger == nil {
		dipper.GetLogger(_testModule, "ERROR")
	}

	return dipper.Logger
}

// makeExecuteSession builds a basic session ready for execute tests.
func makeExecuteSession() *Session {
	state := SessionStateAction
	if dipper.Logger == nil {
		dipper.GetLogger(_testModule, "ERROR")
	}
	wg := &sync.WaitGroup{}

	return &Session{
		ID:         "exec",
		Workflow:   &config.Workflow{},
		CurrentMsg: &dipper.Message{Labels: map[string]string{"cursor": "0"}, Payload: map[string]any{}},
		store:      &execTestStore{},
		Ctx:        map[string]interface{}{},
		State:      state,
		Performing: []string{"init"},
		threads:    wg,
		StartTime:  time.Now(),
	}
}

func TestCallFunction_SendsMessage(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	// prepare a function
	f := &config.Function{Target: config.Action{System: "sys", Function: "fun"}}
	s.CurrentMsg.Labels["cursor"] = "5"

	s.callFunction(f)

	if len(es.lastSentMessages) != 1 {
		t.Fatalf("expected one message sent, got %d", len(es.lastSentMessages))
	}
	msg := es.lastSentMessages[0]
	if msg.Channel != dipper.ChannelEventbus || msg.Subject != "command" {
		t.Errorf("unexpected message channel/subject: %s/%s", msg.Channel, msg.Subject)
	}
	// payload should contain function
	pm, ok := msg.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload is not map: %v", msg.Payload)
	}
	payloadFunc, ok := pm["function"].(config.Function)
	if !ok || payloadFunc.Target.Function != "fun" {
		t.Errorf("function payload missing or wrong: %v", pm)
	}
	// cursor should have been incremented
	if msg.Labels["cursor"] != "6" {
		t.Errorf("cursor expected 6 got %s", msg.Labels["cursor"])
	}
}

func TestCallDriver_And_Shorthand(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es

	// driver call should delegate to callFunction
	s.callDriver("dri.act")
	if len(es.lastSentMessages) != 1 {
		t.Fatal("driver call should send message")
	}
	msg := es.lastSentMessages[0]
	pm, _ := msg.Payload.(map[string]any)
	if payloadFunc, ok := pm["function"].(config.Function); !ok || payloadFunc.Driver != "dri" {
		t.Errorf("callDriver payload mismatch: %v", pm)
	}

	// shorthand function
	es.lastSentMessages = nil
	s.callShorthandFunction("sys.act")
	if len(es.lastSentMessages) != 1 {
		t.Fatal("shorthand call should send message")
	}
	msg = es.lastSentMessages[0]
	pm = msg.Payload.(map[string]any)
	if payloadFunc, ok := pm["function"].(config.Function); !ok || payloadFunc.Target.System != "sys" {
		t.Errorf("callShorthandFunction payload mismatch: %v", pm)
	}
}

func TestExecuteSwitch_Branches(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es

	s.Workflow.Switch = "match"
	s.Workflow.Cases = map[string]interface{}{"match": map[string]any{"Workflow": "w1"}}

	s.executeSwitch()
	if es.createChildCalls != 1 {
		t.Error("expected one child for matching case")
	}
	// default branch must not be executed

	// default case
	es.createChildCalls = 0
	s.Workflow.Switch = "nomatch"
	s.Workflow.Default = &map[string]any{"Workflow": "def"}
	s.executeSwitch()
	if es.createChildCalls != 1 {
		t.Error("expected one child for default case")
	}
}

func TestEnterWait_CallsScheduler(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Ctx["timeout_message"] = map[string]any{"labels": map[string]any{"foo": "bar"}}

	s.Workflow.Wait = "1s"
	s.enterWait()
	if !s.pending {
		t.Error("enterWait should set pending")
	}
	if es.lastCallFeature != "scheduler" || es.lastCallMethod != "once" {
		t.Errorf("scheduler not called, got %s %s", es.lastCallFeature, es.lastCallMethod)
	}
	params, ok := es.lastCallParams.(map[string]any)
	if !ok || params["type"] != "session" {
		t.Errorf("unexpected params: %v", es.lastCallParams)
	}
}

func TestTriggerResume(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{resumeReturn: true}
	s.store = es

	s.Workflow.Resume = "foo"
	s.Ctx["resume_message"] = map[string]any{"labels": map[string]string{"a": "b"}}

	s.triggerResume()
	if s.CurrentMsg.Labels["status"] != SessionStatusSuccess {
		t.Error("expected success status")
	}
	// failure path
	es.resumeReturn = false
	s.Ctx["fail_if_missing"] = true
	s.triggerResume()
	if s.CurrentMsg.Labels["status"] != SessionStatusFailure {
		t.Error("expected failure status")
	}
}

func TestLaunchParallelIterations_and_ExecuteBranches(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es

	// iterate parallel branch
	s.Workflow.IterateParallel = []any{1, 2, 3}
	s.Workflow.IteratePool = "2"
	s.execute()
	if !s.pending {
		t.Error("parallel iterate should set pending")
	}
	if es.asyncCreateCalls == 0 {
		t.Error("should have created async children")
	}

	// callWorkflow branch
	es.asyncCreateCalls = 0
	es.createChildCalls = 0
	s.Workflow = &config.Workflow{Workflow: "mywf"}
	es.workflows = map[string]config.Workflow{"mywf": {Workflow: "mywf"}}
	s.execute()
	if es.createChildCalls != 1 {
		t.Error("callWorkflow should create a child")
	}

	// steps branch
	es.createChildCalls = 0
	s.Workflow = &config.Workflow{Steps: []config.Workflow{{}, {}}}
	s.Current = 0
	s.execute()
	if es.createChildCalls != 1 {
		t.Error("steps branch should create a child")
	}

	// threads branch: Current==0 triggers async children
	es.asyncCreateCalls = 0
	s.Workflow = &config.Workflow{Threads: []config.Workflow{{}, {}}}
	s.Current = 0
	s.execute()
	if !s.pending {
		t.Error("threads branch should set pending")
	}
	if es.asyncCreateCalls == 0 {
		t.Error("threads branch should launch async children")
	}

	// wait branch exercised earlier by enterWait test

	// switch branch exercised earlier

	// resume branch
	s.Workflow = &config.Workflow{Resume: "foo"}
	es.resumeReturn = true
	s.execute()
}

func TestExecute_FunctionBranch(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.Function = config.Function{Target: config.Action{System: "X", Function: "Y"}}
	s.execute()
	if !s.pending {
		t.Error("function branch should set pending")
	}
}

func TestExecute_CallDriverWithLocals(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.CallDriver = "drv.act"
	// supply local parameters
	s.Workflow.Local = []any{map[string]any{"foo": "bar"}}
	s.execute()
	if len(es.lastSentMessages) == 0 {
		t.Fatal("driver call should send message")
	}
}

func TestExecute_CallFunctionBranch(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.CallFunction = "sys.func"
	s.execute()
	if !s.pending {
		t.Error("callFunction branch should set pending")
	}
}

func TestExecute_FailedWorkflowPanics(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.Workflow = "missing"
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		s.execute()
	}()
	if !panicked {
		t.Error("expected panic when workflow not found")
	}
}

func TestLaunchAllParallelIterations_Offset(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.IterateParallel = []any{1, 2, 3, 4}
	s.Workflow.IteratePool = "2"
	s.Iteration = 1
	s.launchAllParallelIterations()
	if es.asyncCreateCalls == 0 {
		t.Error("offset launch should create async children")
	}
}

func TestLaunchParallelIteration_WithIterateAs(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.IterateParallel = []any{"val"}
	s.Workflow.IterateAs = "alias"
	s.launchParallelIteration(0)
	if es.asyncCreateCalls != 1 {
		t.Error("launchParallelIteration should create a child")
	}
	if es.lastAsyncChild == nil {
		t.Fatal("stored async child not set")
	}
	if es.lastAsyncChild.Ctx["alias"] != "val" {
		t.Errorf("alias value not set, got %v", es.lastAsyncChild.Ctx)
	}
}

func TestLaunchAllParallelIterations_PoolPanic(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.IterateParallel = []any{1}
	s.Workflow.IteratePool = "0"
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		s.launchAllParallelIterations()
	}()
	if !panicked {
		t.Error("expected panic for invalid iterate_pool")
	}
}

func TestExecute_SwitchBranchViaExecute(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.Switch = "match"
	s.Workflow.Cases = map[string]interface{}{"match": map[string]any{"Workflow": "w1"}}
	s.execute()
	if s.child == nil {
		t.Error("execute() should create child for switch branch")
	}
}

func TestExecute_ThreadsCurrentGTZero(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.Threads = []config.Workflow{{}, {}}
	s.Current = 1
	s.execute()
	if !s.pending {
		t.Error("threads branch with Current>0 should remain pending")
	}
}

func TestExecute_NoBranchDoesNothing(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	// empty workflow should not panic and not set pending
	s.execute()
	if s.pending {
		t.Error("empty workflow should not set pending")
	}
}

func TestExecute_WaitBranch(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.Wait = "2s"
	s.execute()
	if !s.pending {
		t.Error("wait branch should set pending")
	}
}

func TestExecute_IsFunctionDriverOnly(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.Function = config.Function{Driver: "drv"}
	s.execute()
	if !s.pending {
		t.Error("isFunction driver-only branch should set pending")
	}
}

func TestThreadBranch_CursorIncrement(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.Threads = []config.Workflow{{}, {}}
	s.Current = 0
	s.CurrentMsg.Labels["cursor"] = "0"
	s.execute()
	if s.CurrentMsg.Labels["cursor"] == "0" {
		t.Error("thread branch should increment cursor")
	}
}

func TestCallDriver_LocalMap(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Workflow.CallDriver = "drv.act"
	// supply single map local
	s.Workflow.Local = map[string]any{"foo": "bar"}
	s.execute()
	if len(es.lastSentMessages) == 0 {
		t.Fatal("driver call should send message")
	}
}

func TestEnterWait_WithResumeKey(t *testing.T) {
	s := makeExecuteSession()
	es := &execTestStore{}
	s.store = es
	s.Ctx["resume_key"] = "custom.key"
	s.Workflow.Wait = "1s"
	s.enterWait()
	if !s.pending {
		t.Error("enterWait should set pending")
	}
	params, _ := es.lastCallParams.(map[string]any)
	if params["key"] != "custom.key" {
		t.Errorf("expected custom key, got %v", params["key"])
	}
}
