// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redis-cache enables Honeydipper to use redis as a temporary
// external cache storage.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/honeydipper/honeydipper/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
)

var (
	log          *logging.Logger
	driver       *dipper.Driver
	redisOptions *redisclient.Options
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc\n")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis as a temporary external storage\n")
	}
}

func main() {
	initFlags()
	flag.Parse()
	driver = dipper.NewDriver(os.Args[1], "redis-cache")
	driver.Start = start
	driver.RPCHandlers["save"] = save
	driver.RPCHandlers["load"] = load
	driver.Run()
}

func loadOptions() {
	log = driver.GetLogger()
	redisOptions = redisclient.GetRedisOpts(driver)
	log.Infof("[%s] receiving driver data %+v", driver.Service, driver.Options)
}

func start(msg *dipper.Message) {
	loadOptions()
}

func load(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	val, err := client.Get(ctx, key).Result()
	switch {
	case errors.Is(err, redis.Nil):
		msg.Reply <- dipper.Message{}
	case err != nil:
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	default:
		msg.Reply <- dipper.Message{
			Payload: map[string]interface{}{
				"value": val,
			},
		}
	}
}

func save(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	val := dipper.MustGetMapData(msg.Payload, "value")
	ttl, _ := dipper.GetMapData(msg.Payload, "ttl")

	var exp time.Duration
	if ttl != nil {
		switch t := ttl.(type) {
		case int64:
			exp = time.Second * time.Duration(t)
		case int:
			exp = time.Second * time.Duration(t)
		case string:
			exp = dipper.Must(time.ParseDuration(t)).(time.Duration)
		default:
			log.Panicf("[%s] redis cache unknown TTL type %+v", driver.Service, t)
		}
	}

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	if err := client.Set(ctx, key, val, exp).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	msg.Reply <- dipper.Message{}
}
