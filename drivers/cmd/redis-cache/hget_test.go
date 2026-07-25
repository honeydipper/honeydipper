// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package main

import (
	"testing"

	"github.com/go-redis/redismock/v8"
	"github.com/honeydipper/honeydipper/v4/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestHget(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{Client: db}

	assert.Panics(t, func() { hget(&dipper.Message{}) }, "hget should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key":   "hfoo",
			"field": "field1",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectHGet("hfoo", "field1").SetVal("value1")
	assert.NotPanics(t, func() { hget(msg) }, "hget should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, "value1", string(reply.Payload.([]byte)), "hget should return the field value")
		assert.True(t, reply.IsRaw, "hget should return raw payload")
	default:
		assert.Fail(t, "hget should reply a dipper message")
	}

	mock.ClearExpect()

	msg2 := &dipper.Message{
		Payload: map[string]interface{}{
			"key":   "hfoo2",
			"field": "missing",
		},
		Reply: make(chan dipper.Message, 1),
	}
	mock.ExpectHGet("hfoo2", "missing").RedisNil()
	assert.NotPanics(t, func() { hget(msg2) }, "hget should not panic on cache miss")
	select {
	case reply := <-msg2.Reply:
		assert.Nil(t, reply.Payload, "hget with cache miss should return a nil Payload")
	default:
		assert.Fail(t, "hget with cache miss should reply a dipper message")
	}
}
