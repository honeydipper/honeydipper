package main

import (
	"github.com/honeyscience/honeydipper/dipper"
	"log"
)

var receiver *Service

func startReceiver(cfg *Config) {
	receiver = NewService(cfg, "receiver")
	receiver.Route = receiverRoute
	Services["receiver"] = receiver
	go receiver.start()
}

func receiverRoute(msg *dipper.Message) (ret []RoutedMessage) {
	log.Printf("[receiver] routing message %s.%s", msg.Channel, msg.Subject)
	if msg.Channel == "eventbus" && msg.Subject == "message" {
		rtmsg := RoutedMessage{
			driverRuntime: receiver.getDriverRuntime("eventbus"),
			message:       msg,
		}
		ret = append(ret, rtmsg)
	}
	return ret
}
