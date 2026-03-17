// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package driver

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/honeydipper/honeydipper/v3/internal/daemon"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

// BuiltinDriver are compiled and delivered with daemon binary in the same container image.
type BuiltinDriver struct {
	meta   *Meta
	stream chan<- *dipper.Message

	input  io.ReadCloser
	output io.WriteCloser
	run    *exec.Cmd
}

// BuiltinPath is the path where the builtin drivers are kept. It will try using $HONEYDIPPER_DRIVERS_BUILTIN by default.
// If $HONEYDIPPER_DRIVERS_BUILTIN is not set, will try using $GOPATH/bin.  If $GOPATH is not defined, use "/opt/honeydipper/driver/builtin".
var BuiltinPath string

// replace the func variable with mock during testing.
var execCommand = exec.Command

func builtinPath() string {
	if BuiltinPath == "" {
		driverPath, ok := os.LookupEnv("HONEYDIPPER_DRIVERS_BUILTIN")
		if ok {
			BuiltinPath = driverPath
		} else if gopath, ok := os.LookupEnv("GOPATH"); ok {
			BuiltinPath = filepath.Join(gopath, "bin")
		} else {
			BuiltinPath = "/opt/honeydipper/drivers/builtin"
		}
	}

	return BuiltinPath
}

// Acquire function is used for getting the driver from sources and validate, for.
// builtin drivers, just make sure the name is valid.
func (d *BuiltinDriver) Acquire() {
	shortName, ok := d.meta.HandlerData["shortName"].(string)
	if !ok || shortName == "" {
		panic(fmt.Errorf("%w: shortName is missing for builtin driver: %s", ErrDriverError, d.meta.Name))
	}
	if strings.ContainsRune(shortName, os.PathSeparator) {
		panic(fmt.Errorf("%w: shortName has path separator in driver: %s", ErrDriverError, d.meta.Name))
	}

	d.meta.Executable = filepath.Join(builtinPath(), shortName)
}

// Prepare function is used for preparing the arguments when calling the executable.
// for the driver.
func (d *BuiltinDriver) Prepare(stream chan<- *dipper.Message) {
	d.stream = stream

	var argsList []interface{}

	args, ok := d.meta.HandlerData["arguments"]
	if ok {
		argsList, ok = args.([]interface{})
	} else {
		argsList = []interface{}{}
		ok = true
	}

	if !ok {
		panic(fmt.Errorf("%w: arguments in driver %s should be a list of strings", ErrDriverError, d.meta.Name))
	}

	for _, v := range argsList {
		d.meta.Arguments = append(d.meta.Arguments, fmt.Sprintf("%+v", v))
	}
}

// Meta function exposes the metadata used for this driver handler.
func (d *BuiltinDriver) Meta() *Meta {
	return d.meta
}

// NewBuiltinDriver creates a handler for the builtin driver specified in the meta info.
func NewBuiltinDriver(m *Meta) *BuiltinDriver {
	return &BuiltinDriver{meta: m}
}

// Start the driver child process.  The "service" indicates which service this driver belongs to.
func (d *BuiltinDriver) Start(service string) {
	d.run = execCommand(d.meta.Executable, append([]string{service}, d.meta.Arguments...)...)
	if input, err := d.run.StdoutPipe(); err != nil {
		dipper.Logger.Panicf("[%s] Unable to link to driver stdout %v", service, err)
	} else {
		d.input = input
		go d.fetchMessages(service)
	}

	if output, err := d.run.StdinPipe(); err != nil {
		dipper.Logger.Panicf("[%s] Unable to link to driver stdin %v", service, err)
	} else {
		d.output = output
	}

	d.run.Stderr = os.Stderr
	d.run.ExtraFiles = []*os.File{os.Stdout} // giving child process stdout for logging
	if err := d.run.Start(); err != nil {
		dipper.Logger.Panicf("[%s] Failed to start driver %v", service, err)
	}
}

// SendMessage sends a dipper message to the driver child process.
func (d *BuiltinDriver) SendMessage(msg *dipper.Message) {
	dipper.SendMessage(d.output, msg)
}

func (d *BuiltinDriver) fetchMessages(service string) {
	quit := false
	daemon.Children.Add(1)
	defer daemon.Children.Done()
	for !quit && !daemon.ShuttingDown {
		func() {
			defer dipper.SafeExitOnError(
				"failed to fetching messages from driver %s.%s",
				service,
				d.meta.Name,
			)
			defer dipper.CatchError(io.EOF, func() { quit = true })
			for !quit && !daemon.ShuttingDown {
				message := dipper.FetchRawMessage(d.input)
				d.stream <- message
			}
		}()
	}
	dipper.Logger.Warningf("[%s-%s] driver closed for business", service, d.meta.Name)
}

// Close close all the channels for the driver and signal the child process to exit.
func (d *BuiltinDriver) Close() {
	if d.stream != nil {
		close(d.stream)
		d.stream = nil
	}
	if d.output != nil {
		d.output.Close()
		d.output = nil
	}
}

// Wait wait for the driver process to exit.
func (d *BuiltinDriver) Wait() {
	_ = d.run.Wait()
	d.run = nil
}
