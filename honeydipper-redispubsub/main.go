package main

import (
	"bufio"
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

// Options : received from the honeydipper daemon
var Options map[string]string

// State : the state of the driver
var State = "loaded"

var service = ""
var in = bufio.NewReader(os.Stdin)

func main() {
	flag.Parse()

	service = os.Args[1]

	log.Printf("[%s-redispubsub] receiving configurations\n", service)
	for {
		msg := dipper.FetchMessage(in)

		switch msg.Channel {
		case "command":
			runCommand(msg.Subject, msg.PayloadType, msg.Payload)
		default:
			log.Panicf("[%s-redispubsub] message in unknown channel: %s", service, msg.Channel)
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
		dipper.SendRawMessage(os.Stdout, "state", State, "", nil)
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
	defer dipper.SafeExitOnError("[%s-redispubsub] reconnecting", service)

	client := redis.NewClient(opts)
	subscription := client.Subscribe("honeydipper-eventbus")
	log.Printf("[%s-redispubsub] start receiving messages\n", service)
	if State != "alive" {
		State = "alive"
		dipper.SendRawMessage(os.Stdout, "state", State, "", nil)
	}
	for {
		message, err := subscription.ReceiveMessage()
		if err != nil {
			panic(err)
		}
		payload := []string{
			"channel=" + message.Channel,
			"payload=" + message.Payload,
		}
		dipper.SendRawMessage(os.Stdout, "eventbus", "message", "kv", payload)
	}
}
