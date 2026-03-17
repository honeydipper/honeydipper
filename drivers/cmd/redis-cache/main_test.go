// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-redis/redismock/v8"
	"github.com/honeydipper/honeydipper/v3/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
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
	if driver == nil {
		TestLoadOptions(t)
	}

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
	if driver == nil {
		TestLoadOptions(t)
	}

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
		assert.Equal(t, "bar", string(reply.Payload.([]byte)), "load should return correct value bar")
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

func TestIncr(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{
		Client: db,
	}

	assert.Panics(t, func() { incr(&dipper.Message{}) }, "incr should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "foo",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectIncr("foo").SetVal(1)
	assert.NotPanics(t, func() { incr(msg) }, "incr should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, "1", string(reply.Payload.([]byte)), "incr should return correct value 1")
	default:
		assert.Fail(t, "incr should reply a dipper message")
	}

	mock.ClearExpect()

	msg2 := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "foo2",
		},
		Reply: make(chan dipper.Message, 1),
	}
	mock.ExpectIncr("foo2").SetVal(2)
	assert.NotPanics(t, func() { incr(msg2) }, "incr should not panic with good data")
	select {
	case reply := <-msg2.Reply:
		assert.Equal(t, "2", string(reply.Payload.([]byte)), "incr should return correct value 1")
	default:
		assert.Fail(t, "incr should reply a dipper message")
	}
}

func TestLrange(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{
		Client: db,
	}

	assert.Panics(t, func() { lrange(&dipper.Message{}) }, "lrange should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "foo",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectLRange("foo", 0, -1).SetVal([]string{"bar", "baz"})
	assert.NotPanics(t, func() { lrange(msg) }, "lrange should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, `[bar, baz]`, string(reply.Payload.([]byte)), "lrange should return correct value")
	default:
		assert.Fail(t, "lrange should reply a dipper message")
	}

	mock.ClearExpect()

	msg2 := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "foo2",
		},
		Reply: make(chan dipper.Message, 1),
	}
	mock.ExpectLRange("foo2", 0, -1).SetVal([]string{})
	assert.NotPanics(t, func() { lrange(msg2) }, "lrange should not panic with good data")
	select {
	case reply := <-msg2.Reply:
		assert.Equal(t, "[]", string(reply.Payload.([]byte)), "lrange should return correct value")
	default:
		assert.Fail(t, "lrange should reply a dipper message")
	}
}

func TestBLpop(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{
		Client: db,
	}

	assert.Panics(t, func() { blpop(&dipper.Message{}) }, "blpop should panic with empty request")

	msg := &dipper.Message{
		Labels: map[string]string{
			"timeout": "1s",
		},
		Payload: map[string]interface{}{
			"key": "foo",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectBLPop(time.Second, "foo").SetVal([]string{"foo", "bar"})
	assert.NotPanics(t, func() { blpop(msg) }, "blpop should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Equal(t, `bar`, string(reply.Payload.([]byte)), "blpop should return correct value")
	default:
		assert.Fail(t, "blpop should reply a dipper message")
	}

	mock.ClearExpect()

	msg2 := &dipper.Message{
		Labels: map[string]string{
			"timeout": "0.5s",
		},
		Payload: map[string]interface{}{
			"key": "foo2",
		},
		Reply: make(chan dipper.Message, 1),
	}
	mock.ExpectBLPop(time.Second, "foo2").SetVal([]string{})
	assert.NotPanics(t, func() { blpop(msg2) }, "blpop should not panic with good data")
	select {
	case reply := <-msg2.Reply:
		assert.Equal(t, "", string(reply.Payload.([]byte)), "blpop should return correct value")
	default:
		assert.Fail(t, "blpop should reply a dipper message")
	}
}

func TestDel(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{
		Client: db,
	}

	assert.Panics(t, func() { del(&dipper.Message{}) }, "del should panic with empty request")

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "foo",
		},
		Reply: make(chan dipper.Message, 1),
	}

	mock.ExpectDel("foo").SetVal(1)
	assert.NotPanics(t, func() { del(msg) }, "del should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.Nil(t, reply.Payload, "del should return correct value")
	default:
		assert.Fail(t, "del should reply a dipper message")
	}

	mock.ClearExpect()

	msg2 := &dipper.Message{
		Payload: map[string]interface{}{
			"key": "foo2",
		},
		Reply: make(chan dipper.Message, 1),
	}
	mock.ExpectDel("foo2").SetVal(0)
	assert.NotPanics(t, func() { del(msg2) }, "del should not panic with good data")
	select {
	case reply := <-msg2.Reply:
		assert.Nil(t, reply.Payload, "del should return correct value")
	default:
		assert.Fail(t, "del should reply a dipper message")
	}

	mock.ClearExpect()

	msg3 := &dipper.Message{
		Payload: map[string]interface{}{
			"key": []string{"foo_fail"},
		},
		Reply: make(chan dipper.Message, 1),
	}
	mock.ExpectDel("foo_fail").SetErr(errors.New("something is wrong"))
	assert.Panics(t, func() { del(msg3) }, "del should panic with bad data")
	select {
	case <-msg2.Reply:
		assert.Fail(t, "del panic when error is returned")
	default:
	}
}

func TestExists(t *testing.T) {
	if driver == nil {
		TestLoadOptions(t)
	}

	db, mock := redismock.NewClientMock()
	redisOptions = &redisclient.Options{
		Client: db,
	}

	assert.Panics(t, func() { exists(&dipper.Message{}) }, "exists should panic with empty request")

	msg := &dipper.Message{
		Payload: []byte("foo"),
		Reply:   make(chan dipper.Message, 1),
	}

	mock.ExpectExists("foo").SetVal(1)
	assert.NotPanics(t, func() { exists(msg) }, "exists should not panic with good data")
	select {
	case reply := <-msg.Reply:
		assert.NotEmpty(t, reply.Payload, "exists should return correct value")
	default:
		assert.Fail(t, "exists should reply a dipper message")
	}

	mock.ClearExpect()

	msg2 := &dipper.Message{
		Payload: []byte("foo2"),
		Reply:   make(chan dipper.Message, 1),
	}
	mock.ExpectExists("foo2").SetVal(0)
	assert.NotPanics(t, func() { exists(msg2) }, "exists should not panic with good data")
	select {
	case reply := <-msg2.Reply:
		assert.Nil(t, reply.Payload, "exists should return correct value")
	default:
		assert.Fail(t, "exists should reply a dipper message")
	}
}
