// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redisqueue enables Honeydipper to use redis queue as an eventbus.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/honeydipper/honeydipper/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
	"github.com/redis/go-redis/v9"
)

const (
	// TopicExpireTimeout is the timeout for a message in the topic to expire and be removed.
	TopicExpireTimeout time.Duration = time.Second * 1800
)

// EventBusOptions : stores all the redis key names used by honeydipper.
type EventBusOptions struct {
	EventTopic   string
	CommandTopic string
	ReturnTopic  string
	APITopic     string
}

var (
	log          *logging.Logger
	driver       *dipper.Driver
	eventbus     *EventBusOptions
	redisOptions *redisclient.Options
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc\n")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis as message queue\n")
	}
}

func main() {
	initFlags()
	flag.Parse()
	driver = dipper.NewDriver(os.Args[1], "redisqueue")
	driver.Start = start
	switch driver.Service {
	case "receiver":
		driver.MessageHandlers["eventbus:message"] = relayToRedis
	case "engine":
		driver.MessageHandlers["eventbus:command"] = relayToRedis
	case "operator":
		driver.MessageHandlers["eventbus:return"] = relayToRedis
	}
	driver.MessageHandlers["eventbus:api"] = relayToRedis
	driver.Run()
}

func loadOptions() {
	log = driver.GetLogger()
	redisOptions = redisclient.GetRedisOpts(driver)
	log.Infof("[%s] receiving driver data %+v", driver.Service, driver.Options)

	eb := &EventBusOptions{
		CommandTopic: "honeydipper:commands",
		EventTopic:   "honeydipper:events",
		ReturnTopic:  "honeydipper:return:",
		APITopic:     "honeydipper:api:",
	}
	if commandTopic, ok := driver.GetOptionStr("data.topics.command"); ok {
		eb.CommandTopic = commandTopic
	}
	if eventTopic, ok := driver.GetOptionStr("data.topics.event"); ok {
		eb.EventTopic = eventTopic
	}
	if returnTopic, ok := driver.GetOptionStr("data.topics.return"); ok {
		eb.ReturnTopic = returnTopic
	}
	if apiTopic, ok := driver.GetOptionStr("data.topics.api"); ok {
		eb.APITopic = apiTopic
	}
	eventbus = eb
}

func start(msg *dipper.Message) {
	loadOptions()
	switch driver.Service {
	case "engine":
		go subscribe(eventbus.EventTopic, "message")
		go subscribe(eventbus.ReturnTopic, "return")
	case "operator":
		go subscribe(eventbus.CommandTopic, "command")
	case "api":
		go subscribe(eventbus.APITopic, "api")
	case "receiver":
		client := redisclient.NewClient(redisOptions)
		defer client.Close()
		ctx, cancel := driver.GetContext()
		defer cancel()
		if err := client.Ping(ctx).Err(); err != nil {
			log.Panicf("[%s] redis error: %v", driver.Service, err)
		}
	}
}

func relayToRedis(msg *dipper.Message) {
	returnTo := msg.Labels["from"]
	msg.Labels["from"] = dipper.GetIP()
	topic := eventbus.EventTopic

	switch msg.Subject {
	case "command":
		topic = eventbus.CommandTopic
	case "api":
		topic = eventbus.APITopic + returnTo
		if returnTo == "" {
			log.Panicf("[%s] api return message without receipient", driver.Service)
		}
	case "return":
		if returnTo == "" {
			log.Panicf("[%s] return message without receipient", driver.Service)
		}
		topic = eventbus.ReturnTopic + returnTo
	}

	payload := map[string]interface{}{
		"labels": msg.Labels,
	}
	if msg.Payload != nil {
		payload["data"] = string(msg.Payload.([]byte))
	}
	buf := dipper.SerializeContent(payload)
	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext()
	defer cancel()
	if err := client.RPush(ctx, topic, string(buf)).Err(); err != nil {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	client.Expire(ctx, topic, TopicExpireTimeout)
}

func subscribe(topic string, subject string) {
	for {
		func() {
			defer dipper.SafeExitOnError("[%s] re-subscribing to redis %s", driver.Service, topic)
			client := redisclient.NewClient(redisOptions)
			defer client.Close()
			realTopic := topic
			if topic == eventbus.ReturnTopic || topic == eventbus.APITopic {
				realTopic = topic + dipper.GetIP()
			}
			log.Infof("[%s] start receiving messages on topic: %s", driver.Service, realTopic)
			for {
				messages, err := client.BLPop(context.Background(), time.Second, realTopic).Result()
				if err != nil && !errors.Is(err, redis.Nil) {
					log.Panicf("[%s] redis error: %v", driver.Service, err)
				}
				if len(messages) > 1 {
					for _, m := range messages[1:] {
						payload := dipper.DeserializeContent([]byte(m))
						labels := map[string]string{}
						labelMap, ok := dipper.GetMapData(payload, "labels")
						if ok {
							for k, v := range labelMap.(map[string]interface{}) {
								labels[k] = v.(string)
							}
						}
						data, _ := dipper.GetMapDataStr(payload, "data")
						driver.SendMessage(&dipper.Message{
							Channel: "eventbus",
							Subject: subject,
							Payload: []byte(data),
							Labels:  labels,
							IsRaw:   true,
						})
					}
				}
			}
		}()
		time.Sleep(time.Second)
	}
}
