// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package service

import (
	"bytes"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/internal/driver"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

type bytesBuffer struct {
	*bytes.Buffer
}

func (b bytesBuffer) Close() error {
	return nil
}

func TestExecuteWorkflow(t *testing.T) {
	wf := config.Workflow{
		Type: "function",
		Content: map[string]interface{}{
			"driver":    "test",
			"rawAction": "test",
		},
		Data: map[string]interface{}{
			"param1": "{{ empty .data.test }}",
		},
	}
	msg := &dipper.Message{
		Channel: "eventbus",
		Subject: "return",
		Labels: map[string]string{
			"status": "success",
		},
		Payload: nil,
	}
	parent := &WorkflowSession{
		step:  1,
		Type:  "pipe",
		event: map[string]interface{}{},
		work: []*config.Workflow{
			&config.Workflow{
				Type: "function",
			},
			&wf,
		},
	}
	sessions = map[string]*WorkflowSession{}
	dipper.IDMapMetadata = map[dipper.IDMap]*dipper.IDMapMeta{}
	dipper.InitIDMap(&sessions)

	var b = bytesBuffer{&bytes.Buffer{}}
	b.Grow(512)
	engine = &Service{
		name: "test",
		driverRuntimes: map[string]*driver.Runtime{
			"eventbus": &driver.Runtime{
				Output: b,
			},
		},
	}
	sessionID := dipper.IDMapPut(&sessions, parent)
	testFunc := func() {
		executeWorkflow(sessionID, &wf, msg, nil)
	}
	assert.NotPanics(t, testFunc, "Should not panic when Payload is nil")
	assert.NotZero(t, b.Len(), "Should send message to eventbus")
}

func TestSuspendWorkflowTimeout(t *testing.T) {
	wf := config.Workflow{
		Type:    "suspend",
		Content: "test-suspend",
		Data: map[string]interface{}{
			"timeout": "0.5s",
			"labels": map[string]interface{}{
				"status": "success",
			},
		},
	}
	msg := &dipper.Message{
		Channel: "eventbus",
		Subject: "return",
		Labels: map[string]string{
			"status": "success",
		},
		Payload: nil,
	}
	parent := &WorkflowSession{
		step:  1,
		Type:  "pipe",
		event: map[string]interface{}{},
		work: []*config.Workflow{
			&config.Workflow{
				Type: "function",
			},
			&wf,
			&config.Workflow{
				Type:      "if",
				Condition: "false",
				Content: []interface{}{
					map[string]interface{}{
						"content": "noop",
					},
				},
			},
		},
	}
	sessions = map[string]*WorkflowSession{}
	dipper.IDMapMetadata = map[dipper.IDMap]*dipper.IDMapMeta{}
	dipper.InitIDMap(&sessions)

	var b = bytesBuffer{&bytes.Buffer{}}
	b.Grow(512)
	engine = &Service{
		name: "test",
		driverRuntimes: map[string]*driver.Runtime{
			"eventbus": &driver.Runtime{
				Output: b,
			},
		},
	}
	sessionID := dipper.IDMapPut(&sessions, parent)
	testFunc := func() {
		executeWorkflow(sessionID, &wf, msg, nil)
	}
	assert.NotPanics(t, testFunc, "Should not panic when Payload is nil")
	assert.NotEmpty(t, suspendedSessions, "Session should be suspended but kept in memory")

	s := make(chan int, 1)
	go func() {
		daemon.Children.Wait()
		s <- 1
	}()

	select {
	case <-time.After(time.Second * 2):
		assert.Fail(t, "Timeout waiting for resuming suspended session")
	case <-s:
		assert.Empty(t, suspendedSessions, "Resumed session should be removed after completion")
	}
}

func TestExportEventContext(t *testing.T) {
	engine = &Service{
		name: "test",
		config: &config.Config{
			DataSet: &config.DataSet{
				Systems: map[string]config.System{
					"test_system1": config.System{
						Triggers: map[string]config.Trigger{
							"direct": config.Trigger{
								Driver:   "test_driver",
								RawEvent: "test_raw_event",
								Export: map[string]interface{}{
									"field1": "value1",
									"field2": "value2_old",
									"list1": []interface{}{
										"str1",
										"str2",
									},
									"list2": []interface{}{
										"str1",
										"{{ .event.line1 }}",
									},
								},
							},
							"override1": config.Trigger{
								Source: config.Event{
									System:  "test_system1",
									Trigger: "direct",
								},
								Export: map[string]interface{}{
									"field2": "value2_new",
								},
							},
							"override2": config.Trigger{
								Source: config.Event{
									System:  "test_system1",
									Trigger: "direct",
								},
								Export: map[string]interface{}{
									"list1": []interface{}{
										"str3",
										"str4",
									},
									"*list2": []interface{}{
										"str3",
										"str4",
									},
								},
							},
							"override3": config.Trigger{
								Source: config.Event{
									System:  "test_system1",
									Trigger: "override1",
								},
								Export: map[string]interface{}{
									"list1": []interface{}{
										"str3",
										"str4",
									},
									"*list2": []interface{}{
										"str3",
										"str4",
									},
									"field3": "value3_new",
								},
							},
						},
					},
				},
			},
		},
	}

	envData := map[string]interface{}{
		"event": map[string]interface{}{
			"line1": "words words words",
		},
	}

	var ctx map[string]interface{}
	tests := []map[string]interface{}{
		{
			"name": "trigger calling rawEvent",
			"func": func() { ctx = exportContext(engine.config.DataSet.Systems["test_system1"].Triggers["direct"], envData) },
			"expected": map[string]interface{}{
				"field1": "value1",
				"field2": "value2_old",
				"list1": []interface{}{
					"str1",
					"str2",
				},
				"list2": []interface{}{
					"str1",
					"words words words",
				},
			},
		},
		{
			"name": "override1 trigger",
			"func": func() {
				ctx = exportContext(engine.config.DataSet.Systems["test_system1"].Triggers["override1"], envData)
			},
			"expected": map[string]interface{}{
				"field1": "value1",
				"field2": "value2_new",
				"list1": []interface{}{
					"str1",
					"str2",
				},
				"list2": []interface{}{
					"str1",
					"words words words",
				},
			},
		},
		{
			"name": "override2 trigger with lists",
			"func": func() {
				ctx = exportContext(engine.config.DataSet.Systems["test_system1"].Triggers["override2"], envData)
			},
			"expected": map[string]interface{}{
				"field1": "value1",
				"field2": "value2_old",
				"list1": []interface{}{
					"str1",
					"str2",
					"str3",
					"str4",
				},
				"list2": []interface{}{
					"str3",
					"str4",
				},
			},
		},
		{
			"name": "override3 over override1",
			"func": func() {
				ctx = exportContext(engine.config.DataSet.Systems["test_system1"].Triggers["override3"], envData)
			},
			"expected": map[string]interface{}{
				"field1": "value1",
				"field2": "value2_new",
				"field3": "value3_new",
				"list1": []interface{}{
					"str1",
					"str2",
					"str3",
					"str4",
				},
				"list2": []interface{}{
					"str3",
					"str4",
				},
			},
		},
	}

	for _, test := range tests {
		assert.NotPanicsf(t, test["func"].(func()), "exporting from %v should not panic", test["name"])
		assert.Equalf(t, test["expected"], ctx, "exported context from %v should match", test["name"])
	}
}
