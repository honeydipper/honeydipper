// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package driver

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBuiltinDriver(t *testing.T) {
	m := &Meta{
		Name: "test",
		Type: "test",
	}

	dh := NewBuiltinDriver(m)
	assert.Equal(t, m, dh.meta, "new builtin driver should have meta")
}

func TestBuiltinAcquire(t *testing.T) {
	// test builtin acquire: table driven
	BuiltinPath = "test_fixtures/"

	testCases := map[string]interface{}{
		"panic when shortName is missing": []interface{}{
			&BuiltinDriver{meta: &Meta{HandlerData: map[string]interface{}{}}},
			"shortName is missing ", // error prefix
		},
		"panic when using relative path with / in shortName": []interface{}{
			&BuiltinDriver{meta: &Meta{HandlerData: map[string]interface{}{"shortName": "../fakecmd"}}},
			"shortName has path separator ", // error prefix
		},
		"panic when using absolute path with / in shortName": []interface{}{
			&BuiltinDriver{meta: &Meta{HandlerData: map[string]interface{}{"shortName": "/usr/bin/fakecmd"}}},
			"shortName has path separator ", // error prefix
		},
		"has full path in Executable": []interface{}{
			&BuiltinDriver{meta: &Meta{HandlerData: map[string]interface{}{"shortName": "testDriver"}}},
			"",
			"test_fixtures/testDriver",
		},
	}

	for msg, tc := range testCases {
		func(c []interface{}) {
			defer func() {
				if r := recover(); r != nil {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Equal(t, c[1], r.(error).Error()[:len(c[1].(string))], msg)
					} else {
						assert.Fail(t, "should "+msg)
					}
				} else {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Fail(t, "should "+msg)
					} else {
						assert.Equal(t, c[2].(string), c[0].(*BuiltinDriver).meta.Executable, "should "+msg)
					}
				}
			}()
			c[0].(*BuiltinDriver).Acquire()
		}(tc.([]interface{}))
	}
}

func TestBuiltinPrepare(t *testing.T) {
	// test builtin prepare: table driven

	testCases := map[string]interface{}{
		"have empty arguments if missing in meta": []interface{}{
			&BuiltinDriver{meta: &Meta{HandlerData: map[string]interface{}{}}},
			"", // no error
			0,  // len(Arguments)
		},
		"convert the list of interface to list of strings": []interface{}{
			&BuiltinDriver{meta: &Meta{HandlerData: map[string]interface{}{"arguments": []interface{}{1, 2, false, "test"}}}},
			"",               // no error
			4,                //len(Arguments)
			"1 2 false test", // concatenated parameters
		},
		"panic when arguments is not a list": []interface{}{
			&BuiltinDriver{meta: &Meta{HandlerData: map[string]interface{}{"arguments": false}}},
			"arguments in driver ", // error prefix
		},
	}

	for msg, tc := range testCases {
		func(c []interface{}) {
			defer func() {
				if r := recover(); r != nil {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Equal(t, c[1], r.(error).Error()[:len(c[1].(string))], msg)
					} else {
						assert.Fail(t, "should "+msg)
					}
				} else {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Fail(t, "should "+msg)
					} else {
						assert.Equal(t, c[2].(int), len(c[0].(*BuiltinDriver).meta.Arguments), "should "+msg)
						if (len(c[0].(*BuiltinDriver).meta.Arguments)) > 0 {
							assert.Equal(t, c[3].(string), strings.Join(c[0].(*BuiltinDriver).meta.Arguments, " "), "should "+msg)
						}
					}
				}
			}()
			c[0].(*BuiltinDriver).Prepare()
		}(tc.([]interface{}))
	}
}
