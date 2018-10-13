package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/go-redis/redis"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis pug/sub")
	}
}

// Options : received from the honeydipper daemon
var Options map[string]string

// Sender : lock to protecting the sending operation
var Sender = sync.Mutex{}

// State : the state of the driver
var State = "loaded"

var service = ""
var in = bufio.NewReader(os.Stdin)

func readField(sep byte) string {
	val, err := in.ReadString(sep)
	if err != nil {
		panic(err)
	}
	return val[:len(val)-1]
}

func main() {
	flag.Parse()

	service = os.Args[1]

	log.Printf("[%s-redispubsub] receiving configurations\n", service)
	for {
		var channel string
		var subject string
		var payloadType string
		var payload []string

		channel = readField(':')
		subject = readField(':')
		payloadType = readField('\n')
		log.Printf("[%s-redispubsub] getting data from daemon %s:%s:%s\n", service, channel, subject, payloadType)
		if payloadType != "" {
			for {
				line := readField('\n')
				if len(line) > 0 {
					payload = append(payload, line)
				} else {
					break
				}
			}
		}

		switch channel {
		case "command":
			runCommand(subject, payloadType, payload)
		default:
			log.Panicf("[%s-redispubsub] message in unknown channel: %s", service, channel)
		}
	}
}

func runCommand(cmd string, payloadType string, payload []string) {
	switch cmd {
	case "options":
		for _, line := range payload {
			parts := strings.Split(line, "=")
			key := parts[0]
			val := strings.Join(parts[1:], "=")
			if Options == nil {
				Options = make(map[string]string)
			}
			log.Printf("%s=%s", key, val)
			Options[key] = val
		}
		log.Printf("%+v", Options)
	case "ping":
		sendMessage("state", State, "", nil)
	case "start":
		connect()
	case "quit":
		log.Fatalf("[%s-redispubsub] terminating on signal\n", service)
	default:
		log.Panicf("[%s-redispubsub] unkown command %s\n", service, cmd)
	}
}

func connect() {
	opts := redis.Options{}
	if addr, ok := Options["Addr"]; ok {
		opts.Addr = addr
	}
	if passwd, ok := Options["Password"]; ok {
		opts.Password = passwd
	}
	if DB, ok := Options["DB"]; ok {
		DBnum, err := strconv.Atoi(DB)
		if err != nil {
			log.Panicf("[%s-redispubsub] invalid db number %s", DB)
		}
		opts.DB = DBnum
	}
	log.Printf("[%s-redispubsub] connecting to redis\n", service)
	for {
		reconnect(&opts)
	}
}

func reconnect(opts *redis.Options) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[%s-redispubsub] recovering after error %v", service, r)
			State = "failed"
			sendMessage("state", State, "", nil)
		}
	}()
	client := redis.NewClient(opts)
	subscription := client.Subscribe("honeydipper-eventbus")
	log.Printf("[%s-redispubsub] start receiving messages\n", service)
	State = "alive"
	sendMessage("state", State, "", nil)
	for {
		message, err := subscription.ReceiveMessage()
		if err != nil {
			panic(err)
		}
		payload := []string{
			"channel=" + message.Channel,
			"payload=" + message.Payload,
		}
		sendMessage("eventbus", "message", "kv", payload)
	}
}

func sendMessage(channel string, subject string, payloadType string, payload []string) {
	Sender.Lock()
	fmt.Printf("%s:%s:%s\n", channel, subject, payloadType)
	if len(payload) > 0 {
		for _, line := range payload {
			fmt.Println(line)
		}
		fmt.Println()
	}
	Sender.Unlock()
}
