// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package workflow

import (
	"context"
	"testing"

	cfg "github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/jellydator/ttlcache/v3"
	"github.com/op/go-logging"
	"golang.org/x/exp/slices"
)

// customStore is a tiny Store implementation used by this test file.
type customStore struct {
	cfg    *cfg.Config
	logger *logging.Logger
}

func (s *customStore) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (s *customStore) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	return nil
}

func (s *customStore) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (s *customStore) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}
func (s *customStore) GetName() string { return _testModule }

func (s *customStore) RunAsync(task func()) {
	go task()
}

func (s *customStore) CallWithMessage(msg *dipper.Message) ([]byte, error) { return nil, nil }
func (s *customStore) CallWithMessageNoWait(msg *dipper.Message) error     { return nil }

func (s *customStore) GetConfig() *cfg.Config {
	if s.cfg == nil {
		return &cfg.Config{DataSet: &cfg.DataSet{Contexts: map[string]interface{}{}}}
	}

	return s.cfg
}
func (s *customStore) SendMessage(msg *dipper.Message) {}
func (s *customStore) CreateChildSession(parent *Session, wf *cfg.Workflow, msg *dipper.Message) *Session {
	return nil
}

func (s *customStore) CreateAsyncChildSession(parent *Session, wf *cfg.Workflow, msg *dipper.Message) *Session {
	return nil
}
func (s *customStore) ActivateSession(w *Session) {}
func (s *customStore) EmitResult(w *Session)      {}
func (s *customStore) StartSession(wf *cfg.Workflow, msg *dipper.Message, ctx map[string]interface{}) {
}
func (s *customStore) StartDynamicSession(spec *dipper.Message, ctx map[string]interface{}) {}
func (s *customStore) ContinueSession(ID string, msg *dipper.Message, child *Session) {
}
func (s *customStore) ResumeSession(key string, msg *dipper.Message) bool { return false }
func (s *customStore) GetNumSessions(getAll bool) int                     { return 0 }
func (s *customStore) DumpSessions(_ int, _ string) []byte                { return nil }
func (s *customStore) Wait()                                              {}
func (s *customStore) GetLogger() *logging.Logger {
	if s.logger == nil {
		return dipper.GetLogger(_testModule, "ERROR")
	}

	return s.logger
}
func (s *customStore) Stop() {}
func (s *customStore) GetCache() *ttlcache.Cache[string, map[string]any] {
	return nil
}

// helper for building minimal session via NewSession.
func makeSession() *Session {
	return NewSession("id", &cfg.Workflow{}, &customStore{})
}

func TestNewSession_Basic(t *testing.T) {
	wf := &cfg.Workflow{Description: "desc"}
	s := NewSession("123", wf, &customStore{})
	if s.ID != "123" {
		t.Errorf("expected id 123, got %s", s.ID)
	}
	if s.Workflow == wf {
		t.Error("NewSession should create a copy of workflow")
	}
	if s.Ctx["_meta_desc"] != "desc" {
		t.Errorf("_meta_desc not initialized")
	}
	if len(*s.Performing) != 1 || (*s.Performing)[0] != "initializing" {
		t.Error("performing slice not initialized")
	}
	if s.threads == nil {
		t.Error("threads should not be nil")
	}
}

func TestNewSession_HookFlag(t *testing.T) {
	wf := &cfg.Workflow{Context: SessionContextHooks}
	s := NewSession("id", wf, &customStore{})
	if !s.IsHook {
		t.Error("expected IsHook true for hook context")
	}
}

func TestNewSession_PreservesOriginalWorkflow(t *testing.T) {
	wf := &cfg.Workflow{
		Name:        "{{.ctx.name}}",
		Description: "original desc",
		Steps: []cfg.Workflow{{
			Description: "child",
		}},
	}
	s := NewSession("id", wf, &customStore{})

	if s.OriginalWorkflow == nil {
		t.Fatal("expected OriginalWorkflow to be preserved")
	}
	if s.OriginalWorkflow == wf {
		t.Fatal("expected OriginalWorkflow to be a deep copy")
	}
	if s.OriginalWorkflow.Steps[0].Description != "child" {
		t.Fatalf("unexpected preserved workflow child description: %+v", s.OriginalWorkflow.Steps)
	}

	s.Workflow.Steps[0].Description = "mutated"
	if s.OriginalWorkflow.Steps[0].Description != "child" {
		t.Fatal("mutating runtime workflow should not affect preserved workflow")
	}
	if wf.Steps[0].Description != "child" {
		t.Fatal("mutating runtime workflow should not affect original input workflow")
	}
}

func TestInherentParentData(t *testing.T) {
	parent := makeSession()
	parent.Event = map[string]interface{}{"foo": "bar"}
	parent.EventID = "ev1"
	parent.Ctx = map[string]interface{}{"a": 1, "hooks": "bad"}
	parent.Performing = &[]string{"p"}
	parent.LoadedContexts = []string{"ctx1"}
	parent.depth = 2
	parent.IsHook = true

	child := makeSession()
	child.IsHook = false
	child.inherentParentData(parent)

	//nolint:goconst
	if child.EventID != "ev1" || child.Event["foo"] != "bar" {
		t.Error("event not copied")
	}
	if child.Ctx["a"] != 1 {
		t.Error("ctx not deep copied")
	}
	if _, ok := child.Ctx["hooks"]; ok {
		t.Error("hooks should be dropped")
	}
	if len(*child.Performing) != 2 || (*child.Performing)[1] != "initializing" {
		t.Error("performing not appended")
	}
	if child.depth != 3 {
		t.Error("depth not incremented")
	}
	if !child.IsHook {
		t.Error("IsHook should propagate from parent")
	}
	if child.parent != parent {
		t.Error("parent pointer not set")
	}
}

func TestInjectMsg_DataAndEventID(t *testing.T) {
	s := makeSession()
	msg := &dipper.Message{Labels: map[string]string{"eventID": "e1"}, Payload: map[string]interface{}{"data": map[string]interface{}{"k": "v"}}}
	s.injectMsg(msg)
	if s.CurrentMsg != msg {
		t.Fatal("current message not assigned")
	}
	if msg.Labels["sessionID"] != s.ID {
		t.Errorf("sessionID label not set")
	}
	if s.EventID != "e1" {
		t.Error("eventID not set")
	}
	if s.Event["k"] != "v" {
		t.Error("event data not extracted")
	}
}

func TestInjectMsg_NoData(t *testing.T) {
	s := makeSession()
	msg := &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"other": 1}}
	s.injectMsg(msg)
	if s.Event == nil {
		t.Error("Event should be initialized to map")
	}
}

func TestInjectMsg_ExistingEvent(t *testing.T) {
	s := makeSession()
	s.Event = map[string]interface{}{"foo": "bar"}
	msg := &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{}}
	s.injectMsg(msg)
	if s.Event["foo"] != "bar" {
		t.Error("existing event should not be overwritten")
	}
}

func TestInjectNamedCTX_PanicWhenMissing(t *testing.T) {
	s := makeSession()
	s.store = &customStore{cfg: &cfg.Config{DataSet: &cfg.DataSet{Contexts: map[string]interface{}{}}}}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing named context")
		}
	}()
	s.injectNamedCTX("missing", true)
}

func TestInjectNamedCTX_NilValue(t *testing.T) {
	s := makeSession()
	s.store = &customStore{cfg: &cfg.Config{DataSet: &cfg.DataSet{Contexts: map[string]interface{}{"foo": nil}}}}
	s.injectNamedCTX("foo", true) // should not panic nor modify
}

func TestInjectNamedCTX_GlobalAndEventsAndName(t *testing.T) {
	name := "ctx1"
	s := makeSession()
	s.Workflow.Name = "mywf"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{"cursor": "0"}}
	data := map[string]interface{}{
		"*":       map[string]interface{}{"a": 1},
		"_events": map[string]interface{}{"b": 2},
		"mywf":    map[string]interface{}{"c": 3},
	}
	s.store = &customStore{cfg: &cfg.Config{DataSet: &cfg.DataSet{Contexts: map[string]interface{}{name: data}}}}

	// first call should add *, _events and named
	s.injectNamedCTX(name, true)
	if s.Ctx["a"] != 1 || s.Ctx["b"] != 2 || s.Ctx["c"] != 3 {
		t.Error("named context values not merged")
	}
}

func TestInjectCTXs_ContextAndListAndPanic(t *testing.T) {
	s := makeSession()
	s.CurrentMsg = newMsg()
	store := &customStore{cfg: &cfg.Config{DataSet: &cfg.DataSet{Contexts: map[string]interface{}{}}}}
	s.store = store

	// case 1: single context name
	s.LoadedContexts = []string{}
	s.Workflow.Context = "foo"
	store.cfg.DataSet.Contexts = map[string]interface{}{"foo": nil}
	s.injectCTXs()
	if !slices.Contains(s.LoadedContexts, "foo") {
		t.Fatal("expected foo added to loaded contexts")
	}

	// case 2: list handling nil, empty, duplicates
	s.LoadedContexts = []string{}
	s.Workflow.Context = ""
	s.Workflow.Contexts = []interface{}{nil, "", "bar", "bar"}
	store.cfg.DataSet.Contexts = map[string]interface{}{"bar": nil}
	s.injectCTXs()
	if len(s.LoadedContexts) != 1 || s.LoadedContexts[0] != "bar" {
		t.Fatalf("unexpected loaded contexts %v", s.LoadedContexts)
	}

	// case 3: panic on non-string
	s.Workflow.Contexts = []interface{}{123}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-string context")
		}
	}()
	s.injectCTXs()
}

func TestInjectEventCTX(t *testing.T) {
	s := makeSession()
	s.Ctx = map[string]interface{}{"x": 1}
	s.injectEventCTX(map[string]interface{}{"y": 2})
	if s.Ctx["y"] != 2 {
		t.Error("event ctx not merged")
	}
	// nil case
	s.injectEventCTX(nil)
}

func TestInjectRerunCTX(t *testing.T) {
	s := makeSession()
	s.Ctx = map[string]interface{}{"x": 1}
	s.injectRerunCTX(map[string]interface{}{"y": 2})
	if s.Ctx["y"] != 2 {
		t.Error("rerun ctx not copied")
	}
	// nil case
	s.injectRerunCTX(nil)
}

func TestInjectRerunCTX_DoesNotApplyMergeModifiers(t *testing.T) {
	s := makeSession()
	s.Ctx = map[string]interface{}{"keep": "value"}
	s.injectRerunCTX(map[string]interface{}{"name*": "override", "labels+": []interface{}{"a"}})

	if s.Ctx["name*"] != "override" {
		t.Fatalf("expected literal rerun ctx key name* to be preserved, got %+v", s.Ctx)
	}
	if _, ok := s.Ctx["name"]; ok {
		t.Fatalf("did not expect merge modifier expansion for rerun ctx, got %+v", s.Ctx)
	}
	if _, ok := s.Ctx["labels+"]; !ok {
		t.Fatalf("expected literal rerun ctx key labels+ to be preserved, got %+v", s.Ctx)
	}
}

func TestInitCTX_BasicAndHook(t *testing.T) {
	s := makeSession()
	// provide context foo so that initCTX will not panic
	s.store = &customStore{cfg: &cfg.Config{DataSet: &cfg.DataSet{Contexts: map[string]interface{}{"foo": nil}}}}
	s.LoadedContexts = []string{"foo"}
	s.Workflow.Name = ""
	s.IsHook = true
	s.CurrentMsg = newMsg()
	s.initCTX(map[string]interface{}{"e": 1}, map[string]interface{}{"r": 2})
	if _, ok := s.Ctx["e"]; !ok {
		t.Error("event context not injected")
	}
	if _, ok := s.Ctx["r"]; !ok {
		t.Error("rerun context not injected")
	}
	if _, ok := s.Ctx["hooks"]; ok {
		t.Error("hooks should be removed for hook")
	}
}

func TestInjectLocalCTX(t *testing.T) {
	s := makeSession()
	s.CurrentMsg = newMsg()
	s.Ctx = map[string]interface{}{"x": 1}
	s.Workflow.Local = map[string]interface{}{"foo": "bar"}
	s.injectLocalCTX()
	if s.Ctx["foo"] != "bar" {
		t.Error("local ctx not merged")
	}
	// slice case
	s.Workflow.Local = []interface{}{map[string]interface{}{"a": "b"}}
	s.injectLocalCTX()
	if s.Ctx["a"] != "b" {
		t.Error("local ctx not merged from slice")
	}
	// skip when there is callDriver
	s.Workflow.CallDriver = "drv"
	s.Ctx = map[string]interface{}{}
	s.Workflow.Local = map[string]interface{}{"foo": "baz"}
	s.injectLocalCTX()
	if _, ok := s.Ctx["foo"]; ok {
		t.Error("should not merge when callDriver is set")
	}
}

func TestInterpolateWorkflow(t *testing.T) {
	s := makeSession()
	s.Ctx = map[string]interface{}{"name": "n", "desc": "d", "ifval": "i", "any": "a", "unless": "u", "uall": "ua", "match": "m", "umatch": "um", "retry": "r", "backoff": "b", "wait": "w", "callFunc": "cf", "callDrv": "cd", "resume": "rs", "pool": "p", "evt": "sys.trigger"}
	wf := &cfg.Workflow{
		Name:            "{{.ctx.name}}",
		Description:     "{{.ctx.desc}}",
		If:              []string{"{{.ctx.ifval}}"},
		IfAny:           []string{"{{.ctx.any}}"},
		Unless:          []string{"{{.ctx.unless}}"},
		UnlessAll:       []string{"{{.ctx.uall}}"},
		Match:           map[string]interface{}{"key": "{{.ctx.match}}"},
		UnlessMatch:     map[string]interface{}{"key": "{{.ctx.umatch}}"},
		Retry:           "{{.ctx.retry}}",
		Backoff:         "{{.ctx.backoff}}",
		Wait:            "{{.ctx.wait}}",
		CallFunction:    "{{.ctx.callFunc}}",
		CallDriver:      "{{.ctx.callDrv}}",
		SendEvent:       map[string]interface{}{"events": []interface{}{"{{.ctx.evt}}"}},
		Resume:          "{{.ctx.resume}}",
		Iterate:         []interface{}{1},
		IterateParallel: []interface{}{2},
		IteratePool:     "{{.ctx.pool}}",
	}
	s.Workflow = wf
	s.OriginalWorkflow = cloneWorkflow(wf)
	s.CurrentMsg = newMsg()
	s.interpolateWorkflow()
	if s.OriginalWorkflow.Name != "{{.ctx.name}}" {
		t.Error("preserved workflow should remain uninterpolated")
	}
	if s.Workflow.Name != "n" {
		t.Error("name not interpolated")
	}
	if s.Workflow.Description != "{{.ctx.desc}}" {
		t.Error("iterate-parallel description should remain deferred for child interpolation")
	}
	if s.Workflow.If[0] != "i" || s.Workflow.IfAny[0] != "a" {
		t.Error("If/IfAny not interpolated")
	}
	if s.Workflow.CallFunction != "cf" || s.Workflow.CallDriver != "cd" {
		t.Error("calls not interpolated")
	}
	msg, ok := s.Workflow.SendEvent.(map[string]interface{})
	if !ok {
		t.Fatalf("send_event not interpolated as map: %T", s.Workflow.SendEvent)
	}
	events, ok := msg["events"].([]interface{})
	if !ok || len(events) != 1 || events[0] != "sys.trigger" {
		t.Fatalf("send_event events not interpolated: %+v", msg)
	}
	if arr, ok := s.Workflow.Iterate.([]interface{}); !ok || len(arr) == 0 {
		t.Error("iterate fields lost")
	}
	if arr2, ok := s.Workflow.IterateParallel.([]interface{}); !ok || len(arr2) == 0 {
		t.Error("iterate-parallel fields lost")
	}
}

func TestInherentParentSettings(t *testing.T) {
	p := makeSession()
	p.ID = "parent"
	p.Workflow = &cfg.Workflow{OnError: "e", OnFailure: "f"}
	s := makeSession()
	s.Workflow = &cfg.Workflow{}
	s.inherentParentSettings(p)
	if s.ID != "parent" {
		t.Error("ID not copied")
	}
	if s.Workflow.OnError != "" {
		t.Error("OnError should not be inherited from parent")
	}
	if s.Workflow.OnFailure != "" {
		t.Error("OnFailure should not be inherited from parent")
	}

	// even when child already has values, they should remain unchanged
	s.Workflow.OnError = "child-error"
	s.Workflow.OnFailure = "child-failure"
	s.inherentParentSettings(p)
	if s.Workflow.OnError != "child-error" {
		t.Error("existing OnError should not be overwritten")
	}
	if s.Workflow.OnFailure != "child-failure" {
		t.Error("existing OnFailure should not be overwritten")
	}
}

func TestInit_NoParent(t *testing.T) {
	s := makeSession()
	msg := &dipper.Message{Labels: map[string]string{"cursor": "0"}, Payload: map[string]interface{}{}}
	s.Init(msg, nil, map[string]interface{}{"foo": "bar"}, nil, nil)
	if s.Event == nil {
		t.Error("Event should be initialized")
	}
}

func TestInit_WithParent(t *testing.T) {
	parent := makeSession()
	parent.context = context.Background()
	parent.cancelFunc = func() {}
	msg := &dipper.Message{Labels: map[string]string{"cursor": "0"}, Payload: map[string]interface{}{}}
	s := makeSession()
	s.Init(msg, parent, nil, nil, nil)
	if s.parent != parent {
		t.Error("parent pointer not set in Init")
	}
	if s.context == nil {
		t.Error("context should be set")
	}
}

func TestInit_WithLoadedContexts(t *testing.T) {
	s := makeSession()
	msg := &dipper.Message{Labels: map[string]string{"cursor": "0"}, Payload: map[string]interface{}{}}
	loaded := []string{"_preloaded_ctx"}

	s.Init(msg, nil, nil, nil, loaded)

	if len(s.LoadedContexts) != 1 || s.LoadedContexts[0] != "_preloaded_ctx" {
		t.Fatalf("expected preloaded contexts to be copied, got %+v", s.LoadedContexts)
	}

	loaded[0] = "mutated"
	if s.LoadedContexts[0] != "_preloaded_ctx" {
		t.Fatalf("expected loaded context copy to be independent, got %+v", s.LoadedContexts)
	}
}
