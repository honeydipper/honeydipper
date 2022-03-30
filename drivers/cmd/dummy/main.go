// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of accessing redis pug/sub")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "dummy")
	if driver.Service == "receiver" {
		driver.Start = waitAndSendDummyEvent
	}
	driver.Run()
}

func waitAndSendDummyEvent(msg *dipper.Message) {
	go func() {
		//nolint:gomnd
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
