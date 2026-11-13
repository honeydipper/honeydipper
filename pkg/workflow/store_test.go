//go:build !integration
// +build !integration

package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const foo = "foo"

// dummyHelper implements just enough of StoreHelper for NewStore tests.
// We keep methods no-op since NewStore doesn't actually call them.
type dummyHelper struct{}

func (d *dummyHelper) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (d *dummyHelper) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	return nil
}

func (d *dummyHelper) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (d *dummyHelper) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}
func (d *dummyHelper) GetName() string                                     { return "" }
func (d *dummyHelper) CallWithMessage(msg *dipper.Message) ([]byte, error) { return nil, nil }
func (d *dummyHelper) CallWithMessageNoWait(msg *dipper.Message) error     { return nil }
func (d *dummyHelper) GetConfig() *config.Config                           { return &config.Config{DataSet: &config.DataSet{}} }
func (d *dummyHelper) SendMessage(msg *dipper.Message)                     {}

// sequenceCaller allows returning a pre-cooked sequence of results/errors.  Each
// invocation of Call advances the index.  This is handy for exercising the
// Wait() function with multiple logical RPC interactions.
type sequenceCaller struct {
	results [][]byte
	errs    []error
	idx     int
}

func (s *sequenceCaller) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	if s.idx >= len(s.results) {
		return nil, nil
	}
	res := s.results[s.idx]
	var err error
	if s.idx < len(s.errs) {
		err = s.errs[s.idx]
	}
	s.idx++

	return res, err
}

func (s *sequenceCaller) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	return nil
}

func (s *sequenceCaller) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return nil, nil
}

func (s *sequenceCaller) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}

func (s *sequenceCaller) GetName() string {
	return "caller"
}

//----- tests for utility functions --------------------------------------------------

func TestNewStore_Basics(t *testing.T) {
	helper := &dummyHelper{}
	st := NewStore(helper)

	ps, ok := st.(*PersistedStore)
	if !ok {
		t.Fatalf("expected PersistedStore, got %T", st)
	}
	if ps.StoreHelper != helper {
		t.Error("store helper not set")
	}
	if ps.Logger == nil {
		t.Error("logger should be initialized")
	}
	if ps.storeID == "" {
		t.Error("storeID should be populated")
	}
}

func TestGetStoredSession_HappyPath(t *testing.T) {
	w := &Session{ID: foo, State: SessionStateDone}
	data := w.Marshal()
	call := &sequenceCaller{
		results: [][]byte{data},
		errs:    []error{nil},
	}
	sess := GetStoredSession(call, foo)
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.ID != foo {
		t.Errorf("unexpected ID %s", sess.ID)
	}
}

// ----- Wait() scenarios -----------------------------------------------------------

func TestWait_SessionNotFoundPanic(t *testing.T) {
	// first call returns a session ID; second call returns nil which causes
	// GetStoredSession to return nil and Wait to panic.
	caller := &sequenceCaller{
		results: [][]byte{[]byte("sid"), nil},
		errs:    []error{nil, nil},
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when session not found")
		}
	}()
	Wait(context.Background(), caller, "evt", "key")
}

func TestWait_ImmediateDone(t *testing.T) {
	w := &Session{ID: "sid", State: SessionStateDone}
	caller := &sequenceCaller{
		results: [][]byte{
			[]byte("sid"), // load
			w.Marshal(),   // lrange -> session
		},
		errs: []error{nil, nil},
	}
	res := Wait(context.Background(), caller, "evt", "key")
	if res == nil || res.State != SessionStateDone {
		t.Errorf("unexpected result %v", res)
	}
}

func TestWait_ExpectErrorPanic(t *testing.T) {
	// first iteration has non-done session, expect returns error -> panic
	w := &Session{ID: "sid", State: SessionStateAction}
	caller := &sequenceCaller{
		results: [][]byte{
			[]byte("sid"),
			w.Marshal(),
			nil, // expect response
		},
		errs: []error{
			nil,
			nil,
			errors.New("bad"),
		},
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on expect error")
		}
	}()
	Wait(context.Background(), caller, "evt", "key")
}

func TestWait_TimeoutThenDone(t *testing.T) {
	w1 := &Session{ID: "sid", State: SessionStateAction}
	w2 := &Session{ID: "sid", State: SessionStateDone}
	caller := &sequenceCaller{
		results: [][]byte{
			[]byte("sid"),
			w1.Marshal(),
			nil, // expect
			[]byte("sid"),
			w2.Marshal(),
		},
		errs: []error{
			nil,
			nil,
			dipper.ErrTimeout,
			nil,
			nil,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	res := Wait(ctx, caller, "evt", "key")
	if res == nil || res.State != SessionStateDone {
		t.Errorf("expected done, got %v", res)
	}
}

func TestWait_ContextCancelled(t *testing.T) {
	w := &Session{ID: "sid", State: SessionStateAction}
	caller := &sequenceCaller{
		results: [][]byte{[]byte("sid"), w.Marshal(), nil},
		errs:    []error{nil, nil, nil},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic due to cancelled context")
		}
	}()
	Wait(ctx, caller, "evt", "key")
}

func TestWait_NoErrorLoop(t *testing.T) {
	// simulate one normal loop where expect returns no error and the session
	// becomes done on second iteration.
	w1 := &Session{ID: "sid", State: SessionStateAction}
	w2 := &Session{ID: "sid", State: SessionStateDone}
	caller := &sequenceCaller{
		results: [][]byte{
			[]byte("sid"),
			w1.Marshal(),
			nil,
			[]byte("sid"),
			w2.Marshal(),
		},
		errs: []error{nil, nil, nil, nil, nil},
	}
	ctx := context.Background()
	res := Wait(ctx, caller, "evt", "key")
	if res == nil || res.State != SessionStateDone {
		t.Errorf("expected done, got %v", res)
	}
}

// test that GetStoredSession returns nil when the RPC returns an empty byte
// slice.  This exercise exercises the helper logic we added for safety.
func TestGetStoredSession_EmptySlice(t *testing.T) {
	call := &sequenceCaller{results: [][]byte{{}}}
	sess := GetStoredSession(call, "whatever")
	if sess != nil {
		t.Error("expected nil when rpc returns empty slice")
	}
}
