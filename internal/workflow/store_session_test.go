// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package workflow

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

var configStr = `
---
systems:
  foo_sys:
    functions:
      bar_func:
        driver: foo1
        rawAction: bar1
workflows:
  noop: {}
  test_steps:
    steps:
      - call_function: foo_sys.bar_func
      - call_driver: foo.bar
      - call_workflow: noop
`

func TestWorkflowNoopAsChild(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{Steps: []config.Workflow{{}}},
		"msg": &dipper.Message{
			Payload: map[string]interface{}{
				"data": map[string]interface{}{
					"foo": "bar",
				},
			},
		},
		"ctx":   map[string]interface{}{},
		"steps": []map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowNoop(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{},
		"msg": &dipper.Message{
			Payload: map[string]interface{}{
				"data": map[string]interface{}{
					"foo": "bar",
				},
			},
		},
		"ctx":   map[string]interface{}{},
		"steps": []map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestCallWorkflow(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{Workflow: "noop"},
		"msg":      &dipper.Message{},
		"ctx":      map[string]interface{}{},
		"steps":    []map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestCallDriver(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{CallDriver: "foo.bar", Local: map[string]interface{}{"data": "driver test"}},
		"msg":      &dipper.Message{},
		"ctx":      map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "0",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "foo.bar",
						"resume_token": "//0",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Driver:    "foo",
						RawAction: "bar",
						Parameters: map[string]interface{}{
							"data": "driver test",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
			{
				"sessionID": "0",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "0",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestCallFunction(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{CallFunction: "foo_sys.bar_func"},
		"msg":      &dipper.Message{},
		"ctx":      map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "0",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "foo_sys.bar_func",
						"resume_token": "//0",
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
				"sessionID": "0",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "0",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowSteps(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			Steps: []config.Workflow{
				{
					CallFunction: "foo_sys.bar_func",
				},
				{
					CallDriver: "foo.bar",
				},
				{
					Workflow: "noop",
				},
			},
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "1",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "foo_sys.bar_func",
						"resume_token": "//0",
						"step_number":  int32(0),
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
					mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
						Channel: "eventbus",
						Subject: "command",
						Labels: map[string]string{
							"sessionID": "2",
						},
						Payload: map[string]interface{}{
							"ctx": map[string]interface{}{
								"_meta_desc":   "",
								"_meta_name":   "foo.bar",
								"resume_token": "//0",
								"step_number":  int32(1),
							},
							"data":  map[string]interface{}{},
							"event": map[string]interface{}{},
							"function": config.Function{
								Driver:    "foo",
								RawAction: "bar",
							},
							"labels": map[string]string{
								"sessionID": "1",
								"status":    "success",
							},
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
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowResumeWithTimeout(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			Steps: []config.Workflow{
				{
					CallFunction: "foo_sys.bar_func",
				},
				{
					Wait: "1s",
				},
				{
					Workflow: "noop",
				},
			},
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "1",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "foo_sys.bar_func",
						"resume_token": "//0",
						"step_number":  int32(0),
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
					mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
				},
				"timeout": time.Duration(3),
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowResume(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			Steps: []config.Workflow{
				{
					CallFunction: "foo_sys.bar_func",
				},
				{
					Wait: "infinite",
				},
				{
					Workflow: "noop",
				},
			},
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "1",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "foo_sys.bar_func",
						"resume_token": "//0",
						"step_number":  int32(0),
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
					mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
				},
			},
			{
				"resuming": true,
				"key":      "//0",
				"msg": &dipper.Message{
					Channel: "broadcast",
					Subject: "resume_session",
					Labels:  map[string]string{},
					Payload: map[string]interface{}{
						"key": "//0",
						"labels": map[string]interface{}{
							"status": "success",
						},
					},
				},
				"asserts": func() {
					assert.Equal(t, 2, len(store.sessions), "suspended sessions are still kept in memory")
					assert.Equal(t, 1, len(store.suspendedSessions), "mapping of key to suspended session exists")
				},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestContinueNonexistSession(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{},
		"msg":      &dipper.Message{},
		"ctx":      map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
		"steps": []map[string]interface{}{
			{
				"sessionID": "99",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "99",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
				"asserts": func() {
					mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
				},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowResumeCrash(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			Steps: []config.Workflow{
				{
					CallFunction: "foo_sys.bar_func",
				},
				{
					Wait: "infinite",
				},
				{
					Workflow: "noop",
				},
			},
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "1",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "foo_sys.bar_func",
						"resume_token": "//0",
						"step_number":  int32(0),
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
					mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
				},
			},
			{
				"resuming": true,
				"key":      "//0",
				"msg": &dipper.Message{
					Channel: "broadcast",
					Subject: "resume_session",
					Labels:  map[string]string{},
					Payload: map[string]interface{}{
						"key": "//0",
						"labels": map[string]interface{}{
							"status":   "success",
							"somedata": []interface{}{"wrong", "format"},
						},
					},
				},
				"asserts": func() {
					assert.Equal(t, 2, len(store.sessions), "suspended sessions are still kept in memory")
					assert.Equal(t, 1, len(store.suspendedSessions), "mapping of key to suspended session exists")
				},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowIterate(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			CallFunction: "foo_sys.bar_func",
			Iterate: []string{
				"item1",
				"item2",
				"item3",
			},
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "0",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "foo_sys.bar_func",
						"resume_token": "//0",
						"current":      "item1",
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
				"sessionID": "0",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "0",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
				"asserts": func() {
					mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
						Channel: "eventbus",
						Subject: "command",
						Labels: map[string]string{
							"sessionID": "0",
						},
						Payload: map[string]interface{}{
							"ctx": map[string]interface{}{
								"_meta_desc":   "",
								"_meta_name":   "foo_sys.bar_func",
								"resume_token": "//0",
								"current":      "item2",
							},
							"data":  map[string]interface{}{},
							"event": map[string]interface{}{},
							"function": config.Function{
								Target: config.Action{
									System:   "foo_sys",
									Function: "bar_func",
								},
							},
							"labels": map[string]string{
								"sessionID": "0",
								"status":    "success",
							},
						},
					})).Times(1)
				},
			},
			{
				"sessionID": "0",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "0",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
				"asserts": func() {
					mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
						Channel: "eventbus",
						Subject: "command",
						Labels: map[string]string{
							"sessionID": "0",
						},
						Payload: map[string]interface{}{
							"ctx": map[string]interface{}{
								"_meta_desc":   "",
								"_meta_name":   "foo_sys.bar_func",
								"resume_token": "//0",
								"current":      "item3",
							},
							"data":  map[string]interface{}{},
							"event": map[string]interface{}{},
							"function": config.Function{
								Target: config.Action{
									System:   "foo_sys",
									Function: "bar_func",
								},
							},
							"labels": map[string]string{
								"sessionID": "0",
								"status":    "success",
							},
						},
					})).Times(1)
				},
			},
			{
				"sessionID": "0",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "0",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}
