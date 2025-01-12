// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package main

import (
	"os"
	"testing"
	"time"

	"github.com/go-redis/redismock/v8"
	"github.com/honeydipper/honeydipper/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		f, _ := os.Create("test.log")
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}

func TestLoadOptions(t *testing.T) {
	driver = dipper.NewDriver(os.Args[1], "redis-cache")
	driver.Options = map[string]interface{}{
		"data": map[string]interface{}{
			"connection": map[string]interface{}{
				"Addr":     "1.1.1.1:6379",
				"Username": "nouser",
				"Password": "123",
				"DB":       "2",
			},
		},
	}

	assert.NotPanics(t, func() { start(&dipper.Message{}) }, "start and loadOptions should not panic")
	_, exists := dipper.GetMapData(driver.Options, "data.connection.Password")
	assert.False(t, exists, "Password should be removed from the driver options")
	assert.NotNil(t, redisOptions, "redisOptions should not be nil afterwards")
}

func TestSave(t *testing.T) {
	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{
		Client: db,
	}

	assert.Panics(t, func() { save(&dipper.Message{}) }, "save should panic with empty data")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key":   "foo",
			"value": "bar",
			"ttl":   "1s",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectSet("foo", "bar", time.Second).SetVal("OK")
	assert.NotPanics(t, func() { save(msg) }, "save should not panic with good data")
	select {
	case <-msg.Reply:
	default:
		assert.Fail(t, "save should reply a dipper message")
	}
}

func TestLoad(t *testing.T) {
	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{
		Client: db,
	}

	assert.Panics(t, func() { load(&dipper.Message{}) }, "load should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "foo",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectGet("foo").SetVal("bar")
	assert.NotPanics(t, func() { load(msg) }, "load should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, "bar", reply.Payload.(map[string]interface{})["value"], "load should return correct value bar")
	default:
		assert.Fail(t, "load should reply a dipper message")
	}

	mock.ClearExpect()

	msg2 := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "foo2",
		},
		Reply: make(chan dipper.Message, 1),
	}
	mock.ExpectGet("foo2").RedisNil()
	assert.NotPanics(t, func() { load(msg2) }, "load should not panic with empty return")
	select {
	case reply := <-msg2.Reply:
		assert.Nil(t, reply.Payload, "load with empty return should return a nil Payload")
	default:
		assert.Fail(t, "load with empty return should reply a dipper message")
	}
}

func TestIncrDecr(t *testing.T) {
	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{
		Client: db,
	}

	assert.Panics(t, func() { incr(&dipper.Message{}) }, "incr should panic with empty request")
	assert.Panics(t, func() { decr(&dipper.Message{}) }, "decr should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key":  "foo",
			"wrap": "true",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectIncr("foo").SetVal(1)
	assert.NotPanics(t, func() { incr(msg) }, "incr should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, int64(1), reply.Payload.(map[string]interface{})["value"], "incr should return correct value 1")
	default:
		assert.Fail(t, "incr should reply a dipper message")
	}

	mock.ClearExpect()

	msg.Reply = make(chan dipper.Message, 1)
	mock.ExpectIncr("foo").SetVal(RedisMaxInt64)
	mock.ExpectSet("foo", "0", 0).SetVal("OK")
	assert.NotPanics(t, func() { incr(msg) }, "incr should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, int64(RedisMaxInt64), reply.Payload.(map[string]interface{})["value"], "incr should return correct value 9223372036854775807")
	default:
		assert.Fail(t, "incr should reply a dipper message")
	}

	mock.ClearExpect()

	msg.Reply = make(chan dipper.Message, 1)
	mock.ExpectDecr("foo").SetVal(0)
	assert.NotPanics(t, func() { decr(msg) }, "decr should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, int64(0), reply.Payload.(map[string]interface{})["value"], "decr should return correct value 0")
	default:
		assert.Fail(t, "decr should reply a dipper message")
	}
}
