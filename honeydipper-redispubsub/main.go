package main

import (
	"flag"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/honeyscience/honeydipper/dipper"
	"log"
	"os"
	"strconv"
	"strings"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis pug/sub")
	}
}

var driver *dipper.Driver
var redisClient *redis.Client
var subscription *redis.PubSub

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
	opts := &redis.Options{}
	if addr, ok := driver.Options["Addr"]; ok {
		opts.Addr = addr
	}
	if passwd, ok := driver.Options["Password"]; ok {
		opts.Password = passwd
	}
	if DB, ok := driver.Options["DB"]; ok {
		DBnum, err := strconv.Atoi(DB)
		if err != nil {
			log.Panicf("[%s-%s] invalid db number %s", driver.Service, driver.Name, DB)
		}
		opts.DB = DBnum
	}
	log.Printf("[%s-%s] connecting to redis\n", driver.Service, driver.Name)
	redisClient = redis.NewClient(opts)
	if err := redisClient.Ping().Err(); err != nil {
		panic(err)
	}
}

func relayToRedis(msg *dipper.Message) {
	if redisClient == nil {
		connect()
		subscription = nil
	}
	channel, ok := driver.Options["redis_channel"]
	if !ok {
		channel = "honeydipper:eventbus"
	}
	message := strings.Join(msg.Payload, "\n")
	if err := redisClient.Publish(channel, message).Err(); err != nil {
		panic(err)
	}
}

func connectForSubscription() {
	if redisClient == nil {
		connect()
	}
	if subscription == nil {
		channel, ok := driver.Options["redis_channel"]
		if !ok {
			channel = "honeydipper:eventbus"
		}
		subscription = redisClient.Subscribe(channel)
		log.Printf("[%s-%s] start receiving messages on channel: %s\n", driver.Service, driver.Name, channel)
	}
}

func subscribeToRedis(msg *dipper.Message) {
	connectForSubscription()
	go func() {
		for {
			func() {
				defer dipper.SafeExitOnError("[%s-%s] reconnecting to redis\n", driver.Service, driver.Name)
				connectForSubscription()
				for {
					message, err := subscription.ReceiveMessage()
					if err != nil {
						panic(err)
					}
					payload := []string{
						"channel=" + message.Channel,
						"payload=" + message.Payload,
					}
					driver.SendRawMessage("eventbus", "message", "kv", payload)
				}
			}()
		}
	}()
}
