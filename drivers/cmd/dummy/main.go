package main

import (
	"flag"
	"fmt"
	"github.com/honeyscience/honeydipper/pkg/dipper"
	"os"
	"time"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis pug/sub")
	}
}

var driver *dipper.Driver

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "dummy")
	if driver.Service == "receiver" {
		driver.Start = waitAndSendDummyEvent
	}
	driver.Run()
}

func waitAndSendDummyEvent(msg *dipper.Message) {
	go func() {
		time.Sleep(20 * time.Second)
		driver.SendMessage(&dipper.Message{
			Channel: "eventbus",
			Subject: "message",
			Payload: map[string]interface{}{"data": []string{"line 1", "line 2"}},
		})
		driver.State = "cold"
		driver.Ping(msg)
	}()
}
