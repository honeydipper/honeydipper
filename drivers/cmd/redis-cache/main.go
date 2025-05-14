// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redis-cache enables Honeydipper to use redis as a temporary
// external cache storage.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	driver.RPCHandlers["incr"] = incr
	driver.RPCHandlers["lrange"] = lrange
	driver.RPCHandlers["blpop"] = blpop
	driver.RPCHandlers["rpush"] = rpush
	driver.RPCHandlers["del"] = del
	driver.RPCHandlers["exists"] = exists
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
			Payload: []byte(val),
			IsRaw:   true,
		}
	}
}

func lrange(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	start, _ := dipper.GetMapDataInt(msg.Payload, "start")
	raw, _ := dipper.GetMapDataBool(msg.Payload, "raw")
	del, _ := dipper.GetMapDataBool(msg.Payload, "del")

	stop := -1
	if _, ok := msg.Payload.(map[string]any)["stop"]; ok {
		stop = dipper.MustGetMapDataInt(msg.Payload, "stop")
	}

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	val, err := client.LRange(ctx, key, int64(start), int64(stop)).Result()
	switch {
	case errors.Is(err, redis.Nil):
		msg.Reply <- dipper.Message{}
	case err != nil:
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	default:
		var buf string
		if raw {
			buf = strings.Join(val, "")
		} else {
			buf = "[" + strings.Join(val, ", ") + "]"
		}
		msg.Reply <- dipper.Message{
			Payload: []byte(buf),
			IsRaw:   true,
		}

		if del {
			_, e := client.Del(ctx, key).Result()
			if e != nil {
				dipper.Logger.Warningf("[%s] redis error deleting: %v", driver.Service, e)
			}
		}
	}
}

func incr(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	val, err := client.Incr(ctx, key).Result()
	switch {
	case errors.Is(err, redis.Nil):
		msg.Reply <- dipper.Message{}
	case err != nil:
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	default:
		msg.Reply <- dipper.Message{
			Payload: []byte(strconv.Itoa(int(val))),
			IsRaw:   true,
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

func rpush(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	val := dipper.MustGetMapData(msg.Payload, "value")
	toJSON, _ := dipper.GetMapDataBool(msg.Payload, "toJson")
	if toJSON {
		val = string(dipper.Must(json.Marshal(val)).([]byte))
	}

	var ttl time.Duration
	ttlData, _ := dipper.GetMapData(msg.Payload, "ttl")
	if ttlData != nil {
		ttl = time.Duration(ttlData.(float64))
	}

	valStr, ok := val.(string)
	if !ok {
		valStr = string(dipper.Must(json.Marshal(val)).([]byte))
	}

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	if err := client.RPush(ctx, key, valStr).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	if ttl > 0 {
		if err := client.Expire(ctx, key, ttl).Err(); err != nil && !errors.Is(err, redis.Nil) {
			log.Panicf("[%s] redis error: %v", driver.Service, err)
		}
	}
	msg.Reply <- dipper.Message{}
}

func blpop(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	timeoutDuration := time.Duration(0)
	timeout := msg.Labels["timeout"]
	if timeout != "" {
		timeoutDuration = dipper.Must(time.ParseDuration(timeout)).(time.Duration)
		if timeoutDuration > time.Second*2 {
			timeoutDuration -= time.Second * 2
		}
	}

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()
	val, err := client.BLPop(ctx, timeoutDuration, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) && !errors.Is(err, context.DeadlineExceeded) {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	ret := ""
	if len(val) > 1 {
		ret = val[1]
	}
	msg.Reply <- dipper.Message{
		Payload: []byte(ret),
		IsRaw:   true,
	}
}

func del(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	if err := client.Del(ctx, key).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	msg.Reply <- dipper.Message{}
}

func exists(msg *dipper.Message) {
	key := string(msg.Payload.([]byte))

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	found := int(dipper.Must(client.Exists(ctx, key).Result()).(int64))
	var payload []byte
	dipper.Logger.Debugf("[%s] redis cache exists: %s %d", driver.Service, key, found)
	if found > 0 {
		payload = []byte{1}
	}
	msg.Reply <- dipper.Message{Payload: payload, IsRaw: true}
}
