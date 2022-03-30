// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package dipper

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCommandRetrySuccess(t *testing.T) {
	b := bytes.Buffer{}
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
}

func TestCommandRetryFailure(t *testing.T) {
	b := bytes.Buffer{}
	counter := 0

	b.Reset()
	counter = 0

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
			"retry":      "1",
			"sessionID":  "2",
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
	assert.Equal(t, "error", ret.Labels["status"], "should return error after retry error")
}

func TestCommandRetryRougueFunction(t *testing.T) {
	b := bytes.Buffer{}
	counter := 0

	b.Reset()
	counter = 0

	subject := CommandProvider{
		ReturnWriter: &b,
		Channel:      "test",
		Subject:      "test",

		Commands: map[string]MessageHandler{
			"test": func(m *Message) {
				switch counter {
				case 0:
					counter++
					panic(fmt.Errorf("please retry"))
				case 1:
					counter++
					m.Reply <- Message{
						Labels: map[string]string{
							"error": "second failure 1",
						},
					}
					m.Reply <- Message{
						Labels: map[string]string{
							"error": "second failure 2",
						},
					}
					m.Reply <- Message{
						Labels: map[string]string{
							"error": "second failure 3",
						},
					}
				case 2:
					counter++
				}
				m.Reply <- Message{
					Payload: map[string]interface{}{
						"counter": strconv.Itoa(counter),
					},
				}
			},
		},
	}

	m := &Message{
		Labels: map[string]string{
			"method":     "test",
			"retry":      "2",
			"sessionID":  "2",
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

	assert.Equal(t, 3, counter, "should fail twice")
	ret := FetchMessage(&b)
	assert.Equal(t, "success", ret.Labels["status"], "should return error after retry error")
	assert.Equal(t, "3", ret.Payload.(map[string]interface{})["counter"], "should return the final counter")
}
