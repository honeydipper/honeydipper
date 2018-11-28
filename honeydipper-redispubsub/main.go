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

// EventBusOptions : stores all the redis key names used by honeydipper
type EventBusOptions struct {
	EventTopic   string
	CommandTopic string
	ReturnTopic  string
}

var log *logging.Logger = dipper.GetLogger("redispubsub")
var driver *dipper.Driver
var eventbus *EventBusOptions
var redisOptions *redis.Options

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc\n")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis as message queue\n")
	}
}

func main() {
	flag.Parse()
	driver = dipper.NewDriver(os.Args[1], "redispubsub")
	if driver.Service == "receiver" {
		driver.MessageHandlers["eventbus:message"] = relayToRedis
		driver.Start = start
	} else if driver.Service == "engine" {
		driver.MessageHandlers["eventbus:command"] = relayToRedis
		driver.Start = start
	} else if driver.Service == "operator" {
		driver.MessageHandlers["eventbus:message"] = relayToRedis
		driver.MessageHandlers["eventbus:return"] = relayToRedis
		driver.Start = start
	}
	driver.Run()
}

func loadOptions() {
	log.Infof("[%s] receiving driver data %+v", driver.Service, driver.Options)

	eb := &EventBusOptions{
		CommandTopic: "honeydipper:commands",
		EventTopic:   "honeydipper:events",
		ReturnTopic:  "honeydipper:return:",
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
	eventbus = eb

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
	if driver.Service == "engine" {
		go subscribe(eventbus.EventTopic, "message")
		go subscribe(eventbus.ReturnTopic, "return")
	} else if driver.Service == "operator" {
		go subscribe(eventbus.CommandTopic, "command")
	} else { // "receiver"
		// test connection
		client := redis.NewClient(redisOptions)
		defer client.Close()
		if err := client.Ping().Err(); err != nil {
			log.Panicf("[%s] redis error: %v", driver.Service, err)
		}
	}
}

func relayToRedis(msg *dipper.Message) {
	client := redis.NewClient(redisOptions)
	defer client.Close()

	var buf []byte
	var returnTo string

	if !msg.IsRaw {
		if msg.Subject == "command" {
			msg.Payload.(map[string]interface{})["from"] = dipper.GetIP()
		}
		buf = dipper.SerializeContent(msg.Payload)
	} else {
		buf = msg.Payload.([]byte)
		msg = dipper.DeserializePayload(msg)
		if msg.Subject == "command" {
			msg.Payload.(map[string]interface{})["from"] = dipper.GetIP()
			buf = dipper.SerializeContent(msg.Payload)
		}
	}

	returnTo, _ = dipper.GetMapDataStr(msg.Payload, "return_to")

	topic := eventbus.EventTopic
	isReturn := false
	if msg.Subject == "command" {
		topic = eventbus.CommandTopic
	} else if msg.Subject == "return" {
		topic = eventbus.ReturnTopic + returnTo
		isReturn = true
	}

	if err := client.RPush(topic, string(buf)).Err(); err != nil {
		log.Panicf("[%s] redis error: %v", driver.Service, err)
	}
	if isReturn {
		client.Expire(topic, time.Second*1800)
	}
}

func subscribe(topic string, subject string) {
	for {
		func() {
			defer dipper.SafeExitOnError("[%s] re-subscribing to redis %s", driver.Service, topic)
			client := redis.NewClient(redisOptions)
			defer client.Close()
			log.Infof("[%s] start receiving messages on topic: %s", driver.Service, topic)
			for {
				realTopic := topic
				if topic == eventbus.ReturnTopic {
					realTopic = topic + dipper.GetIP()
				}
				messages, err := client.BLPop(time.Second, realTopic).Result()
				if err != nil && err != redis.Nil {
					log.Panicf("[%s] redis error: %v", driver.Service, err)
				}
				if len(messages) > 1 {
					for _, m := range messages[1:] {
						driver.SendRawMessage("eventbus", subject, []byte(m))
					}
				}
			}
		}()
		time.Sleep(time.Second)
	}
}
