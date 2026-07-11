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
	"github.com/honeydipper/honeydipper/v4/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
)

const (
	incrScript = `
        local remove_zero = tonumber(ARGV[1])
        local current_value = redis.call("INCR", KEYS[1])

        if current_value == 0 and remove_zero > 0 then
            redis.call("DEL", KEYS[1])
        end
        
		return current_value
	`
	decrScript = `
        local remove_zero = tonumber(ARGV[1])
        local current_value = redis.call("DECR", KEYS[1])

        if current_value == 0 and remove_zero > 0 then
            redis.call("DEL", KEYS[1])
        end
        
		return current_value
	`
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
	driver.RPCHandlers["decr"] = decr
	driver.RPCHandlers["lrange"] = lrange
	driver.RPCHandlers["blpop"] = blpop
	driver.RPCHandlers["rpush"] = rpush
	driver.RPCHandlers["ltrim"] = ltrim
	driver.RPCHandlers["del"] = del
	driver.RPCHandlers["exists"] = exists
	driver.RPCHandlers["scan"] = scan
	driver.RPCHandlers["expire"] = expire
	driver.RPCHandlers["hset"] = hset
	driver.RPCHandlers["hvals"] = hvals
	driver.RPCHandlers["hmget"] = hmget
	driver.RPCHandlers["stream_hset"] = streamHset
	driver.RPCHandlers["stream_hvals"] = streamHvals
	driver.Run()
}

func loadOptions() {
	log = driver.GetLogger()
	redisOptions = redisclient.GetRedisOpts(driver)
	log.Infof("[%s] receiving driver data %+v", driver.Service, driver.Options)

	// Load stream_hset configuration from driver options
	loadStreamConfig()
}

func start(msg *dipper.Message) {
	loadOptions()
}

func scan(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	pattern := dipper.MustGetMapDataStr(msg.Payload, "pattern")
	count := int64(100)
	if v, ok := dipper.GetMapDataInt(msg.Payload, "count"); ok && v > 0 {
		count = int64(v)
	}
	var cursor uint64
	if v, ok := dipper.GetMapDataStr(msg.Payload, "cursor"); ok && v != "" {
		cursor = uint64(dipper.Must(strconv.Atoi(v)).(int))
	}

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	keys := []string{}
	for int64(len(keys)) < count {
		remaining := count - int64(len(keys))
		res := dipper.Must(client.Scan(ctx, cursor, pattern, remaining).Result()).([]any)
		keys = append(keys, res[0].([]string)...)
		cursor = res[1].(uint64)
		if cursor == 0 {
			break
		}
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]any{
			"keys":   keys,
			"cursor": cursor,
		},
	}
}

func load(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
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
	ctx, cancel := driver.GetContext(msg)
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
			buf = strings.Join(val, "\n")
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
	removeZero, _ := dipper.GetMapDataInt(msg.Payload, "remove_zero")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	val := dipper.Must(client.Eval(ctx, incrScript, []string{key}, removeZero).Result()).(int64)

	msg.Reply <- dipper.Message{
		Payload: []byte(strconv.Itoa(int(val))),
		IsRaw:   true,
	}
}

func decr(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	removeZero, _ := dipper.GetMapDataInt(msg.Payload, "remove_zero")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	val := dipper.Must(client.Eval(ctx, decrScript, []string{key}, removeZero).Result()).(int64)

	msg.Reply <- dipper.Message{
		Payload: []byte(strconv.Itoa(int(val))),
		IsRaw:   true,
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
	ctx, cancel := driver.GetContext(msg)
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
		switch t := ttlData.(type) {
		case int64:
			ttl = time.Second * time.Duration(t)
		case int:
			ttl = time.Second * time.Duration(t)
		case float64:
			ttl = time.Duration(t)
		case string:
			ttl = dipper.Must(time.ParseDuration(t)).(time.Duration)
		default:
			log.Panicf("[%s] redis cache unknown TTL type %+v", driver.Service, t)
		}
	}

	valStr, ok := val.(string)
	if !ok {
		valStr = string(dipper.Must(json.Marshal(val)).([]byte))
	}

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
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

func expire(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")

	var ttl time.Duration
	ttlData, _ := dipper.GetMapData(msg.Payload, "ttl")
	if ttlData != nil {
		ttl = time.Duration(ttlData.(float64))
	}

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	if err := client.Expire(ctx, key, ttl).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
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

func ltrim(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	start := dipper.MustGetMapDataInt(msg.Payload, "start")
	stop := dipper.MustGetMapDataInt(msg.Payload, "stop")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()
	if err := client.LTrim(ctx, key, int64(start), int64(stop)).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	msg.Reply <- dipper.Message{}
}

func del(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
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
	ctx, cancel := driver.GetContext(msg)
	defer cancel()
	found := int(dipper.Must(client.Exists(ctx, key).Result()).(int64))
	var payload []byte
	dipper.Logger.Debugf("[%s] redis cache exists: %s %d", driver.Service, key, found)
	if found > 0 {
		payload = []byte{1}
	}
	msg.Reply <- dipper.Message{Payload: payload, IsRaw: true}
}
