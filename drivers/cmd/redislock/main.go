// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redispubsub enables Honeydipper to use redis to broadcast internal messages.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/honeydipper/honeydipper/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// DefaultPrefix is the prefix used for naming the locking topic.
const DefaultPrefix = "lock:"

var (
	// ErrFailToLock means not being able to acquire the lock.
	ErrFailToLock = errors.New("fail to lock")
	// ErrFailToUnlock means not being able to unlock.
	ErrFailToUnlock = errors.New("fail to unlock")
)

// Locker holds the driver, configurations and runtime information.
type Locker struct {
	driver       *dipper.Driver
	redisOptions *redis.Options
	prefix       string
	nodeID       string
}

func (l *Locker) start(msg *dipper.Message) {
	l.loadOptions(msg)
}

func (l *Locker) loadOptions(msg *dipper.Message) {
	l.redisOptions = redisclient.GetRedisOpts(l.driver)
	var ok bool
	if l.prefix, ok = dipper.GetMapDataStr(l.driver.Options, "data.prefix"); !ok {
		l.prefix = DefaultPrefix
	}
}

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc\n")
		fmt.Printf("  This program provides honeydipper with capability of acquiring locks\n")
	}
}

func main() {
	initFlags()
	flag.Parse()
	l := Locker{}
	l.driver = dipper.NewDriver(os.Args[1], "redislock")
	l.driver.Start = l.start
	l.driver.Reload = l.loadOptions
	l.driver.RPCHandlers["lock"] = l.lock
	l.driver.RPCHandlers["unlock"] = l.unlock
	l.driver.Run()
	l.nodeID = dipper.GetIP()
}

func (l *Locker) lock(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	expire := dipper.Must(time.ParseDuration(dipper.MustGetMapDataStr(msg.Payload, "expire"))).(time.Duration)
	name := dipper.MustGetMapDataStr(msg.Payload, "name")

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if attemptMsStr, ok := dipper.GetMapDataStr(msg.Payload, "attempt_ms"); ok {
		attemptMs := dipper.Must(strconv.Atoi(attemptMsStr)).(int)
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(attemptMs)*time.Millisecond)
	} else {
		ctx, cancel = l.driver.GetContext()
	}
	defer cancel()

	client := redis.NewClient(l.redisOptions)
	defer client.Close()

	ok := dipper.Must(client.SetNX(ctx, l.prefix+name, l.nodeID, expire).Result()).(bool)
	if !ok {
		panic(ErrFailToLock)
	}

	msg.Reply <- dipper.Message{}
}

func (l *Locker) unlock(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	name := dipper.MustGetMapDataStr(msg.Payload, "name")

	ctx, cancel := l.driver.GetContext()
	defer cancel()

	client := redis.NewClient(l.redisOptions)
	defer client.Close()

	ok := dipper.Must(client.Del(ctx, l.prefix+name).Result()).(int64) > 0
	if !ok {
		panic(ErrFailToUnlock)
	}

	msg.Reply <- dipper.Message{}
}
