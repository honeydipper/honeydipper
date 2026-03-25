// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/go-redis/redismock/v8"
	"github.com/honeydipper/honeydipper/v4/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestHset(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{Client: db}

	assert.Panics(t, func() { hset(&dipper.Message{}) }, "hset should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "hfoo",
			"value": map[string]interface{}{
				"field": "value",
			},
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectHSet("hfoo", map[string]interface{}{"field": "value"}).SetVal(1)
	mock.ExpectEval(hexpireScript, []string{"hfoo"}, int64(86400), 1, "field").SetVal([]interface{}{int64(1)})
	assert.NotPanics(t, func() { hset(msg) }, "hset should not panic with default ttl")
	select {
	case <-msg.Reply:
	default:
		assert.Fail(t, "hset should reply a dipper message")
	}

	mock.ClearExpect()

	msg2 := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "hfoo2",
			"value": map[string]interface{}{
				"field": "value2",
			},
			"ttl": "1h",
		},
		Reply: make(chan dipper.Message, 1),
	}
	mock.ExpectHSet("hfoo2", map[string]interface{}{"field": "value2"}).SetVal(1)
	mock.ExpectEval(hexpireScript, []string{"hfoo2"}, int64(3600), 1, "field").SetVal([]interface{}{int64(1)})
	assert.NotPanics(t, func() { hset(msg2) }, "hset should not panic with custom ttl")
	select {
	case <-msg2.Reply:
	default:
		assert.Fail(t, "hset should reply a dipper message")
	}
}

func TestHvals(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{Client: db}

	assert.Panics(t, func() { hvals(&dipper.Message{}) }, "hvals should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "hfoo",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectHVals("hfoo").SetVal([]string{"\"value\""})
	assert.NotPanics(t, func() { hvals(msg) }, "hvals should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, "[\"value\"]", string(reply.Payload.([]byte)), "hvals should return hash field value")
	default:
		assert.Fail(t, "hvals should reply a dipper message")
	}
}

func TestHmget(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{Client: db}

	assert.Panics(t, func() { hmget(&dipper.Message{}) }, "hmget should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key":    "hfoo",
			"fields": []string{"field1", "field2"},
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectHMGet("hfoo", "field1", "field2").SetVal([]interface{}{"value1", "value2"})
	assert.NotPanics(t, func() { hmget(msg) }, "hmget should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, "[value1, value2]", string(reply.Payload.([]byte)), "hmget should return hash field values")
	default:
		assert.Fail(t, "hmget should reply a dipper message")
	}

	msg2 := &dipper.Message{
		Payload: map[string]interface{}{
			"key":    "hfoo",
			"fields": []interface{}{"field1", "field2"},
			"raw":    true,
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectHMGet("hfoo", "field1", "field2").SetVal([]interface{}{"value1", "value2"})
	assert.NotPanics(t, func() { hmget(msg2) }, "hmget should not panic with raw flag")
	select {
	case reply := <-msg2.Reply:
		assert.Equal(t, "value1\nvalue2", string(reply.Payload.([]byte)), "hmget with raw should return newline-separated values")
	default:
		assert.Fail(t, "hmget should reply a dipper message")
	}
}

func TestStreamHset(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{Client: db}

	assert.Panics(t, func() { streamHset(&dipper.Message{}) }, "streamHset should panic with empty request")

	now := time.Now().Truncate(time.Hour)
	if df := now.Hour() % StreamHsetIntervalHours; df != 0 {
		now = now.Add(time.Duration(-df) * time.Hour)
	}
	setName := "session_stream_" + now.Format("2006010215")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"prefix": "session_stream_",
			"key":    "sid-1",
			"value":  `{"id":"sid-1"}`,
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectHSet(setName, []string{"sid-1", `{"id":"sid-1"}`}).SetVal(1)
	mock.ExpectExpire(setName, StreamHsetTTLHours*time.Hour).SetVal(true)
	assert.NotPanics(t, func() { streamHset(msg) }, "streamHset should not panic with valid input")
	select {
	case <-msg.Reply:
	default:
		assert.Fail(t, "streamHset should reply a dipper message")
	}
}

func TestStreamHvals(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{Client: db}

	assert.Panics(t, func() { streamHvals(&dipper.Message{}) }, "streamHvals should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"prefix":    "session_stream_",
			"look_back": 2,
			"raw":       true,
		},
		Reply: make(chan dipper.Message, 1),
	}

	end := time.Now().Truncate(time.Hour)
	if df := end.Hour() % StreamHsetIntervalHours; df != 0 {
		end = end.Add(time.Duration(-df) * time.Hour)
	}
	earliest := end.Add(-2 * StreamHsetIntervalHours * time.Hour)

	keys := []string{
		"session_stream_" + earliest.Format("2006010215"),
		"session_stream_" + earliest.Add(2*time.Hour).Format("2006010215"),
		"session_stream_" + end.Format("2006010215"),
	}
	mock.ExpectEval(streamHvalsScript, keys).SetVal([]interface{}{
		`"` + keys[0] + `"`,
		`"` + keys[len(keys)-1] + `"`,
		`{"id":"sid-1"}`,
	})
	assert.NotPanics(t, func() { streamHvals(msg) }, "streamHvals should not panic with valid input")
	select {
	case reply := <-msg.Reply:
		payload := string(reply.Payload.([]byte))
		assert.True(t, strings.Contains(payload, earliest.Format("2006010215")), "streamHvals should include earliest marker")
		assert.True(t, strings.Contains(payload, end.Format("2006010215")), "streamHvals should include latest marker")
		assert.True(t, strings.Contains(payload, "sid-1"), "streamHvals should include session data")
		assert.True(t, reply.IsRaw, "streamHvals should return raw payload")
	default:
		assert.Fail(t, "streamHvals should reply a dipper message")
	}
}
