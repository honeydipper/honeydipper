// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
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
		{
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

func TestCheckMapData(t *testing.T) {
	data := map[string]interface{}{
		"1": map[string]interface{}{
			"a": "there",
			"b": "false",
			"c": 0,
		},
		"2": []interface{}{
			false,
			"true",
			true,
		},
	}

	assert.False(t, CheckMapData(data, "1.a"), `"1.a" value "there" should be false.`)
	assert.False(t, CheckMapData(data, "1.b"), `"1.b" value "false" should be false.`)
	assert.False(t, CheckMapData(data, "1.c"), `"1.c" value 0 should be false.`)
	assert.False(t, CheckMapData(data, "1.d"), `"1.d" not present should be false.`)
	assert.False(t, CheckMapData(data, "2.0"), `"2.0" value false should be false.`)
	assert.False(t, CheckMapData(data, "2.3"), `"2.3" value not present should be false.`)

	assert.True(t, CheckMapData(data, "2.1"), `"2.1" value "true" should be true.`)
	assert.True(t, CheckMapData(data, "2.2"), `"2.2" value boolean "true" should be true.`)
}

func TestGetMapDataInt(t *testing.T) {
	data := map[string]any{
		"intvalue":    12,
		"int64value":  3000,
		"floatvalue":  34.22202,
		"stringvalue": "33",
		"boolvalue":   true,
		"nonint":      "this is no a int",
	}

	v, ok := GetMapDataInt(data, "intvalue")
	assert.Equal(t, 12, v, "able to get int value")
	assert.True(t, ok, "able to get int value")

	v, ok = GetMapDataInt(data, "int64value")
	assert.Equal(t, 3000, v, "able to get int64 value")
	assert.True(t, ok, "able to get int64 value")

	v, ok = GetMapDataInt(data, "floatvalue")
	assert.Equal(t, 34, v, "able to get float value")
	assert.True(t, ok, "able to get float value")

	v, ok = GetMapDataInt(data, "stringvalue")
	assert.Equal(t, 33, v, "able to get string value")
	assert.True(t, ok, "able to get string value")

	v, ok = GetMapDataInt(data, "boolvalue")
	assert.Equal(t, 0, v, "error when getting bool value")
	assert.False(t, ok, "error when getting bool value")

	v, ok = GetMapDataInt(data, "nonint")
	assert.Equal(t, 0, v, "error when getting non-int value")
	assert.False(t, ok, "error when getting non-int value")

	assert.Panics(t, func() { v = MustGetMapDataInt(data, "nonint") }, "panic when getting non-int with must prefix")
	assert.Panics(t, func() { v = MustGetMapDataInt(data, "non-exist") }, "panic when getting non-exist with must prefix")
}
