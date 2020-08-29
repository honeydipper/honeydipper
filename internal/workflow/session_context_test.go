// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package workflow

import (
	"testing"

	"github.com/ghodss/yaml"
	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/workflow/mock_workflow"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestSessionContexts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelper := mock_workflow.NewMockSessionStoreHelper(ctrl)
	s := NewSessionStore(mockHelper)

	// tear down
	defer delete(dipper.IDMapMetadata, &s.sessions)

	configStr := `
---
contexts:
  _default:
    "*":
      foo: bar_default
    workflow1:
      foo: bar_context

  _events:
    "workflow2":
      foo: bar_event

  override:
    workflow1:
      foo: bar_override
`

	testDataSet := &config.DataSet{}
	err := yaml.Unmarshal([]byte(configStr), testDataSet)
	assert.Nil(t, err, "test config")
	testConfig := &config.Config{DataSet: testDataSet}

	mockHelper.EXPECT().GetConfig().AnyTimes().Return(testConfig)

	w := s.newSession("", "uuid1", &config.Workflow{}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{})
	assert.Equal(t, "bar_default", w.ctx["foo"], "inheriting from default context '*'")

	w = s.newSession("", "uuid2", &config.Workflow{Name: "workflow1"}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{})
	assert.Equal(t, "bar_context", w.ctx["foo"], "inheriting from default context targeted to 'workflow1'")

	w = s.newSession("", "uuid3", &config.Workflow{Name: "workflow2"}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{})
	assert.Equal(t, "bar_event", w.ctx["foo"], "inheriting from event context targeted to 'workflow2'")

	w = s.newSession("", "uuid4", &config.Workflow{Name: "workflow1", Context: "override"}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{})
	assert.Equal(t, "bar_override", w.ctx["foo"], "inheriting from overriding context")

	w = s.newSession("", "uuid5", &config.Workflow{Name: "workflow1", Local: map[string]interface{}{}}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{"foo": "bar_event"})
	assert.Equal(t, "bar_event", w.ctx["foo"], "inheriting from overriding context")

	w = s.newSession("", "uuid6", &config.Workflow{Name: "workflow1", Local: map[string]interface{}{"foo": "bar_local"}}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{"foo": "bar_event"})
	assert.Equal(t, "bar_local", w.ctx["foo"], "using local context")

	child := s.newSession("", "uuid7", &config.Workflow{Name: "workflow2", Local: map[string]interface{}{}}).(*Session)
	child.prepare(&dipper.Message{}, w, nil)
	assert.Equal(t, "bar_local", w.ctx["foo"], "inherit from parent context")

	w = s.newSession("", "uuid8", &config.Workflow{Name: "workflow1", Local: map[string]interface{}{"hooks": map[string]interface{}{"on_first_action": "bar"}}}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{"foo": "bar_event"})
	child = s.newSession("", "uuid9", &config.Workflow{Name: "workflow2", Local: map[string]interface{}{}}).(*Session)
	child.prepare(&dipper.Message{}, w, nil)
	assert.NotContains(t, w.ctx["foo"], "hooks", "not inheriting hooks from parent context")

	w = s.newSession("", "uuid10", &config.Workflow{Name: "workflow1", NoExport: []string{"data1"}}).(*Session)
	exported := map[string]interface{}{"data1": "testdata", "data2": "shouldstay"}
	w.processNoExport(exported)
	assert.NotContains(t, exported, "data1", "remove no_export items from exported data")

	w = s.newSession("", "uuid11", &config.Workflow{Name: "workflow1", NoExport: []string{"*"}}).(*Session)
	exported = map[string]interface{}{"data1": "testdata"}
	w.processNoExport(exported)
	assert.Empty(t, exported, "remove all items from exported data")
}

var configStrWithEventContexts = `
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
contexts:
  _events:
    "*":
      is_event: yes
`

func TestWorkflowWithEventContext(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{CallDriver: "foo.bar"},
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
						"is_event":     true,
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
	syntheticTest(t, configStrWithEventContexts, testcase)
}

var configStrWithEventSectionInContext = `
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
contexts:
  _default:
    _events:
      is_event: yes
`

func TestWorkflowWithEventSectionInContext(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{CallDriver: "foo.bar"},
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
						"is_event":     true,
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
	syntheticTest(t, configStrWithEventSectionInContext, testcase)
}

var configStrWithNamedContext = `
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
contexts:
  test_context:
    "*":
      is_test: yes
`

func TestWorkflowWithNamedContext(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			CallDriver: "foo.bar",
			Contexts: []interface{}{
				"test_context",
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
						"_meta_name":   "foo.bar",
						"resume_token": "//0",
						"is_test":      true,
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
	syntheticTest(t, configStrWithNamedContext, testcase)
}
