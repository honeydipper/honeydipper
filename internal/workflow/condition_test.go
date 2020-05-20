// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package workflow

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/workflow/mock_workflow"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestTruthyCheck(t *testing.T) {
	assert.Falsef(t, isTruthy(dipper.InterpolateStr("{{ .something_undefined }}", map[string]interface{}{})), "interpolated <no value> should be false")
}

func TestConditionElse(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{If: []string{"false"}, Else: config.Workflow{CallDriver: "foo.bar"}},
		"msg":      &dipper.Message{},
		"ctx":      map[string]interface{}{},
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
						"_meta_name":   "foo.bar",
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
				"sessionID": "1",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "1",
						"status":    "success",
					},
				},
				"ctx": []map[string]interface{}{},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestSwitchDefault(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{Switch: "branch1", Cases: map[string]interface{}{}, Default: map[string]interface{}{
			"call_driver": "foo.bar",
		}},
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
						"_meta_name":   "foo.bar",
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
				"sessionID": "1",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "1",
						"status":    "success",
					},
				},
				"ctx": []map[string]interface{}{},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestSwitch(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{Switch: "branch1", Cases: map[string]interface{}{
			"branch1": map[string]interface{}{
				"call_driver": "foo.bar",
			},
		}},
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
						"_meta_name":   "foo.bar",
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
				"sessionID": "1",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "1",
						"status":    "success",
					},
				},
				"ctx": []map[string]interface{}{},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowLoop(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			Local: map[string]interface{}{
				"counter": 3,
			},
			While: []string{"$ctx.counter"},
			Steps: []config.Workflow{
				{
					CallFunction: "foo_sys.bar_func",
					Export: map[string]interface{}{
						"counter": "{{ sub (int .ctx.counter) 1 }}",
					},
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
						"counter":      3,
						"loop_count":   0,
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
				"ctx": []map[string]interface{}{},
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
								"_meta_name":   "foo_sys.bar_func",
								"resume_token": "//0",
								"counter":      "2",
								"loop_count":   1,
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
				"ctx": []map[string]interface{}{},
				"asserts": func() {
					mockHelper.EXPECT().SendMessage(gomock.Eq(&dipper.Message{
						Channel: "eventbus",
						Subject: "command",
						Labels: map[string]string{
							"sessionID": "3",
						},
						Payload: map[string]interface{}{
							"ctx": map[string]interface{}{
								"_meta_desc":   "",
								"_meta_name":   "foo_sys.bar_func",
								"resume_token": "//0",
								"counter":      "1",
								"loop_count":   2,
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
							"labels": map[string]string{
								"sessionID": "2",
								"status":    "success",
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
				"ctx": []map[string]interface{}{},
			},
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestSessionConditions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelper := mock_workflow.NewMockSessionStoreHelper(ctrl)
	s := NewSessionStore(mockHelper)

	w := s.newSession("", &config.Workflow{If: []string{"yes", "1", "true"}}).(*Session)
	assert.True(t, w.checkCondition(), "return true if all truy value")

	w = s.newSession("", &config.Workflow{If: []string{"yes", "0", "true"}}).(*Session)
	assert.False(t, w.checkCondition(), "return false if any false value")

	w = s.newSession("", &config.Workflow{IfAny: []string{"nil", "0", "true"}}).(*Session)
	assert.True(t, w.checkCondition(), "return true if_any truy value")

	w = s.newSession("", &config.Workflow{IfAny: []string{"nil", "0", ""}}).(*Session)
	assert.False(t, w.checkCondition(), "return false if_any all false value")

	w = s.newSession("", &config.Workflow{Unless: []string{"nil", "0", ""}}).(*Session)
	assert.True(t, w.checkCondition(), "return true  unless all false value")

	w = s.newSession("", &config.Workflow{Unless: []string{"nil", "1", ""}}).(*Session)
	assert.False(t, w.checkCondition(), "return true  unless with a truy value")

	w = s.newSession("", &config.Workflow{UnlessAll: []string{"true", "1", "yes"}}).(*Session)
	assert.False(t, w.checkCondition(), "return false  unless_all with all truy values")

	w = s.newSession("", &config.Workflow{UnlessAll: []string{"nil", "1", ""}}).(*Session)
	assert.True(t, w.checkCondition(), "return true  unless_all with some false value")

	w = s.newSession("", &config.Workflow{
		Match: map[string]interface{}{
			"expect_value1": "value1",
		},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkCondition(), "return true when ctx match skeleton")
	w.ctx = map[string]interface{}{
		"expect_value1": "value2",
		"expect_other":  "not matter",
	}
	assert.False(t, w.checkCondition(), "return false when ctx not match skeleton")

	w = s.newSession("", &config.Workflow{
		Match: []interface{}{
			map[string]interface{}{"expect_value1": "value1"},
			map[string]interface{}{"expect_value1": "value2"},
		},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkCondition(), "return true when ctx match one of the skeletons")
	w.ctx = map[string]interface{}{
		"expect_value1": "value2",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkCondition(), "return true when ctx match the other skeleton")
	w.ctx = map[string]interface{}{
		"expect_value1": "value3",
		"expect_other":  "not matter",
	}
	assert.False(t, w.checkCondition(), "return false when ctx not match any of the skeletons")

	w = s.newSession("", &config.Workflow{
		UnlessMatch: map[string]interface{}{
			"expect_value1": "value1",
		},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value2",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkCondition(), "return true when ctx unless_match not match skeleton")
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.False(t, w.checkCondition(), "return false when ctx unless_match match skeleton")

	w = s.newSession("", &config.Workflow{
		UnlessMatch: []interface{}{
			map[string]interface{}{"expect_value1": "value1"},
			map[string]interface{}{"expect_value1": "value2"},
		},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.False(t, w.checkCondition(), "return false when unless_match ctx match one of the skeletons")
	w.ctx = map[string]interface{}{
		"expect_value1": "value3",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkCondition(), "return true when unless_match ctx not match any of the skeletons")

	w = s.newSession("", &config.Workflow{
		UnlessMatch: []interface{}{},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkCondition(), "return true when unless_match is empty list")
}

func TestSessionLoopConditions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelper := mock_workflow.NewMockSessionStoreHelper(ctrl)
	s := NewSessionStore(mockHelper)

	msg := &dipper.Message{}

	w := s.newSession("", &config.Workflow{While: []string{"yes", "1", "true"}}).(*Session)
	assert.True(t, w.checkLoopCondition(msg), "return true if all truy value")

	w = s.newSession("", &config.Workflow{While: []string{"yes", "0", "true"}}).(*Session)
	assert.False(t, w.checkLoopCondition(msg), "return false if any false value")

	w = s.newSession("", &config.Workflow{WhileAny: []string{"nil", "0", "true"}}).(*Session)
	assert.True(t, w.checkLoopCondition(msg), "return true if_any truy value")

	w = s.newSession("", &config.Workflow{WhileAny: []string{"nil", "0", ""}}).(*Session)
	assert.False(t, w.checkLoopCondition(msg), "return false if_any all false value")

	w = s.newSession("", &config.Workflow{Until: []string{"nil", "0", ""}}).(*Session)
	assert.True(t, w.checkLoopCondition(msg), "return true  unless all false value")

	w = s.newSession("", &config.Workflow{Until: []string{"nil", "1", ""}}).(*Session)
	assert.False(t, w.checkLoopCondition(msg), "return true  unless with a truy value")

	w = s.newSession("", &config.Workflow{UntilAll: []string{"true", "1", "yes"}}).(*Session)
	assert.False(t, w.checkLoopCondition(msg), "return false  unless_all with all truy values")

	w = s.newSession("", &config.Workflow{UntilAll: []string{"nil", "1", ""}}).(*Session)
	assert.True(t, w.checkLoopCondition(msg), "return true  unless_all with some false value")

	w = s.newSession("", &config.Workflow{
		WhileMatch: map[string]interface{}{
			"expect_value1": "value1",
		},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkLoopCondition(msg), "return true when ctx match skeleton")
	w.ctx = map[string]interface{}{
		"expect_value1": "value2",
		"expect_other":  "not matter",
	}
	assert.False(t, w.checkLoopCondition(msg), "return false when ctx not match skeleton")

	w = s.newSession("", &config.Workflow{
		WhileMatch: []interface{}{
			map[string]interface{}{"expect_value1": "value1"},
			map[string]interface{}{"expect_value1": "value2"},
		},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkLoopCondition(msg), "return true when ctx match one of the skeletons")
	w.ctx = map[string]interface{}{
		"expect_value1": "value2",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkLoopCondition(msg), "return true when ctx match the other skeleton")
	w.ctx = map[string]interface{}{
		"expect_value1": "value3",
		"expect_other":  "not matter",
	}
	assert.False(t, w.checkLoopCondition(msg), "return false when ctx not match any of the skeletons")

	w = s.newSession("", &config.Workflow{
		UntilMatch: map[string]interface{}{
			"expect_value1": "value1",
		},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value2",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkLoopCondition(msg), "return true when ctx unless_match not match skeleton")
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.False(t, w.checkLoopCondition(msg), "return false when ctx unless_match match skeleton")

	w = s.newSession("", &config.Workflow{
		UntilMatch: []interface{}{
			map[string]interface{}{"expect_value1": "value1"},
			map[string]interface{}{"expect_value1": "value2"},
		},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.False(t, w.checkLoopCondition(msg), "return false when unless_match ctx match one of the skeletons")
	w.ctx = map[string]interface{}{
		"expect_value1": "value3",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkLoopCondition(msg), "return true when unless_match ctx not match any of the skeletons")

	w = s.newSession("", &config.Workflow{
		UntilMatch: []interface{}{},
	}).(*Session)
	w.ctx = map[string]interface{}{
		"expect_value1": "value1",
		"expect_other":  "not matter",
	}
	assert.True(t, w.checkLoopCondition(msg), "return true when unless_match is empty list")
}
