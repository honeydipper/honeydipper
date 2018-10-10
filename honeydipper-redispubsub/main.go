package main

import (
	"flag"
	"fmt"
	"github.com/go-redis/redis"
	"log"
	"os"
	"sync"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis pug/sub")
	}
}

var raiser = sync.Mutex{}
var started = false
var alive = false
var service = ""

func main() {
	flag.Parse()

	service = os.Args[1]

	word := ""

	clientOps := redis.Options{
		Addr: "127.0.0.1:6379",
	}
	log.Printf("[%s-redispubsub] receiving configurations\n", service)
	for {
		_, err := fmt.Scanln(&word)
		log.Printf("[%s-redispubsub] getting data from daemon %s\n", service, word)
		if err != nil {
			log.Panicf("[%s-redispubsub] unable to read next instruction", service)
		}
		switch word {
		case "option:Addr":
			if _, err := fmt.Scan(&word); err != nil {
				log.Panicln("[%s-redispubsub] unable to read Addr, service")
			}
			clientOps.Addr = word
		case "option:Password":
			if _, err := fmt.Scan(&word); err != nil {
				log.Panicln("[%s-redispubsub] unable to read Password, service")
			}
			clientOps.Password = word
		case "action:go":
			if !started {
				started = true
				go connect(&clientOps)
			}
		case "action:ping":
			raiseEvent("signal:pong{started:%t,alive:%t}\n", started, alive)
		case "action:quit":
			log.Fatalf("[%s-redispubsub] terminating on signal\n", service)
		}
	}
}

func connect(opts *redis.Options) {
	log.Printf("[%s-redispubsub] connecting to redis\n", service)
	for {
		reconnect(opts)
	}
}

func reconnect(opts *redis.Options) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[%s-redispubsub] recovering after error %v", service, r)
		}
	}()
	client := redis.NewClient(opts)
	subscription := client.Subscribe("honeydipper-eventbus")
	alive = true
	log.Printf("[%s-redispubsub] start receiving messages\n", service)
	for {
		message, err := subscription.ReceiveMessage()
		if err != nil {
			panic(err)
		}
		raiseEvent("signal:message\n%s\n", message.Payload)
	}
}

func raiseEvent(msg string, args ...interface{}) {
	raiser.Lock()
	defer raiser.Unlock()
	fmt.Printf(msg, args...)
}
