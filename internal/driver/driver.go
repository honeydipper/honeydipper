// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package driver

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

const (
	// DriverError is the base for all driver related error.
	DriverError dipper.Error = "driver error"

	// DriverMessageBuffer is the size of the driver message buffer.
	DriverMessageBuffer = 10
)

// Handler provides common functions for handling a driver.
type Handler interface {
	Acquire()
	Prepare()
	Meta() *Meta
}

// replace the func variable with mock during testing.
var execCommand = exec.Command

// NewDriver creates a driver object to represent a child process.
func NewDriver(data map[string]interface{}) Handler {
	var meta Meta
	err := mapstructure.Decode(data, &meta)
	if err != nil {
		panic(fmt.Errorf("malformat driver meta: %+v: %w", data, err))
	}

	if meta.Name == "" {
		panic(fmt.Errorf("driver name missing: %+v: %w", meta, DriverError))
	}

	var dh Handler

	switch meta.Type {
	case "builtin":
		dh = NewBuiltinDriver(&meta)
	default:
		panic(fmt.Errorf("unsupported driver type: %s: %w", meta.Type, DriverError))
	}

	dh.Acquire()
	dh.Prepare()

	if meta.Executable == "" {
		panic(fmt.Errorf("executable not defined for driver: %s: %w", meta.Name, DriverError))
	}

	return dh
}

// Runtime contains the runtime information of the running driver.
type Runtime struct {
	Data        interface{}
	DynamicData interface{}
	Feature     string
	Stream      chan dipper.Message
	Input       io.ReadCloser
	Output      io.WriteCloser
	Service     string
	Run         *exec.Cmd
	State       int
	Handler     Handler
}

// Start the driver child process.  The "service" indicates which service this driver belongs to.
func (runtime *Runtime) Start(service string) {
	runtime.Service = service

	m := runtime.Handler.Meta()

	run := execCommand(m.Executable, append([]string{service}, m.Arguments...)...)
	if input, err := run.StdoutPipe(); err != nil {
		dipper.Logger.Panicf("[%s] Unable to link to driver stdout %v", service, err)
	} else {
		runtime.Input = input
		runtime.Stream = make(chan dipper.Message, DriverMessageBuffer)
		go runtime.fetchMessages()
	}

	if output, err := run.StdinPipe(); err != nil {
		dipper.Logger.Panicf("[%s] Unable to link to driver stdin %v", service, err)
	} else {
		runtime.Output = output
	}

	run.Stderr = os.Stderr
	run.ExtraFiles = []*os.File{os.Stdout} // giving child process stdout for logging
	if err := run.Start(); err != nil {
		dipper.Logger.Panicf("[%s] Failed to start driver %v", service, err)
	}

	runtime.Run = run
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

// SendMessage sentds a dipper message to the driver child process.
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
	dipper.SendMessage(runtime.Output, msg)
}

func (runtime *Runtime) fetchMessages() {
	quit := false
	daemon.Children.Add(1)
	defer daemon.Children.Done()
	for !quit && !daemon.ShuttingDown {
		func() {
			defer dipper.SafeExitOnError(
				"failed to fetching messages from driver %s.%s",
				runtime.Service,
				runtime.Handler.Meta().Name,
			)
			defer dipper.CatchError(io.EOF, func() { quit = true })
			for !quit && !daemon.ShuttingDown {
				message := dipper.FetchRawMessage(runtime.Input)
				runtime.Stream <- *message
			}
		}()
	}
	dipper.Logger.Warningf("[%s-%s] driver closed for business", runtime.Service, runtime.Handler.Meta().Name)
}
