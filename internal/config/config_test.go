// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package config

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigGetDriverData(t *testing.T) {
	mockdata := map[string]interface{}{
		"test1": "string1",
		"test2": map[string]interface{}{
			"test2_1": "string2",
		},
	}

	config := &Config{
		DataSet: &DataSet{
			Drivers: mockdata,
		},
	}

	string1, ok := config.GetDriverDataStr("test1")
	assert.True(t, ok, "GetDriverDataStr should be able to find test1")
	assert.Equal(t, "string1", string1, "GetDriverDataStr should find path 'test1' point to 'string1'")
	string2, ok := config.GetDriverDataStr("test2.test2_1")
	assert.True(t, ok, "GetDriverDataStr should be able to find test2.test2_1")
	assert.Equal(t, "string2", string2, "GetDriverDataStr should find path 'test2.test2_1' point to 'string2'")
	obj, ok := config.GetDriverData("test2")
	assert.True(t, ok, "GetDriverData should be able to find test2")
	objMap, ok := obj.(map[string]interface{})
	assert.True(t, ok, "GetDriverData should be able to fetch an obj from map test2")
	assert.Equal(t, "string2", objMap["test2_1"], "GetDriverData fetched object test2 should be useable")
	nonexist, ok := config.GetDriverData("test3")
	assert.False(t, ok, "GetDriverData should set ok to false when 'test3' is not found")
	assert.Nil(t, nonexist, "GetDriverData should return nil when 'test3' is not found")
}

func TestRegexParsing(t *testing.T) {
	config := &Config{
		DataSet: &DataSet{
			Workflows: map[string]Workflow{
				"test-workflow": Workflow{
					Match: map[string]interface{}{
						"key1": ":regex:test1",
						"key2": "non regex",
					},
					UnlessMatch: map[string]interface{}{
						"key3": ":regex:test2",
						"key4": "non regex",
					},
				},
			},
			Rules: []Rule{
				Rule{
					When: Trigger{
						Match: map[string]interface{}{
							"key5": ":regex:test3",
							"key6": "non regex",
						},
					},
				},
			},
		},
	}

	assert.NotPanics(t, func() { config.parseWorkflowRegex() }, "parsing regex in config should not panic")
	assert.IsType(t, &regexp.Regexp{}, config.DataSet.Workflows["test-workflow"].Match.(map[string]interface{})["key1"], "workflow match regex should be parsed")
	assert.Equal(t, "non regex", config.DataSet.Workflows["test-workflow"].Match.(map[string]interface{})["key2"], "workflow match non-regex should remain")
	assert.IsType(t, &regexp.Regexp{}, config.DataSet.Workflows["test-workflow"].UnlessMatch.(map[string]interface{})["key3"], "workflow unless_match regex should be parsed")
	assert.Equal(t, "non regex", config.DataSet.Workflows["test-workflow"].UnlessMatch.(map[string]interface{})["key4"], "workflow unless_match non-regex should remain")
	assert.Equal(t, ":regex:test3", config.DataSet.Rules[0].When.Match["key5"], "rule match regex should remain for later driver parsing")
	assert.Equal(t, "non regex", config.DataSet.Rules[0].When.Match["key6"], "rule match non-regex should remain")
}
