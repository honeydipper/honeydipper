// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package workflow

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

var configStrHook = `
---
workflows:
  send_chat:
    call_driver: foo.bar
`

func TestOnSessionHook(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{},
		"msg":      &dipper.Message{},
		"ctx": map[string]interface{}{
			"hooks": map[string]interface{}{
				"on_session": "send_chat",
			},
		},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().AnyTimes().Return("")
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "2",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "",
						"resume_token": "//1",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Driver:    "foo",
						RawAction: "bar",
					},
					"labels": emptyLabels,
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
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
				"asserts": func() {
					mockHelper.EXPECT().GetDaemonID().AnyTimes().Return("")
				},
			},
		},
	}

	syntheticTest(t, configStrHook, testcase)
}

func TestOnFirstActionHook(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{Steps: []config.Workflow{}},
		"msg":      &dipper.Message{},
		"ctx": map[string]interface{}{
			"hooks": map[string]interface{}{
				"on_first_action": "send_chat",
			},
		},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().Return("")
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "2",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "",
						"resume_token": "//0",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Driver:    "foo",
						RawAction: "bar",
					},
					"labels": emptyLabels,
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
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

	syntheticTest(t, configStrHook, testcase)
}

func TestSkipHookWithFalseCondition(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{If: []string{"false"}},
		"msg":      &dipper.Message{},
		"ctx": map[string]interface{}{
			"hooks": map[string]interface{}{
				"on_first_action": "send_chat",
			},
		},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
		"steps": []map[string]interface{}{},
	}
	syntheticTest(t, configStrHook, testcase)
}

func TestOnExitHook(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{},
		"msg":      &dipper.Message{},
		"ctx": map[string]interface{}{
			"hooks": map[string]interface{}{
				"on_exit": "send_chat",
			},
		},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().Return("")
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "2",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "",
						"resume_token": "//0",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Driver:    "foo",
						RawAction: "bar",
					},
					"labels": map[string]string{
						"status": "success",
					},
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
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
	syntheticTest(t, configStrHook, testcase)
}

func TestOnSuccessHook(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{},
		"msg":      &dipper.Message{},
		"ctx": map[string]interface{}{
			"hooks": map[string]interface{}{
				"on_success": "send_chat",
			},
		},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().Return("")
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "2",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "",
						"resume_token": "//0",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Driver:    "foo",
						RawAction: "bar",
					},
					"labels": map[string]string{
						"status": "success",
					},
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
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
	syntheticTest(t, configStrHook, testcase)
}

func TestOnFailureHook(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{CallDriver: "foo.failure"},
		"msg":      &dipper.Message{},
		"ctx": map[string]interface{}{
			"hooks": map[string]interface{}{
				"on_failure": []interface{}{
					"send_chat",
				},
			},
		},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().Return("")
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "0",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "foo.failure",
						"resume_token": "//0",
						"hooks": map[string]interface{}{
							"on_failure": []interface{}{
								"send_chat",
							},
						},
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Driver:    "foo",
						RawAction: "failure",
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
						"status":    "failure",
						"reason":    "testing hook on failure",
					},
				},
				"ctx": map[string]interface{}{},
				"asserts": func() {
					mockHelper.EXPECT().GetDaemonID().Return("")
					mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
						Channel: "eventbus",
						Subject: "command",
						Labels: map[string]string{
							"sessionID": "3",
						},
						Payload: map[string]interface{}{
							"ctx": map[string]interface{}{
								"_meta_desc":    "",
								"_meta_name":    "foo.failure",
								"resume_token":  "//2",
								"thread_number": 0,
							},
							"data":  map[string]interface{}{},
							"event": map[string]interface{}{},
							"function": config.Function{
								Driver:    "foo",
								RawAction: "bar",
							},
							"labels": map[string]string{
								"sessionID":  "0",
								"status":     "failure",
								"reason":     "testing hook on failure",
								"performing": "driver foo.failure",
							},
						},
					})).Times(1)
				},
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
	}
	syntheticTest(t, configStrHook, testcase)
}

func TestOnErrorHook(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{CallDriver: "foo.error"},
		"msg":      &dipper.Message{},
		"ctx": map[string]interface{}{
			"hooks": map[string]interface{}{
				"on_failure": "send_chat_failure",
				"on_error":   "send_chat",
				"on_success": []interface{}{
					"send_chat_success",
				},
			},
		},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().Return("")
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "0",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "foo.error",
						"resume_token": "//0",
						"hooks": map[string]interface{}{
							"on_failure": "send_chat_failure",
							"on_error":   "send_chat",
							"on_success": []interface{}{
								"send_chat_success",
							},
						},
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Driver:    "foo",
						RawAction: "error",
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
						"status":    "error",
						"reason":    "testing hook on error",
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
								"_meta_name":   "foo.error",
								"resume_token": "//0",
							},
							"data":  map[string]interface{}{},
							"event": map[string]interface{}{},
							"function": config.Function{
								Driver:    "foo",
								RawAction: "bar",
							},
							"labels": map[string]string{
								"sessionID":  "0",
								"status":     "error",
								"reason":     "testing hook on error",
								"performing": "driver foo.error",
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
	syntheticTest(t, configStrHook, testcase)
}

func TestFailureInHook(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{Steps: []config.Workflow{}},
		"msg":      &dipper.Message{},
		"ctx": map[string]interface{}{
			"hooks": map[string]interface{}{
				"on_first_action": "send_chat",
			},
		},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().Return("")
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "2",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "",
						"resume_token": "//0",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Driver:    "foo",
						RawAction: "bar",
					},
					"labels": emptyLabels,
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
			{
				"sessionID": "2",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "2",
						"status":    "failure",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	}
	syntheticTest(t, configStrHook, testcase)
}

func TestErrorInCompletionHook(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{},
		"msg":      &dipper.Message{},
		"ctx": map[string]interface{}{
			"hooks": map[string]interface{}{
				"on_success": "send_chat",
			},
		},
		"asserts": func() {
			mockHelper.EXPECT().GetDaemonID().Return("")
			mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels: map[string]string{
					"sessionID": "2",
				},
				Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":   "",
						"_meta_name":   "",
						"resume_token": "//0",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Driver:    "foo",
						RawAction: "bar",
					},
					"labels": map[string]string{
						"status": "success",
					},
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
			{
				"sessionID": "2",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "2",
						"status":    "error",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	}

	syntheticTest(t, configStrHook, testcase)
}
