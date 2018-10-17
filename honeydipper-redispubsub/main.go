package main

import (
	"flag"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/honeyscience/honeydipper/dipper"
	"log"
	"os"
	"strconv"
	"time"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis as message queue")
	}
}

var driver *dipper.Driver
var redisClient *redis.Client
var subscription *redis.PubSub
var eventTopic string
var ok bool

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "redispubsub")
	if driver.Service == "receiver" {
		driver.MessageHandlers["eventbus:message"] = relayToRedis
	} else {
		driver.Start = subscribeToRedis
	}
	driver.Run()
}

func connect() {
	eventTopic, ok = driver.GetOptionStr("eventTopic")
	if !ok {
		eventTopic = "honeydipper:events"
	}
	opts := &redis.Options{}
	if value, ok := driver.GetOptionStr("Addr"); ok {
		opts.Addr = value
	}
	if value, ok := driver.GetOptionStr("Password"); ok {
		opts.Password = value
	}
	if DB, ok := driver.GetOptionStr("DB"); ok {
		DBnum, err := strconv.Atoi(DB)
		if err != nil {
			log.Panicf("[%s-%s] invalid db number %s", driver.Service, driver.Name, DB)
		}
		opts.DB = DBnum
	}
	log.Printf("[%s-%s] connecting to redis\n", driver.Service, driver.Name)
	redisClient = redis.NewClient(opts)
	if err := redisClient.Ping().Err(); err != nil {
		log.Panicf("[%s-%s] redis error: %v", driver.Service, driver.Name, err)
	}
}

func relayToRedis(msg *dipper.Message) {
	if redisClient == nil {
		connect()
	}
	var buf []byte
	if !msg.IsRaw {
		buf = dipper.SerializePayload(msg.Payload)
	} else {
		buf = msg.Payload.([]byte)
	}
	if err := redisClient.RPush(eventTopic, string(buf)).Err(); err != nil {
		log.Panicf("[%s-%s] redis error: %v", driver.Service, driver.Name, err)
	}
}

func subscribeToRedis(msg *dipper.Message) {
	log.Printf("[%s-%s] start receiving messages on topic: %s\n", driver.Service, driver.Name, eventTopic)
	connect()
	go func() {
		for {
			func() {
				defer dipper.SafeExitOnError("[%s-%s] reconnecting to redis\n", driver.Service, driver.Name)
				connect()
				for {
					messages, err := redisClient.BLPop(time.Second, eventTopic).Result()
					if err != nil && err != redis.Nil {
						log.Panicf("[%s-%s] redis error: %v", driver.Service, driver.Name, err)
					}
					if len(messages) > 1 {
						for _, m := range messages[1:] {
							driver.SendRawMessage("eventbus", "message", []byte(m))
						}
					}
				}
			}()
			time.Sleep(time.Second)
		}
	}()
}
