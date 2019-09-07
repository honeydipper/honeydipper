// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package workflow

import (
	"github.com/ghodss/yaml"
	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/workflow/mock_workflow"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"testing"
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
	err := yaml.UnmarshalStrict([]byte(configStr), testDataSet, yaml.DisallowUnknownFields)
	assert.Nil(t, err, "test config")
	testConfig := &config.Config{DataSet: testDataSet}

	mockHelper.EXPECT().GetConfig().AnyTimes().Return(testConfig)

	w := s.newSession("", &config.Workflow{}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{})
	assert.Equal(t, "bar_default", w.ctx["foo"], "inheriting from default context '*'")

	w = s.newSession("", &config.Workflow{Name: "workflow1"}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{})
	assert.Equal(t, "bar_context", w.ctx["foo"], "inheriting from default context targetted to 'workflow1'")

	w = s.newSession("", &config.Workflow{Name: "workflow2"}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{})
	assert.Equal(t, "bar_event", w.ctx["foo"], "inheriting from event context targetted to 'workflow2'")

	w = s.newSession("", &config.Workflow{Name: "workflow1", Context: "override"}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{})
	assert.Equal(t, "bar_override", w.ctx["foo"], "inheriting from overriding context")

	w = s.newSession("", &config.Workflow{Name: "workflow1", Local: map[string]interface{}{}}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{"foo": "bar_event"})
	assert.Equal(t, "bar_event", w.ctx["foo"], "inheriting from overriding context")

	w = s.newSession("", &config.Workflow{Name: "workflow1", Local: map[string]interface{}{"foo": "bar_local"}}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{"foo": "bar_event"})
	assert.Equal(t, "bar_local", w.ctx["foo"], "using local context")

	child := s.newSession("", &config.Workflow{Name: "workflow2", Local: map[string]interface{}{}}).(*Session)
	child.prepare(&dipper.Message{}, w, nil)
	assert.Equal(t, "bar_local", w.ctx["foo"], "inherit from parent context")

	w = s.newSession("", &config.Workflow{Name: "workflow1", Local: map[string]interface{}{"hooks": map[string]interface{}{"on_first_action": "bar"}}}).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{"foo": "bar_event"})
	child = s.newSession("", &config.Workflow{Name: "workflow2", Local: map[string]interface{}{}}).(*Session)
	child.prepare(&dipper.Message{}, w, nil)
	assert.NotContains(t, w.ctx["foo"], "hooks", "not inheriting hooks from parent context")
}
