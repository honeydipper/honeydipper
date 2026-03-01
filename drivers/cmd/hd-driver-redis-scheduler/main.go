// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redis-cache enables Honeydipper to use redis as a temporary
// external cache storage.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/honeydipper/honeydipper/v4/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/op/go-logging"
)

const SCHEDULEKEY = "honeydipper:schedule"

var (
	log          *logging.Logger
	driver       *dipper.Driver
	redisOptions *redisclient.Options
	reloadSignal chan bool
	scheduleKey  = SCHEDULEKEY
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc\n")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis as a scheduler.\n")
	}
}

func main() {
	initFlags()
	flag.Parse()
	driver = dipper.NewDriver(os.Args[1], "redis-scheduler")
	driver.Start = start
	driver.Reload = reload
	driver.Stop = stop
	driver.RPCHandlers["once"] = scheduleOnce
	driver.RPCHandlers["cancel"] = cancelSchedule
	driver.Run()
}

func loadOptions() {
	log = driver.GetLogger()
	redisOptions = redisclient.GetRedisOpts(driver)
	scheduleKey = SCHEDULEKEY
	if k, ok := driver.GetOptionStr("schedule_key"); ok && len(k) > 0 {
		scheduleKey = k
	}
	log.Infof("[%s] receiving driver data %+v", driver.Service, driver.Options)
}

func watch() {
	for {
		func() {
			dipper.SafeExitOnError("Error processing priority set for scheduler.")
			dipper.IgnoreError(context.Canceled)
			client := redisclient.NewClient(redisOptions)
			defer client.Close()
			ctx, cancel := driver.GetContext(nil)
			defer cancel()

			now := float64(time.Now().UnixMicro())
			ret := dipper.Must(client.Eval(ctx, `
				local vals = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, '-1')
				if #vals == 0 then
					return {}
				end
				redis.call('ZREM', KEYS[1], unpack(vals))
				return vals
			`, []string{scheduleKey}, now).StringSlice()).([]string)

			for _, member := range ret {
				splitAt := strings.Index(member, ":")
				if splitAt == -1 {
					log.Warningf("[%s] invalid member format: %s", driver.Service, member)

					continue
				}
				itemType := member[:splitAt]

				payload := dipper.Must(client.Eval(context.Background(), `
					local x = redis.call('GET', KEYS[1])
					redis.call('DEL', KEYS[1])
					 return x
				`, []string{scheduleKey + ":payload:" + member}).Result()).(string)

				msg := &dipper.Message{}
				dipper.Must(json.Unmarshal([]byte(payload), msg))
				msg.Channel = "scheduler"
				msg.Subject = itemType

				driver.SendMessage(msg)
			}
		}()

		if driver.State != dipper.DriverStateAlive {
			return
		}

		select {
		case <-reloadSignal:
			return
		case <-time.After(time.Second):
		}
	}
}

func start(msg *dipper.Message) {
	loadOptions()
	reloadSignal = make(chan bool, 1)
	go watch()
}

func stop(msg *dipper.Message) {
	reloadSignal <- true
	close(reloadSignal)
}

func reload(msg *dipper.Message) {
	stop(msg)
	start(msg)
}

func scheduleOnce(msg *dipper.Message) {
	dipper.DeserializePayload(msg)

	dueTime := dipper.MustGetMapDataStr(msg.Payload, "due")
	due := dipper.Must(strconv.ParseInt(dueTime, 10, 64)).(int64)

	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	itemType := dipper.MustGetMapDataStr(msg.Payload, "type")
	member := fmt.Sprintf("%s:%s", itemType, key)

	dueMsg := &dipper.Message{}
	data := dipper.MustGetMapData(msg.Payload, "due_message")
	dipper.Must(mapstructure.Decode(data, dueMsg))
	payload := string(dipper.Must(json.Marshal(dueMsg)).([]byte))

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	dipper.Must(client.Set(ctx, scheduleKey+":payload:"+member,
		payload,
		0,
	).Err())
	dipper.Must(client.ZAdd(ctx, scheduleKey, &redis.Z{
		Score:  float64(due),
		Member: member,
	}).Result())

	msg.Reply <- dipper.Message{}
}

func cancelSchedule(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	key := dipper.MustGetMapDataStr(msg.Payload, "key")
	itemType := dipper.MustGetMapDataStr(msg.Payload, "type")
	member := fmt.Sprintf("%s:%s", itemType, key)

	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	dipper.Must(client.ZRem(ctx, scheduleKey, member).Result())
	payload := dipper.Must(client.Eval(ctx, `
		local x = redis.call('GET', KEYS[1])
		redis.call('DEL', KEYS[1])
			return x
	`, []string{scheduleKey + ":payload:" + member}).Result()).(string)

	msg.Reply <- dipper.Message{
		Payload: []byte(payload),
		IsRaw:   true,
	}
}
