package main

import (
	"github.com/honeyscience/honeydipper/dipper"
	"log"
)

var engine *Service

func startEngine(cfg *Config) {
	engine = NewService(cfg, "engine")
	engine.Route = engineRoute
	Services["engine"] = engine
	go engine.start()
}

func engineRoute(msg *dipper.Message) (ret []RoutedMessage) {
	log.Printf("[engine] routing message %s.%s %+v", msg.Channel, msg.Subject, msg.Payload)
	return ret
}
