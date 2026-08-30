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

func TestHgetall(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{Client: db}

	assert.Panics(t, func() { hgetall(&dipper.Message{}) }, "hgetall should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "hfoo",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectHGetAll("hfoo").SetVal(map[string]string{"field1": "value1", "field2": "value2"})
	assert.NotPanics(t, func() { hgetall(msg) }, "hgetall should not panic with good data")
	select {
	case reply := <-msg.Reply:
		result, ok := reply.Payload.(map[string]any)
		assert.True(t, ok, "hgetall should return a map[string]any")
		assert.Equal(t, "value1", result["field1"], "hgetall should return field1 value")
		assert.Equal(t, "value2", result["field2"], "hgetall should return field2 value")
	default:
		assert.Fail(t, "hgetall should reply a dipper message")
	}

	mock.ClearExpect()

	msg2 := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "hfoo2",
		},
		Reply: make(chan dipper.Message, 1),
	}
	// A non-existent key returns an empty map and nil error from go-redis.
	mock.ExpectHGetAll("hfoo2").SetVal(map[string]string{})
	assert.NotPanics(t, func() { hgetall(msg2) }, "hgetall should not panic on empty hash")
	select {
	case reply := <-msg2.Reply:
		result, ok := reply.Payload.(map[string]any)
		assert.True(t, ok, "hgetall should return a map[string]any")
		assert.Empty(t, result, "hgetall with empty hash should return an empty map")
	default:
		assert.Fail(t, "hgetall with empty hash should reply a dipper message")
	}
}
