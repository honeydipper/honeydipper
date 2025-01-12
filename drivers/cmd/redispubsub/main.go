// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redispubsub enables Honeydipper to use redis to broadcast internal messages.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/honeydipper/honeydipper/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
)

var (
	log              *logging.Logger
	driver           *dipper.Driver
	redisOptions     *redisclient.Options
	broadcastTopic   string
	broadcastChannel string
	ok               bool
	err              error
)

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
	driver.RPCHandlers["send"] = sendBroadcast
	if driver.Service == "operator" {
		driver.Commands["send"] = broadcastToRedis
	}
	driver.Run()
}

func loadOptions() {
	log = driver.GetLogger()
	redisOptions = redisclient.GetRedisOpts(driver)
	log.Infof("[%s] receiving driver data %+v", driver.Service, driver.Options)

	broadcastTopic, ok = driver.GetOptionStr("data.topic")
	if !ok {
		broadcastTopic = "honeydipper:broadcast"
	}
	broadcastChannel, ok = driver.GetOptionStr("data.channel")
	if !ok {
		broadcastChannel = "broadcast"
	}
}

func start(msg *dipper.Message) {
	loadOptions()

	go subscribe()
}

func broadcastToRedis(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	labels := msg.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labels["from"] = dipper.GetIP()
	payload := map[string]interface{}{
		"labels":           labels,
		"broadcastSubject": dipper.MustGetMapDataStr(msg.Payload, "broadcastSubject"),
	}
	if data, ok := dipper.GetMapData(msg.Payload, "data"); ok && data != nil {
		payload["data"] = data
	}
	buf := dipper.SerializeContent(payload)
	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	if err := client.Publish(ctx, broadcastTopic, string(buf)).Err(); err != nil {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	msg.Reply <- dipper.Message{}
}

func sendBroadcast(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	labels, ok := dipper.GetMapData(msg.Payload, "labels")
	if !ok || labels == nil {
		labels = map[string]interface{}{}
	}
	labels.(map[string]interface{})["from"] = dipper.GetIP()
	payload := map[string]interface{}{
		"labels":           labels,
		"broadcastSubject": dipper.MustGetMapDataStr(msg.Payload, "broadcastSubject"),
	}
	if data, ok := dipper.GetMapData(msg.Payload, "data"); ok && data != nil {
		payload["data"] = data
	}
	buf := dipper.SerializeContent(payload)
	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	if err := client.Publish(ctx, broadcastTopic, string(buf)).Err(); err != nil {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	msg.Reply <- dipper.Message{}
}

func subscribe() {
	for {
		func() {
			defer dipper.SafeExitOnError("[%s] re-subscribing to redis pubsub %s", driver.Service, broadcastTopic)
			client := redisclient.NewClient(redisOptions)
			defer client.Close()
			var pubsub *redis.PubSub
			ctx, cancel := driver.GetContext()
			func() {
				defer cancel()
				pubsub = client.Subscribe(ctx, broadcastTopic)
			}()

			_, err = pubsub.Receive(context.Background())
			if err != nil {
				panic(err)
			}

			ch := pubsub.Channel()
			for msg := range ch {
				payload := dipper.DeserializeContent([]byte(msg.Payload))
				labels := map[string]string{}
				skip := false
				labelMap, ok := dipper.GetMapData(payload, "labels")
				if ok {
					for k, v := range labelMap.(map[string]interface{}) {
						if k == "service" && v != nil && v.(string) != "" && v.(string) != driver.Service {
							skip = true

							break
						}
						labels[k] = v.(string)
					}
					if skip {
						continue
					}
				}
				data, _ := dipper.GetMapData(payload, "data")
				driver.SendMessage(&dipper.Message{
					Channel: broadcastChannel,
					Subject: dipper.MustGetMapDataStr(payload, "broadcastSubject"),
					Payload: data,
					Labels:  labels,
				})
			}
		}()
		time.Sleep(time.Second)
	}
}
