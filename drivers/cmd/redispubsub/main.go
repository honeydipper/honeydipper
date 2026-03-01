// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redispubsub enables Honeydipper to use redis to broadcast internal messages.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/honeydipper/honeydipper/v4/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/v4/internal/daemon"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/op/go-logging"
)

var (
	log              *logging.Logger
	driver           *dipper.Driver
	redisOptions     *redisclient.Options
	broadcastTopic   string
	broadcastChannel string
	ok               bool
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
	driver.RPCHandlers["expect|interruptible"] = expect
	if driver.Service == "operator" {
		driver.Commands["send"] = sendBroadcast
		driver.Commands["expect|interruptible"] = expect
		driver.DefaultTimeout["expect"] = "30m"
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

func sendBroadcast(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)

	labels := map[string]any{}
	if l, ok := dipper.GetMapData(msg.Payload, "labels"); ok {
		labels = l.(map[string]interface{})
	}
	labels["from"] = dipper.GetIP()

	topic := broadcastTopic
	if str, ok := dipper.GetMapDataStr(msg.Payload, "topic"); ok {
		topic = str
	}

	rmsg := map[string]interface{}{
		"labels":  labels,
		"subject": dipper.MustGetMapDataStr(msg.Payload, "subject"),
	}
	if data, ok := dipper.GetMapData(msg.Payload, "payload"); ok && data != nil {
		rmsg["payload"] = data
	}

	buf := dipper.SerializeContent(rmsg)
	client := redisclient.NewClient(redisOptions)
	defer client.Close()
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	if err := client.Publish(ctx, topic, string(buf)).Err(); err != nil {
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
			ctx, cancel := driver.GetContext(nil)
			defer cancel()
			defer dipper.IgnoreError(context.Canceled)
			pubsub := client.Subscribe(ctx, broadcastTopic)

			dipper.Must(pubsub.Receive(ctx))

			ch := pubsub.Channel()
			for rmsg := range ch {
				msg := &dipper.Message{}
				dipper.Must(json.Unmarshal([]byte(rmsg.Payload), msg))
				if msg.Labels != nil && msg.Labels["service"] != "" && msg.Labels["service"] != driver.Service {
					continue
				}
				if msg.Subject == "" {
					log.Warningf("[%s] received message without subject %v", driver.Service, msg)

					continue
				}
				msg.Channel = broadcastChannel
				driver.SendMessage(msg)
			}
		}()
		if driver.State != dipper.DriverStateAlive {
			// gracefully exit if driver is stopping, otherwise keep retrying to subscribe
			return
		}
		time.Sleep(time.Second)
	}
}

var (
	subscriberLock sync.Mutex
	subscribers    = map[string]map[string]func(map[string]any){}
	cancelers      = map[string]context.CancelFunc{}
)

func subscribeOnce(ctx context.Context, topic string, key string, fn func(rpayload map[string]any)) {
	subscriberLock.Lock()
	subs := subscribers[topic]
	if subs == nil {
		subs = map[string]func(map[string]any){}
		subscribers[topic] = subs
	}
	for _, found := subs[key]; found; _, found = subs[key] {
		subscriberLock.Unlock()
		select {
		case <-ctx.Done():
			log.Warningf("[%s] key %s busy for expecting messages on topic %s", driver.Service, key, topic)

			return
		case <-time.After(time.Millisecond * 10):
		}
		subscriberLock.Lock()
	}
	subs[key] = fn
	if len(subs) == 1 {
		daemon.Go(func() {
			client := redisclient.NewClient(redisOptions)
			defer client.Close()
			subcriberCtx, cancel := context.WithCancel(context.Background())
			cancelers[topic] = cancel
			pubsub := client.PSubscribe(subcriberCtx, topic)

			for rmesg := range pubsub.Channel() {
				if rmesg == nil {
					return
				}

				rmsg := dipper.DeserializeContent([]byte(rmesg.Payload)).(map[string]any)
				subscriberLock.Lock()
				for _, sub := range subs {
					go sub(rmsg)
				}
				subscriberLock.Unlock()
			}
		})
	}
	subscriberLock.Unlock()

	<-ctx.Done()

	subscriberLock.Lock()
	defer subscriberLock.Unlock()
	delete(subs, key)
	if len(subs) == 0 {
		cancelers[topic]()
		delete(cancelers, topic)
		delete(subscribers, topic)
	}
}

func expect(msg *dipper.Message) {
	dipper.DeserializePayload(msg)
	topic := broadcastTopic
	if str, ok := dipper.GetMapDataStr(msg.Payload, "topic"); ok {
		topic = str
	}
	subject, _ := dipper.GetMapDataStr(msg.Payload, "subject")
	criteria, _ := dipper.GetMapData(msg.Payload, "match")
	key := msg.Labels["key"]

	ctx, cancel := driver.GetContext(msg)
	defer cancel()
	received := false

	subscribeOnce(ctx, topic, key, func(rmsg map[string]any) {
		if subject != "" && rmsg["subject"] != subject {
			return
		}
		if dipper.CompareAll(rmsg, criteria) {
			defer cancel()

			received = true
			ret := dipper.Message{}
			dipper.Must(mapstructure.Decode(rmsg, &ret))

			msg.Reply <- ret
		}
	})

	if !received && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		panic(ctx.Err())
	}
}
