// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package driver

import (
	"errors"
	"fmt"
	"time"

	"github.com/honeydipper/honeydipper/v3/internal/daemon"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

// ErrDriverError is the base for all driver related error.
var ErrDriverError = errors.New("driver error")

// DriverMessageBuffer is the size of the driver message buffer.
const DriverMessageBuffer = 10

// Handler provides common functions for handling a driver.
type Handler interface {
	Acquire()
	Meta() *Meta
	Prepare(input chan<- *dipper.Message)
	SendMessage(*dipper.Message)
	Start(string)
	Close()
	Wait()
}

// Runtime contains the runtime information of the running driver.
type Runtime struct {
	Data        interface{}
	DynamicData interface{}
	Feature     string
	Handler     Handler
	Stream      <-chan *dipper.Message
	Service     string
	State       int
}

// NewDriver creates a driver object to represent a child process.
func NewDriver(feature string, metaData map[string]interface{}, driverData interface{}, dynamicData interface{}) *Runtime {
	var meta Meta
	err := mapstructure.Decode(metaData, &meta)
	if err != nil {
		panic(fmt.Errorf("malformat driver meta: %+v: %w", metaData, err))
	}

	if meta.Name == "" {
		panic(fmt.Errorf("%w: driver name missing: %+v", ErrDriverError, meta))
	}

	var dh Handler

	switch meta.Type {
	case "builtin":
		dh = NewBuiltinDriver(&meta)
	case "null":
		dh = NewNullDriver(&meta)
	default:
		panic(fmt.Errorf("%w: unsupported driver type: %s", ErrDriverError, meta.Type))
	}

	stream := make(chan *dipper.Message, DriverMessageBuffer)

	runtime := &Runtime{
		Data:        driverData,
		DynamicData: dynamicData,
		Feature:     feature,
		Handler:     dh,
		Stream:      stream,
	}

	dh.Acquire()
	dh.Prepare(stream)

	return runtime
}

// Start the driver child process.  The "service" indicates which service this driver belongs to.
func (runtime *Runtime) Start(service string) {
	runtime.Service = service
	runtime.Handler.Start(service)
	runtime.SendOptions()
}

// SendOptions sends driver options and data to the child process as a dipper message.
func (runtime *Runtime) SendOptions() {
	runtime.SendMessage(&dipper.Message{
		Channel: "command",
		Subject: "options",
		IsRaw:   false,
		Payload: map[string]interface{}{
			"data":        runtime.Data,
			"dynamicData": runtime.DynamicData,
		},
	})
	runtime.SendMessage(&dipper.Message{
		Channel: "command",
		Subject: "start",
	})
}

// SendMessage sends a dipper message to the driver child process.
func (runtime *Runtime) SendMessage(msg *dipper.Message) {
	if runtime.Feature != "emitter" {
		if emitter, ok := daemon.Emitters[runtime.Service]; ok {
			emitter.CounterIncr("honey.honeydipper.local.message", []string{
				"service:" + runtime.Service,
				"driver:" + runtime.Handler.Meta().Name,
				"direction:outbound",
				"channel:" + msg.Channel,
				"subject:" + msg.Subject,
			})
		}
	}
	runtime.Handler.SendMessage(msg)
}

// Ready waits until the driver is alive or report error.
func (runtime *Runtime) Ready(d time.Duration) {
	var elapsed time.Duration
	for runtime.State < DriverAlive && elapsed < d {
		time.Sleep(time.Second)
		elapsed += time.Second
	}

	if runtime.State != DriverAlive {
		panic(fmt.Errorf("%w: feature failed or loading timeout: %s", ErrDriverError, runtime.Feature))
	}
}
