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
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

func TestWorkflowErrorContextNotDefined(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{Context: "notdefined", CallDriver: "shouldnot.call"},
		"msg":      &dipper.Message{},
		"ctx":      map[string]interface{}{},
		"steps":    []map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowErrorInvalidContextName(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			CallDriver: "foo.bar",
			Contexts: []interface{}{
				"test_context",
				123,
			},
		},
		"msg":   &dipper.Message{},
		"ctx":   map[string]interface{}{},
		"steps": []map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
	}
	syntheticTest(t, configStrWithNamedContext, testcase)
}

func TestWorkflowErrorWorkflowNotDefined(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{Workflow: "notdefined", Description: "should fail but caught by the outter loop"},
		"msg":      &dipper.Message{},
		"ctx":      map[string]interface{}{},
		"steps":    []map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowIterateEmpty(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			CallFunction: "foo_sys.bar_func",
			Iterate:      "$?nil",
		},
		"msg":   &dipper.Message{},
		"ctx":   map[string]interface{}{},
		"steps": []map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowIterateEmptyAsChild(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{
			Steps: []config.Workflow{
				{
					CallFunction: "foo_sys.bar_func",
					Iterate:      "$?nil",
				},
			},
		},
		"msg":   &dipper.Message{},
		"ctx":   map[string]interface{}{},
		"steps": []map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
	}
	syntheticTest(t, configStr, testcase)
}

func TestWorkflowErrorInvalidElse(t *testing.T) {
	testcase := map[string]interface{}{
		"workflow": &config.Workflow{If: []string{"false"}, Else: map[string]interface{}{"call_workflow": 123}},
		"msg":      &dipper.Message{},
		"ctx":      map[string]interface{}{},
		"steps":    []map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(gomock.Any()).Times(0)
		},
	}
	syntheticTest(t, configStr, testcase)
}
