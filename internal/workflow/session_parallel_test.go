// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package workflow

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/internal/daemon"
	"github.com/honeydipper/honeydipper/v3/internal/workflow/mock_workflow"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

type DipperMsgMatcher struct {
	val         interface{}
	description string
}

func (e *DipperMsgMatcher) Matches(x interface{}) bool {
	// for thread operations, ignore the sessionID and resume_token

	m := x.(*dipper.Message)
	msg := *m
	msg.Labels = dipper.MustDeepCopy(m.Labels).(map[string]string)
	delete(msg.Labels, "sessionID")

	if c, ok := msg.Payload.(map[string]interface{})["ctx"]; ok {
		c = dipper.MustDeepCopy(c)
		delete(c.(map[string]interface{}), "resume_token")
		msg.Payload.(map[string]interface{})["ctx"] = c
	}

	return reflect.DeepEqual(x, e.val)
}

func (e *DipperMsgMatcher) String() string {
	return e.description
}

func DipperMsgEq(x interface{}) gomock.Matcher {
	return &DipperMsgMatcher{
		val:         x,
		description: fmt.Sprintf("%v", x),
	}
}

func TestWorkflowIterateParallel(t *testing.T) {
	syntheticTest(t, configStr, map[string]interface{}{
		"workflow": &config.Workflow{
			CallFunction: "foo_sys.bar_func",
			IterateParallel: []string{
				"item1",
				"item2",
				"item3",
			},
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().AnyTimes().Return("")
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc": "",
						"_meta_name": "foo_sys.bar_func",
						"current":    "item1",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc": "",
						"_meta_name": "foo_sys.bar_func",
						"current":    "item2",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc": "",
						"_meta_name": "foo_sys.bar_func",
						"current":    "item3",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
			{
				"sessionID": "1",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "1",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
			{
				"sessionID": "2",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "2",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
			{
				"sessionID": "3",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "3",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	})
}

func TestWorkflowIterateParallelPool(t *testing.T) {
	syntheticTest(t, configStr, map[string]interface{}{
		"workflow": &config.Workflow{
			CallFunction: "foo_sys.bar_func",
			IterateParallel: []string{
				"item1",
				"item2",
				"item3",
			},
			IteratePool: "2",
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().AnyTimes().Return("")
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc": "",
						"_meta_name": "foo_sys.bar_func",
						"current":    "item1",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc": "",
						"_meta_name": "foo_sys.bar_func",
						"current":    "item2",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
			{
				"sessionID": "1",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "1",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
				"asserts": func() {
					mockHelper.EXPECT().GetDaemonID().AnyTimes().Return("")
					mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
						Channel: "eventbus",
						Subject: "command",
						Labels:  map[string]string{}, Payload: map[string]interface{}{
							"ctx": map[string]interface{}{
								"_meta_desc": "",
								"_meta_name": "foo_sys.bar_func",
								"current":    "item3",
							},
							"data":  map[string]interface{}{},
							"event": map[string]interface{}{},
							"function": config.Function{
								Target: config.Action{
									System:   "foo_sys",
									Function: "bar_func",
								},
							},
							"labels": emptyLabels,
						},
					})).Times(1)
				},
			},
			{
				"sessionID": "2",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "2",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
			{
				"sessionID": "3",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "3",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	})
}

func TestWorkflowThreads(t *testing.T) {
	syntheticTest(t, configStr, map[string]interface{}{
		"workflow": &config.Workflow{
			Threads: []config.Workflow{
				{
					CallFunction: "foo_sys.bar_func",
					Local:        map[string]interface{}{"item": 1},
				},
				{
					CallFunction: "foo_sys.bar_func",
					Local:        map[string]interface{}{"item": 2},
				},
				{
					CallFunction: "foo_sys.bar_func",
					Local:        map[string]interface{}{"item": 3},
				},
			},
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().AnyTimes().Return("")
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":    "",
						"_meta_name":    "foo_sys.bar_func",
						"item":          1,
						"thread_number": 0,
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":    "",
						"_meta_name":    "foo_sys.bar_func",
						"item":          2,
						"thread_number": 1,
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":    "",
						"_meta_name":    "foo_sys.bar_func",
						"item":          3,
						"thread_number": 2,
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
			{
				"sessionID": "1",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "1",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
			{
				"sessionID": "2",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "2",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
			{
				"sessionID": "3",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "3",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	})
}

func TestExecuteThreadsWaitsForContextSnapshotLock(t *testing.T) {
	ctrl := gomock.NewController(t)
	helper := mock_workflow.NewMockSessionStoreHelper(ctrl)
	testStore := NewSessionStore(helper)
	defer delete(dipper.IDMapMetadata, &testStore.sessions)

	helper.EXPECT().GetConfig().AnyTimes().Return(&config.Config{DataSet: &config.DataSet{}})
	helper.EXPECT().GetDaemonID().AnyTimes().Return("")
	launched := make(chan struct{}, 2)
	helper.EXPECT().SendMessage(gomock.Any()).Do(func(*dipper.Message) {
		launched <- struct{}{}
	}).Times(2)

	parent := testStore.newSession("", "", &config.Workflow{
		Threads: []config.Workflow{
			{CallFunction: "foo_sys.bar_func"},
			{CallFunction: "foo_sys.bar_func"},
		},
	}).(*Session)
	parent.ID = "parent"
	parent.ctx = map[string]interface{}{"shared": "value"}
	parent.event = map[string]interface{}{}
	msg := &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{}}

	parent.ctxLock.Lock()
	locked := true
	defer func() {
		if locked {
			parent.ctxLock.Unlock()
		}
	}()

	done := make(chan struct{})
	go func() {
		parent.executeThreads(msg)
		close(done)
	}()

	select {
	case <-launched:
		t.Fatal("child launched before the parent context snapshot lock was available")
	case <-done:
		t.Fatal("thread preparation completed without acquiring the parent context snapshot lock")
	case <-time.After(100 * time.Millisecond):
	}

	parent.ctxLock.Unlock()
	locked = false

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("thread preparation did not complete after releasing the context snapshot lock")
	}

	daemon.Children.Wait()
	for i := 0; i < 2; i++ {
		select {
		case <-launched:
		case <-time.After(time.Second):
			t.Fatal("prepared child did not execute")
		}
	}
}
