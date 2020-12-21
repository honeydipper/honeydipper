// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package driver

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDriverNewDriver(t *testing.T) {
	// test new driver: table driven
	testCases := map[string]interface{}{
		"panic when driver name is missing": []interface{}{
			map[string]interface{}{},              // driver meta
			"driver error: driver name missing: ", // error prefix
		},
		"panic when driver type is missing": []interface{}{
			map[string]interface{}{"name": "test"},    // driver meta
			"driver error: unsupported driver type: ", // error prefix
		},
		"panic when driver type is unknown": []interface{}{
			map[string]interface{}{"name": "test", "type": "faketype"}, // driver meta
			"driver error: unsupported driver type: ",                  // error prefix
		},
		"panic when builtin driver data missing": []interface{}{
			map[string]interface{}{"name": "test", "type": "builtin"}, // driver meta
			"driver error: shortName is missing for builtin driver: ", // error prefix
		},
	}

	for msg, tc := range testCases {
		func(c []interface{}) {
			defer func() {
				if r := recover(); r != nil {
					if len(c) > 1 {
						assert.Equal(t, c[1], r.(error).Error()[:len(c[1].(string))], msg)
					} else {
						assert.Fail(t, "should "+msg)
					}
				} else {
					if len(c) > 1 {
						assert.Fail(t, "should "+msg)
					}
				}
			}()
			NewDriver(c[0].(map[string]interface{}))
		}(tc.([]interface{}))
	}
}

func TestDriverStart(t *testing.T) {
	testCases := map[string]interface{}{
		"start a driver": []interface{}{ // case msg
			&Runtime{ // runtime
				Handler: &BuiltinDriver{
					meta: &Meta{
						Executable: "/fakecommand1",
						Arguments:  []string{},
					},
				},
			},
			"", // no error
			1,  // execCount
			[]string{
				"/fakecommand1 test", // first command executed
			},
		},
	}

	for msg, tc := range testCases {
		func(c []interface{}) {
			execCommandCount := 0
			execCommand = func(command string, args ...string) *exec.Cmd {
				if execCommandCount >= c[2].(int) {
					assert.Fail(t, "should "+msg+", exec.Command count too big")
				}
				assert.Equal(t, strings.Join(append([]string{command}, args...), " "), c[3].([]string)[execCommandCount], "should "+msg)
				execCommandCount++
				return generateFakeExecCommand("TestExecCommandDummy")(command, args...)
			}
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
						assert.Equal(t, c[2].(int), execCommandCount, "should "+msg)
					}
				}
			}()
			c[0].(*Runtime).Start("test")
		}(tc.([]interface{}))
	}
}
