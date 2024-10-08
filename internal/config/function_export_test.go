// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
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
				"test": {
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]interface{}{
							"key3": "value3",
						},
					},
					Functions: map[string]Function{
						"testFunc1": {
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
			"key2": map[string]interface{}{
				"key3": "should_not_be_used",
			},
		},
	}

	result = ExportFunctionContext(
		&Function{Target: Action{System: "test", Function: "testFunc1"}},
		map[string]interface{}{
			"ctx":     map[string]interface{}{"path": "test/path1"},
			"sysData": newSys1.Data,
		},
		&cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1", result["url"])
}

func TestFunctionExportWithParameters(t *testing.T) {
	cfg := Config{
		DataSet: &DataSet{
			Systems: map[string]System{
				"test": {
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]string{
							"key3": "value3",
						},
					},
					Functions: map[string]Function{
						"testFunc1": {
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
		&Function{Target: Action{System: "test", Function: "testFunc1"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		&cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1?q1=param1", result["url"])

	result = ExportFunctionContext(
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
				"test": {
					Data: map[string]interface{}{
						"key1": "this_should_not_be_used",
						"key2": map[string]string{
							"key3": "value3",
						},
					},
					Functions: map[string]Function{
						"testFunc1": {
							Driver:    "testDriver",
							RawAction: "action",
							Parameters: map[string]interface{}{
								"q1": "this_should_not_be_used",
								"q2": "param2",
							},
						},
					},
				},
				"test2": {
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]string{
							"key3": "value3",
						},
					},
					Functions: map[string]Function{
						"testFunc2": {
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
				"test": {
					Data: map[string]interface{}{
						"key4": "value4",
					},
					Functions: map[string]Function{
						"testFunc1": {
							Driver:    "testDriver",
							RawAction: "action",
							Parameters: map[string]interface{}{
								"q1": "$sysData.key4",
							},
						},
					},
				},
				"test2": {
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]string{
							"key3": "value3",
						},
						"key4": "this_should_not_be_used",
					},
					Functions: map[string]Function{
						"testFunc2": {
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
		&Function{Target: Action{System: "test2", Function: "testFunc2"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		&cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1?q1=value4", result["url"])
}

func TestFunctionExportWithSubsystem(t *testing.T) {
	cfg := &Config{
		Staged: &DataSet{
			Systems: map[string]System{
				"outter": {
					Extends: []string{
						"inner=test",
					},
					Data: map[string]interface{}{
						"key1": "should-not-be-used",
					},
				},
				"test": {
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]interface{}{
							"key3": "value3",
						},
					},
					Functions: map[string]Function{
						"testFunc1": {
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

	cfg.extendAllSystems()
	cfg.DataSet = cfg.Staged

	result := ExportFunctionContext(
		&Function{Target: Action{System: "outter", Function: "inner.testFunc1"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1", result["url"])
}

func TestFunctionExportWithSubsystemParameters(t *testing.T) {
	cfg := &Config{
		Staged: &DataSet{
			Systems: map[string]System{
				"outter": {
					Extends: []string{
						"inner=test",
					},
					Data: map[string]interface{}{
						"key1": "should-not-be-used",
					},
				},
				"test": {
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]interface{}{
							"key3": "value3",
						},
						"key5": "internal-data",
					},
					Functions: map[string]Function{
						"testFunc1": {
							Driver:    "testDriver",
							RawAction: "action",
							Parameters: map[string]interface{}{
								"param1": "$sysData.key5",
							},
							Export: map[string]interface{}{
								"url": "http://{{ .sysData.key1 }}.{{ .sysData.key2.key3}}/{{ .ctx.path }}?{{ .params.param1 }}",
							},
						},
					},
				},
			},
		},
	}

	cfg.extendAllSystems()
	cfg.DataSet = cfg.Staged

	result := ExportFunctionContext(
		&Function{Target: Action{System: "outter", Function: "inner.testFunc1"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1?internal-data", result["url"])
}

func TestFunctionExportWithSubsystemParentData(t *testing.T) {
	cfg := &Config{
		Staged: &DataSet{
			Systems: map[string]System{
				"outter": {
					Extends: []string{
						"inner=test",
					},
					Data: map[string]interface{}{
						"key1": "should-not-be-used",
						"key2": "belong-to-parent",
					},
				},
				"test": {
					Data: map[string]interface{}{
						"key1": "value1",
						"key2": map[string]interface{}{
							"key3": "value3",
						},
						"key5": "internal-data",
					},
					Functions: map[string]Function{
						"testFunc1": {
							Driver:    "testDriver",
							RawAction: "action",
							Export: map[string]interface{}{
								"url": "http://{{ .sysData.key1 }}.{{ .sysData.key2.key3}}/{{ .ctx.path }}?{{ .sysData.parent.key2 }}",
							},
						},
					},
				},
			},
		},
	}

	cfg.extendAllSystems()
	cfg.DataSet = cfg.Staged

	result := ExportFunctionContext(
		&Function{Target: Action{System: "outter", Function: "inner.testFunc1"}},
		map[string]interface{}{
			"ctx": map[string]interface{}{"path": "test/path1"},
		},
		cfg,
	)

	assert.Equal(t, "http://value1.value3/test/path1?belong-to-parent", result["url"])
}
