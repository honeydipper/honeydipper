package main

import (
	"flag"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/op/go-logging"
	"os"
	"strconv"
	"time"
)

var log *logging.Logger
var driver *dipper.Driver
var redisOptions *redis.Options
var broadcastTopic string
var ok bool
var err error

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc\n")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis as pub/sub\n")
	}
}

func main() {
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
