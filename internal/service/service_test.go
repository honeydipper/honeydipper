// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package service

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/internal/driver"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestServiceLoopCatchError(t *testing.T) {
	f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0777)
	dipper.GetLogger("test service", "DEBUG", f, f)

	svc := &Service{
		name: "testsvc",
		driverRuntimes: map[string]*driver.Runtime{
			"d1": &driver.Runtime{
				State: driver.DriverAlive,
				Meta: &config.DriverMeta{
					Name: "testdriver1",
				},
			},
		},
		responders: map[string][]MessageResponder{
			"test:error1": []MessageResponder{
				func(d *driver.Runtime, m *dipper.Message) {
					panic(fmt.Errorf("error in responder"))
				},
			},
		},
		transformers: map[string][]func(*driver.Runtime, *dipper.Message) *dipper.Message{
			"test:error2": []func(*driver.Runtime, *dipper.Message) *dipper.Message{
				func(d *driver.Runtime, m *dipper.Message) *dipper.Message {
					panic(fmt.Errorf("error in transformer"))
				},
			},
		},
		Route: func(m *dipper.Message) []RoutedMessage {
			if m.Channel == "test" && m.Subject == "error0" {
				panic(fmt.Errorf("error in route"))
			}
			return nil
		},
	}

	svc.driverRuntimes["d1"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0777)
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic in route")
	}()
	// injecting error in route
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
		Channel: "test",
		Subject: "error0",
	}
	time.Sleep(30 * time.Millisecond)
	// quiting faster by send an extra message
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
		Channel: "test",
		Subject: "noerror",
	}
	daemon.ShutDown()
	daemon.ShuttingDown = false

	svc.driverRuntimes["d1"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0777)
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic in responder")
	}()
	// injecting error in responder
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
		Channel: "test",
		Subject: "error1",
	}
	time.Sleep(30 * time.Millisecond)
	// quiting faster by send an extra message
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
		Channel: "test",
		Subject: "noerror",
	}
	daemon.ShutDown()
	daemon.ShuttingDown = false

	svc.driverRuntimes["d1"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0777)
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic in transformer")
	}()
	// injecting error in transformer
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
		Channel: "test",
		Subject: "error2",
	}
	time.Sleep(30 * time.Millisecond)
	// quiting faster by send an extra message
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
		Channel: "test",
		Subject: "noerror",
	}
	daemon.ShutDown()
	daemon.ShuttingDown = false

	svc.driverRuntimes["d1"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0777)
	// injecting error in process
	svc.driverRuntimes["d1"].Meta = nil
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic in process itself")
	}()
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
		Channel: "test",
		Subject: "error3",
	}
	// recover the service object to avoid crash during quiting
	svc.driverRuntimes["d1"].Meta = &config.DriverMeta{
		Name: "testdriver1",
	}
	time.Sleep(30 * time.Millisecond)
	// quiting faster by send an extra message
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
		Channel: "test",
		Subject: "noerror",
	}
	daemon.ShutDown()
	daemon.ShuttingDown = false
}
