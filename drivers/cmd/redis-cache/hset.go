// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redis-cache enables Honeydipper to use redis as a temporary
// external cache storage.
package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/honeydipper/honeydipper/v4/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// streamHsetIntervalHours is the time interval in hours for stream_hset blocks.
// Configurable via driver option "data.stream_interval_hours".
var streamHsetIntervalHours = 2

// streamHsetTTLHours is the TTL in hours for stream_hset data.
// Controls how far back conversation history can be retrieved.
// Default: 336 hours (2 weeks). Configurable via driver option "data.stream_ttl_hours".
var streamHsetTTLHours = 336

// perFieldExpiration controls how TTL is applied to hash fields saved via the
// hset handler. When true, each field gets its own expiration through per-field
// HEXPIRE (requires Redis >= 7.4). When false (the default), the whole hash key
// is given a single shared TTL via EXPIRE, which is compatible with older Redis
// versions.
//
// Configurable via driver option "data.per_field_expiration" (boolean).
var perFieldExpiration = false

const (
	streamHvalsScript = `
	    local sessions = {'"'..KEYS[1]..'"', '"'..KEYS[#KEYS]..'"'}

		for i, key in ipairs(KEYS) do
			local parts = redis.call("HVALS", key)
			for _, item in ipairs(parts) do
				table.insert(sessions, item)
			end
		end

		return sessions
	`
	hexpireScript = `
		local ttl = ARGV[1]
		local field_count = ARGV[2]
		local fields = {}
		for i = 1, tonumber(field_count) do
			fields[i] = ARGV[i+2]
		end
		return redis.call("HEXPIRE", KEYS[1], ttl, "FIELDS", field_count, unpack(fields))
	`
)

// loadStreamConfig loads stream_hset configuration from driver options.
func loadStreamConfig() {
	if driver.Options == nil {
		return
	}

	if v, ok := dipper.GetMapData(driver.Options, "data.stream_ttl_hours"); ok {
		switch t := v.(type) {
		case int64:
			streamHsetTTLHours = int(t)
		case int:
			streamHsetTTLHours = t
		case float64:
			streamHsetTTLHours = int(t)
		default:
			log.Warningf("[%s] redis cache invalid stream_ttl_hours type %T, using default", driver.Service, t)
		}
	}

	if v, ok := dipper.GetMapData(driver.Options, "data.stream_interval_hours"); ok {
		switch t := v.(type) {
		case int64:
			streamHsetIntervalHours = int(t)
		case int:
			streamHsetIntervalHours = t
		case float64:
			streamHsetIntervalHours = int(t)
		default:
			log.Warningf("[%s] redis cache invalid stream_interval_hours type %T, using default", driver.Service, t)
		}
	}

	log.Infof("[%s] stream_hset: interval=%dh, ttl=%dh (default: 336h=2w)",
		driver.Service, streamHsetIntervalHours, streamHsetTTLHours)
}

// loadHashConfig loads hash-related configuration from driver options.
func loadHashConfig() {
	if driver.Options == nil {
		return
	}

	if v, ok := dipper.GetMapData(driver.Options, "data.per_field_expiration"); ok {
		switch t := v.(type) {
		case bool:
			perFieldExpiration = t
		default:
			log.Warningf("[%s] redis cache invalid per_field_expiration type %T, using default", driver.Service, t)
		}
	}

	log.Infof("[%s] hash per_field_expiration: %v", driver.Service, perFieldExpiration)
}

func streamHset(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	prefix := dipper.MustGetMapDataStr(msg.Payload, "prefix")
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	val := dipper.MustGetMapDataStr(msg.Payload, "value")
	ttl, _ := dipper.GetMapData(msg.Payload, "ttl")

	exp := time.Duration(streamHsetTTLHours) * time.Hour
	if ttl != nil {
		switch t := ttl.(type) {
		case int64:
			exp = time.Second * time.Duration(t)
		case int:
			exp = time.Second * time.Duration(t)
		case float64:
			exp = time.Second * time.Duration(int64(t))
		case string:
			exp = dipper.Must(time.ParseDuration(t)).(time.Duration)
		default:
			log.Panicf("[%s] redis cache unknown TTL type %+v", driver.Service, t)
		}
	}

	curr := time.Now().Truncate(time.Hour)
	if df := curr.Hour() % streamHsetIntervalHours; df != 0 {
		curr = curr.Add(time.Duration(-df) * time.Hour)
	}

	setName := prefix + curr.Format("2006010215")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	if err := client.HSet(ctx, setName, []string{key, val}).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}

	dipper.Must(client.Expire(ctx, setName, exp).Err())

	msg.Reply <- dipper.Message{}
}

func streamHvals(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	prefix := dipper.MustGetMapDataStr(msg.Payload, "prefix")
	lookBack, _ := dipper.GetMapData(msg.Payload, "look_back")
	asOf, _ := dipper.GetMapDataStr(msg.Payload, "asOf")
	raw, _ := dipper.GetMapDataBool(msg.Payload, "raw")

	end := time.Now().Truncate(time.Hour)
	oldest := end.Add(-time.Duration(streamHsetTTLHours) * time.Hour)
	if df := oldest.Hour() % streamHsetIntervalHours; df != 0 {
		oldest = oldest.Add(time.Duration(-df) * time.Hour)
	}

	if asOf != "" {
		end = dipper.Must(time.ParseInLocation("2006010215", asOf, oldest.Location())).(time.Time)
	}
	if df := end.Hour() % streamHsetIntervalHours; df != 0 {
		end = end.Add(time.Duration(-df) * time.Hour)
	}

	if end.Before(oldest) {
		end = oldest
	}

	earliest := end

	if lookBack != nil {
		blocks := 1
		switch t := lookBack.(type) {
		case int64:
			blocks = int(t)
		case int:
			blocks = t
		case float64:
			blocks = int(t)
		case string:
			blocks = dipper.Must(strconv.Atoi(t)).(int)
		default:
			log.Panicf("[%s] redis cache unknown look_back type %+v", driver.Service, t)
		}
		if blocks < 0 {
			blocks = 0
		}
		earliest = end.Add(-time.Duration(blocks*streamHsetIntervalHours) * time.Hour)
		if earliest.Before(oldest) {
			earliest = oldest
		}
	}

	size := int(end.Sub(earliest).Hours()/float64(streamHsetIntervalHours)) + 1
	keys := make([]string, size)
	for i := 0; i < size; i++ {
		t := earliest.Add(time.Duration(i*streamHsetIntervalHours) * time.Hour)
		keys[i] = prefix + t.Format("2006010215")
	}

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	ret := dipper.Must(client.Eval(ctx, streamHvalsScript, keys).Result())
	var result []string
	switch items := ret.(type) {
	case []string:
		result = items
	case []interface{}:
		result = make([]string, 0, len(items))
		for _, item := range items {
			result = append(result, fmt.Sprint(item))
		}
	default:
		log.Panicf("[%s] redis cache unexpected stream_hvals return type %T", driver.Service, ret)
	}

	buf := ""
	if raw {
		buf = strings.Join(result, "\n")
	} else {
		buf = "[" + strings.Join(result, ",") + "]"
	}

	msg.Reply <- dipper.Message{
		Payload: []byte(buf),
		IsRaw:   true,
	}
}

func hset(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	val := dipper.MustGetMapData(msg.Payload, "value")
	ttl, _ := dipper.GetMapData(msg.Payload, "ttl")
	fields := []string{}
	switch f := val.(type) {
	case map[string]interface{}:
		for name := range f {
			fields = append(fields, name)
		}
	case map[string]string:
		for name := range f {
			fields = append(fields, name)
		}
	case []interface{}:
		for i := 0; i < len(f); i += 2 {
			if name, ok := f[i].(string); ok {
				fields = append(fields, name)
			}
		}
	case []string:
		for i := 0; i < len(f); i += 2 {
			fields = append(fields, f[i])
		}
	}

	exp := 24 * time.Hour
	if ttl != nil {
		switch t := ttl.(type) {
		case int64:
			exp = time.Second * time.Duration(t)
		case int:
			exp = time.Second * time.Duration(t)
		case float64:
			exp = time.Second * time.Duration(int64(t))
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

	if err := client.HSet(ctx, key, val).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	if len(fields) > 0 {
		applyHashExpiration(ctx, client, key, exp, fields)
	}

	msg.Reply <- dipper.Message{}
}

// applyHashExpiration sets the TTL for a hash key written by hset. When
// perFieldExpiration is enabled (Redis >= 7.4), each field receives its own
// expiration via per-field HEXPIRE (executed through a Lua script so expired
// fields are auto-removed by Redis). Otherwise the whole hash key is given a
// single shared TTL via EXPIRE, which works on Redis versions older than 7.4.
func applyHashExpiration(ctx context.Context, client redisclient.Options, key string, exp time.Duration, fields []string) {
	if perFieldExpiration {
		ttlSeconds := int64(exp / time.Second)
		args := make([]interface{}, 2+len(fields))
		args[0] = ttlSeconds
		args[1] = len(fields)
		for i, field := range fields {
			args[2+i] = field
		}
		if _, err := client.Eval(ctx, hexpireScript, []string{key}, args...).Result(); err != nil && !errors.Is(err, redis.Nil) {
			log.Panicf("[%s] redis error: %v", driver.Service, err)
		}

		return
	}

	if err := client.Expire(ctx, key, exp).Err(); err != nil && !errors.Is(err, redis.Nil) {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
}

// hget performs HGET key field and returns the raw field value. It returns an
// empty (nil) payload on a cache miss (redis.Nil) and panics on real errors,
// mirroring the behavior of the load handler.
func hget(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	field := dipper.MustGetMapDataStr(msg.Payload, "field")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	val, err := client.HGet(ctx, key, field).Result()
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

// hgetall performs HGETALL key and returns a map[string]any of field -> value
// (string). When per-field expiration is enabled, expired hash fields are
// auto-removed by Redis so they are naturally excluded from the result. It
// returns an empty (no error) payload on a cache miss.
func hgetall(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	val, err := client.HGetAll(ctx, key).Result()
	switch {
	case errors.Is(err, redis.Nil):
		msg.Reply <- dipper.Message{}
	case err != nil:
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	default:
		result := map[string]any{}
		for k, v := range val {
			result[k] = v
		}
		msg.Reply <- dipper.Message{
			Payload: result,
		}
	}
}

func hvals(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	raw, _ := dipper.GetMapDataBool(msg.Payload, "raw")

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	val, err := client.HVals(ctx, key).Result()
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
	}
}

func hmget(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	fieldsData := dipper.MustGetMapData(msg.Payload, "fields")
	raw, _ := dipper.GetMapDataBool(msg.Payload, "raw")

	var fields []string
	switch f := fieldsData.(type) {
	case []interface{}:
		for _, field := range f {
			if str, ok := field.(string); ok {
				fields = append(fields, str)
			}
		}
	case []string:
		fields = f
	}

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	val, err := client.HMGet(ctx, key, fields...).Result()
	switch {
	case errors.Is(err, redis.Nil):
		msg.Reply <- dipper.Message{}
	case err != nil:
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	default:
		// Convert interface{} results to strings
		valStrs := make([]string, len(val))
		i := 0
		for _, v := range val {
			if v != nil {
				valStrs[i] = v.(string)
				i++
			}
		}
		valStrs = valStrs[:i]
		var buf string
		if raw {
			buf = strings.Join(valStrs, "\n")
		} else {
			buf = "[" + strings.Join(valStrs, ", ") + "]"
		}
		msg.Reply <- dipper.Message{
			Payload: []byte(buf),
			IsRaw:   true,
		}
	}
}
