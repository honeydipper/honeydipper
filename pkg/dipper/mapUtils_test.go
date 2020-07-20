// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package dipper

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecursive(t *testing.T) {
	type testStruct struct {
		Field1 string
		Field2 []int
		Field3 map[string]interface{}
		Field4 map[string]interface{}
		field5 string
	}

	process := func(path string, val interface{}) (newval interface{}, ok bool) {
		switch v := val.(type) {
		case string:
			newval = v + " processed"
			ok = true
		case int:
			newval = v + 1000
			ok = true
		}
		return newval, ok
	}

	testCases := []interface{}{
		map[string]interface{}{
			"key1": "val1",
			"key2": []int{1, 2, 3},
			"key3": map[string]interface{}{
				"childkey1": "val2",
			},
		},
		&testStruct{
			Field1: "val1",
			Field2: []int{1, 2, 3},
			Field3: map[string]interface{}{
				"childkey1": "val2",
			},
			field5: "val3",
		},
		map[string]interface{}{
			"key1": "val1",
			"key2": []int{1, 2, 3},
			"key3": &testStruct{
				Field1: "val2",
			},
		},
	}

	testExpects := []interface{}{
		map[string]interface{}{
			"key1": "val1 processed",
			"key2": []int{1001, 1002, 1003},
			"key3": map[string]interface{}{
				"childkey1": "val2 processed",
			},
		},
		testStruct{
			Field1: "val1 processed",
			Field2: []int{1001, 1002, 1003},
			Field3: map[string]interface{}{
				"childkey1": "val2 processed",
			},
			field5: "val3",
		},
		map[string]interface{}{
			"key1": "val1 processed",
			"key2": []int{1001, 1002, 1003},
			"key3": &testStruct{
				Field1: "val2 processed",
			},
		},
	}

	for i := 0; i < len(testCases); i++ {
		testValue := testCases[i]
		Recursive(testValue, process)
		if reflect.ValueOf(testValue).Kind() == reflect.Ptr {
			testValue = reflect.ValueOf(testValue).Elem().Interface()
		}
		assert.Equal(t, testExpects[i], testValue, "recursive test case %v failed", i)
	}
}

func TestDeepCopyNil(t *testing.T) {
	var ret interface{}
	var err error
	assert.NotPanics(t, func() { ret, err = DeepCopy(nil) })
	assert.Nil(t, ret)
	assert.Nil(t, err)
}

func TestDeepCopyNilMap(t *testing.T) {
	var ret interface{}
	var err error
	assert.NotPanics(t, func() { ret, err = DeepCopyMap(nil) })
	assert.Nil(t, ret)
	assert.Nil(t, err)
}

func TestDeepCopy(t *testing.T) {
	var ret interface{}
	var err error
	src := map[string]interface{}{
		"key1": "value1",
		"key2": 2,
		"key3": true,
	}
	assert.NotPanics(t, func() { ret, err = DeepCopy(src) })
	assert.Equal(t, src, ret)
	src["key3"] = false
	assert.NotEqual(t, src, ret)
	assert.Nil(t, err)
}

func TestMerge(t *testing.T) {

	testCases := []map[string]interface{}{
		map[string]interface{}{
			"name": "append modifier with nil in src",
			"dst": map[string]interface{}{
				"f1": "d1",
				"f2": []interface{}{"item1", "item2"},
			},
			"src": map[string]interface{}{
				"f2+": nil,
			},
			"expect": map[string]interface{}{
				"f1": "d1",
				"f2": []interface{}{"item1", "item2"},
			},
		},
		{
			"name": "append modifier with nil in dst",
			"dst": map[string]interface{}{
				"f1": "d1",
				"f2": nil,
			},
			"src": map[string]interface{}{
				"f2+": []interface{}{"item1", "item2"},
			},
			"expect": map[string]interface{}{
				"f1": "d1",
				"f2": []interface{}{"item1", "item2"},
			},
		},
		{
			"name": "append modifier missing key in dst",
			"dst": map[string]interface{}{
				"f1": "d1",
			},
			"src": map[string]interface{}{
				"f2+": []interface{}{"item1", "item2"},
			},
			"expect": map[string]interface{}{
				"f1": "d1",
				"f2": []interface{}{"item1", "item2"},
			},
		},
	}
	for _, c := range testCases {
		d, _ := c["dst"].(map[string]interface{})
		s := c["src"]
		e := c["expect"]
		n := c["name"]

		assert.NotPanics(t, func() { MergeMap(d, s) })
		assert.Equal(t, e, d, n)
	}
}
