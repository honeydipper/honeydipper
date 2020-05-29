// Copyright 2020 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFunctionExport(t *testing.T) {
	cfg := Config{
		DataSet: &DataSet{
			Systems: map[string]System{
				"test": System{
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]string{
							"key3": "value3",
						},
					},
					Functions: map[string]Function{
						"testFunc1": Function{
							Driver:    "testDriver",
							RawAction: "action",
							Export: map[string]interface{}{
								"url": "http://{{ .sysData.key1 }}.{{ .sysData.key2.key3}}/{{ .ctx.path }}",
							},
						},
					},
				},
			},
		},
	}

	result := ExportFunctionContext(
		nil,
		&Function{Target: Action{System: "test", Function: "testFunc1"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		&cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1", result["url"])

	newSys1 := &System{
		Data: map[string]interface{}{
			"key1": "value11",
			"key2": "value22",
		},
	}

	result = ExportFunctionContext(
		newSys1,
		&Function{Target: Action{System: "test", Function: "testFunc1"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		&cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1", result["url"])
}

func TestFunctionExportWithParameters(t *testing.T) {
	cfg := Config{
		DataSet: &DataSet{
			Systems: map[string]System{
				"test": System{
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]string{
							"key3": "value3",
						},
					},
					Functions: map[string]Function{
						"testFunc1": Function{
							Driver:    "testDriver",
							RawAction: "action",
							Parameters: map[string]interface{}{
								"q1": "param1",
								"q2": "param2",
							},
							Export: map[string]interface{}{
								"url": "http://{{ .sysData.key1 }}.{{ .sysData.key2.key3}}/{{ .ctx.path }}?q1={{ .params.q1 }}",
							},
						},
					},
				},
			},
		},
	}

	result := ExportFunctionContext(
		nil,
		&Function{Target: Action{System: "test", Function: "testFunc1"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		&cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1?q1=param1", result["url"])

	result = ExportFunctionContext(
		nil,
		&Function{
			Target: Action{System: "test", Function: "testFunc1"},
			Parameters: map[string]interface{}{
				"q1": "updated",
			},
		},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		&cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1?q1=updated", result["url"])
}

func TestFunctionExportWithSquashedParameters(t *testing.T) {
	cfg := Config{
		DataSet: &DataSet{
			Systems: map[string]System{
				"test": System{
					Data: map[string]interface{}{
						"key1": "this_should_not_be_used",
						"key2": map[string]string{
							"key3": "value3",
						},
					},
					Functions: map[string]Function{
						"testFunc1": Function{
							Driver:    "testDriver",
							RawAction: "action",
							Parameters: map[string]interface{}{
								"q1": "this_should_not_be_used",
								"q2": "param2",
							},
						},
					},
				},
				"test2": System{
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]string{
							"key3": "value3",
						},
					},
					Functions: map[string]Function{
						"testFunc2": Function{
							Target: Action{
								System:   "test",
								Function: "testFunc1",
							},
							Parameters: map[string]interface{}{
								"q1": "param1",
								"q2": "param2",
							},
							Export: map[string]interface{}{
								"url": "http://{{ .sysData.key1 }}.{{ .sysData.key2.key3}}/{{ .ctx.path }}?q1={{ .params.q1 }}",
							},
						},
					},
				},
			},
		},
	}

	result := ExportFunctionContext(
		nil,
		&Function{Target: Action{System: "test2", Function: "testFunc2"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		&cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1?q1=param1", result["url"])
}

func TestFunctionExportWithSquashedSysData(t *testing.T) {
	cfg := Config{
		DataSet: &DataSet{
			Systems: map[string]System{
				"test": System{
					Data: map[string]interface{}{
						"key4": "value4",
					},
					Functions: map[string]Function{
						"testFunc1": Function{
							Driver:    "testDriver",
							RawAction: "action",
							Parameters: map[string]interface{}{
								"q1": "$sysData.key4",
							},
						},
					},
				},
				"test2": System{
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]string{
							"key3": "value3",
						},
						"key4": "this_should_not_be_used",
					},
					Functions: map[string]Function{
						"testFunc2": Function{
							Target: Action{
								System:   "test",
								Function: "testFunc1",
							},
							Export: map[string]interface{}{
								"url": "http://{{ .sysData.key1 }}.{{ .sysData.key2.key3}}/{{ .ctx.path }}?q1={{ .params.q1 }}",
							},
						},
					},
				},
			},
		},
	}

	result := ExportFunctionContext(
		nil,
		&Function{Target: Action{System: "test2", Function: "testFunc2"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		&cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1?q1=value4", result["url"])
}
