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

func TestRecusive(t *testing.T) {
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
