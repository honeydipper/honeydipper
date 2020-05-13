// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package dipper

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCommandRetry(t *testing.T) {
	var b = bytes.Buffer{}
	counter := 0

	subject := CommandProvider{
		ReturnWriter: &b,
		Channel:      "test",
		Subject:      "test",

		Commands: map[string]MessageHandler{
			"test": func(m *Message) {
				if counter < 2 {
					counter++
					panic(fmt.Errorf("please retry"))
				}
				m.Reply <- Message{}
			},
		},
	}

	m := &Message{
		Labels: map[string]string{
			"method":     "test",
			"retry":      "2",
			"sessionID":  "1",
			"backoff_ms": "10",
		},
	}

	subject.Router(m)

	sleepCount := 0
	for sleepCount < 100 && m.Reply != nil {
		time.Sleep(time.Millisecond * 5)
		sleepCount++
	}

	if sleepCount == 100 {
		assert.Fail(t, "timeout waiting for retry to send result")
	}

	assert.Equal(t, 2, counter, "should fail twice")
	ret := FetchMessage(&b)
	assert.Equal(t, "success", ret.Labels["status"], "should return success after retry")

	b.Reset()
	counter = 0
	m = &Message{
		Labels: map[string]string{
			"method":     "test",
			"retry":      "1",
			"sessionID":  "2",
			"backoff_ms": "10",
		},
	}

	subject.Router(m)

	sleepCount = 0
	for sleepCount < 100 && m.Reply != nil {
		time.Sleep(time.Millisecond * 5)
		sleepCount++
	}

	if sleepCount == 100 {
		assert.Fail(t, "timeout waiting for retry to send result")
	}

	assert.Equal(t, 2, counter, "should fail twice")
	ret = FetchMessage(&b)
	assert.Equal(t, "error", ret.Labels["status"], "should return error after retry error")
}
