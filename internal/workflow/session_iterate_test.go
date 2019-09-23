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

var configStrIterate = `
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
				"ctx": []map[string]interface{}{},
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
				"ctx": []map[string]interface{}{},
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
				"ctx": []map[string]interface{}{},
			},
		},
	}
	syntheticTest(t, configStrIterate, testcase)
}
