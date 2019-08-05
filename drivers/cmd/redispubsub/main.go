// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package redispubsub enables Honeydipper to use redis to broadcast internal messages.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
)

var log *logging.Logger
var driver *dipper.Driver
var redisOptions *redis.Options
var broadcastTopic string
var ok bool
var err error

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc\n")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis as pub/sub\n")
	}
}

func main() {
	initFlags()
	flag.Parse()
	driver = dipper.NewDriver(os.Args[1], "redispubsub")
	driver.Start = start
	if driver.Service == "operator" {
		driver.CommandProvider.Commands["send"] = broadcastToRedis
	}
	driver.Run()
}

func loadOptions() {
	log = driver.GetLogger()
	log.Infof("[%s] receiving driver data %+v", driver.Service, driver.Options)

	broadcastTopic, ok = driver.GetOptionStr("data.topics.broadcast")
	if !ok {
		broadcastTopic = "honeydipper:broadcast"
	}

	opts := &redis.Options{}
	if localRedis, ok := os.LookupEnv("LOCALREDIS"); ok && localRedis != "" {
		opts.Addr = "127.0.0.1:6379"
		opts.DB = 0
	} else {
		if value, ok := driver.GetOptionStr("data.connection.Addr"); ok {
			opts.Addr = value
		}
		if value, ok := driver.GetOptionStr("data.connection.Password"); ok {
			opts.Password = value
		}
		if DB, ok := driver.GetOptionStr("data.connection.DB"); ok {
			DBnum, err := strconv.Atoi(DB)
			if err != nil {
				log.Panicf("[%s] invalid db number %s", driver.Service, DB)
			}
			opts.DB = DBnum
		}
	}
	redisOptions = opts
}

func start(msg *dipper.Message) {
	loadOptions()

	go subscribe()
}

func broadcastToRedis(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	payload := map[string]interface{}{
		"labels": msg.Labels,
	}
	if msg.Payload != nil {
		payload["data"] = msg.Payload
	}
	buf := dipper.SerializeContent(payload)
	client := redis.NewClient(redisOptions)
	defer client.Close()
	if err := client.Publish(broadcastTopic, string(buf)).Err(); err != nil {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	msg.Reply <- dipper.Message{}
}

func subscribe() {
	for {
		func() {
			defer dipper.SafeExitOnError("[%s] re-subscribing to redis pubsub %s", driver.Service, broadcastTopic)
			client := redis.NewClient(redisOptions)
			defer client.Close()
			pubsub := client.Subscribe(broadcastTopic)

			_, err = pubsub.Receive()
			if err != nil {
				panic(err)
			}

			ch := pubsub.Channel()
			for msg := range ch {
				payload := dipper.DeserializeContent([]byte(msg.Payload))
				labels := map[string]string{}
				labelMap, ok := dipper.GetMapData(payload, "labels")
				if ok {
					for k, v := range labelMap.(map[string]interface{}) {
						labels[k] = v.(string)
					}
				}
				data, _ := dipper.GetMapData(payload, "data")
				driver.SendMessage(&dipper.Message{
					Channel: "broadcast",
					Subject: dipper.MustGetMapDataStr(data, "broadcastSubject"),
					Payload: data,
					Labels:  labels,
				})
			}
		}()
		time.Sleep(time.Second)
	}
}
