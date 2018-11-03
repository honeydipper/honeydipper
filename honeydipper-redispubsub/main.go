package main

import (
	"flag"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/honeyscience/honeydipper/dipper"
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
var commandTopic string
var ok bool

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "redispubsub")
	if driver.Service == "receiver" {
		driver.MessageHandlers["eventbus:message"] = relayToRedis
	} else if driver.Service == "engine" {
		driver.MessageHandlers["eventbus:command"] = relayToRedis
		driver.Start = subscribeToRedis
	} else if driver.Service == "operator" {
		driver.MessageHandlers["eventbus:message"] = relayToRedis
		driver.Start = subscribeToRedis
	}
	driver.Run()
}

func connect() {
	commandTopic, ok = driver.GetOptionStr("commandTopic")
	if !ok {
		commandTopic = "honeydipper:commands"
	}
	eventTopic, ok = driver.GetOptionStr("eventTopic")
	if !ok {
		eventTopic = "honeydipper:events"
	}
	opts := &redis.Options{}
	driver.Logger.Infof("[%s] receiving driver data %+v", driver.Service, driver.Options)
	if value, ok := driver.GetOptionStr("data.Addr"); ok {
		opts.Addr = value
	}
	if value, ok := driver.GetOptionStr("data.Password"); ok {
		opts.Password = value
	}
	if DB, ok := driver.GetOptionStr("data.DB"); ok {
		DBnum, err := strconv.Atoi(DB)
		if err != nil {
			driver.Logger.Panicf("[%s] invalid db number %s", driver.Service, DB)
		}
		opts.DB = DBnum
	}
	driver.Logger.Infof("[%s] connecting to redis\n", driver.Service)
	redisClient = redis.NewClient(opts)
	if err := redisClient.Ping().Err(); err != nil {
		driver.Logger.Panicf("[%s] redis error: %v", driver.Service, err)
	}
}

func relayToRedis(msg *dipper.Message) {
	if redisClient == nil {
		connect()
	}
	var buf []byte
	if !msg.IsRaw {
		buf = dipper.SerializeContent(msg.Payload)
	} else {
		buf = msg.Payload.([]byte)
	}
	topic := eventTopic
	if msg.Subject == "command" {
		topic = commandTopic
	}
	if err := redisClient.RPush(topic, string(buf)).Err(); err != nil {
		driver.Logger.Panicf("[%s] redis error: %v", driver.Service, err)
	}
}

func subscribeToRedis(msg *dipper.Message) {
	driver.Logger.Infof("[%s] start receiving messages on topic: %s", driver.Service, eventTopic)
	connect()
	go func() {
		for {
			func() {
				defer dipper.SafeExitOnError(driver.Logger, "[%s] reconnecting to redis\n", driver.Service)
				connect()
				for {
					topic := eventTopic
					if driver.Service == "operator" {
						topic = commandTopic
					}
					messages, err := redisClient.BLPop(time.Second, topic).Result()
					if err != nil && err != redis.Nil {
						driver.Logger.Panicf("[%s] redis error: %v", driver.Service, err)
					}
					if len(messages) > 1 {
						for _, m := range messages[1:] {
							if driver.Service == "operator" {
								driver.SendRawMessage("eventbus", "command", []byte(m))
							} else {
								driver.SendRawMessage("eventbus", "message", []byte(m))
							}
						}
					}
				}
			}()
			time.Sleep(time.Second)
		}
	}()
}
