// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
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
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}

	stream := make(chan *dipper.Message, 1)
	svc := &Service{
		name: "testsvc",
		driverRuntimes: map[string]*driver.Runtime{
			"d1": {
				Feature: "d1",
				Handler: &driver.NullDriverHandler{},
				State:   driver.DriverAlive,
				Stream:  stream,
			},
		},
		responders: map[string][]MessageResponder{
			"test:error1": {
				func(d *driver.Runtime, m *dipper.Message) {
					panic(fmt.Errorf("error in responder"))
				},
			},
		},
		transformers: map[string][]func(*driver.Runtime, *dipper.Message) *dipper.Message{
			"test:error2": {
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

	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic in route")
	}()
	// injecting error in route
	stream <- &dipper.Message{
		Channel: "test",
		Subject: "error0",
	}
	time.Sleep(30 * time.Millisecond)
	// quiting faster by send an extra message
	stream <- &dipper.Message{
		Channel: "test",
		Subject: "noerror",
	}
	daemon.ShutDown()

	daemon.ShuttingDown = false
	stream = make(chan *dipper.Message, 1)
	svc.driverRuntimes["d1"].Stream = stream
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic in responder")
	}()
	// injecting error in responder
	stream <- &dipper.Message{
		Channel: "test",
		Subject: "error1",
	}
	time.Sleep(30 * time.Millisecond)
	// quiting faster by send an extra message
	stream <- &dipper.Message{
		Channel: "test",
		Subject: "noerror",
	}
	daemon.ShutDown()

	daemon.ShuttingDown = false
	stream = make(chan *dipper.Message, 1)
	svc.driverRuntimes["d1"].Stream = stream
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic in transformer")
	}()
	// injecting error in transformer
	stream <- &dipper.Message{
		Channel: "test",
		Subject: "error2",
	}
	time.Sleep(30 * time.Millisecond)
	// quiting faster by send an extra message
	stream <- &dipper.Message{
		Channel: "test",
		Subject: "noerror",
	}
	daemon.ShutDown()

	daemon.ShuttingDown = false
	stream = make(chan *dipper.Message, 1)
	svc.driverRuntimes["d1"].Stream = stream
	// injecting error in process
	svc.driverRuntimes["d1"].Handler = nil
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic in process itself")
	}()
	stream <- &dipper.Message{
		Channel: "test",
		Subject: "error3",
	}
	// recover the service object to avoid crash during quiting
	svc.driverRuntimes["d1"].Handler = &driver.NullDriverHandler{}
	time.Sleep(30 * time.Millisecond)
	// quiting faster by send an extra message
	stream <- &dipper.Message{
		Channel: "test",
		Subject: "noerror",
	}
	daemon.ShutDown()
	daemon.ShuttingDown = false
}

func TestServiceRemoveEmitter(t *testing.T) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}

	stream := make(chan *dipper.Message, 1)
	svc := &Service{
		name: "testsvc",
		driverRuntimes: map[string]*driver.Runtime{
			"driver:d1": {
				Feature: "driver:d1",
				Handler: driver.NewNullDriver(&driver.Meta{
					Name: "d1",
					Type: "null",
				}),
				State:  driver.DriverAlive,
				Stream: stream,
			},
			"emitter": {
				Feature: "emitter",
				Handler: driver.NewNullDriver(&driver.Meta{
					Name: "test-emitter",
					Type: "null",
				}),
				State:  driver.DriverAlive,
				Stream: make(chan *dipper.Message),
			},
		},
		Route: func(m *dipper.Message) []RoutedMessage {
			return nil
		},
	}
	daemon.Emitters["testsvc"] = svc

	daemon.ShuttingDown = false
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic if emitter is removed")
	}()

	go func() {
		daemon.Children.Add(1)
		defer daemon.Children.Done()

		assert.NotPanics(t, func() {
			for i := 0; i < 50; i++ {
				select {
				case stream <- &dipper.Message{
					Channel: "test",
					Subject: "noerror",
				}:
					dipper.Logger.Infof("written msg no. %+v", i)
					time.Sleep(10 * time.Millisecond)
				default:
					dipper.Logger.Infof("unable to write, server shutdown")
				}
			}
		}, "sending message to service should not panic when emitter is removed")
	}()

	newCfg := &config.Config{
		Services: []string{"testsvc"},
		Staged: &config.DataSet{
			Drivers: map[string]interface{}{
				"daemon": map[string]interface{}{
					"features": map[string]interface{}{
						"global": []interface{}{
							map[string]interface{}{
								"name": "driver:d1",
							},
						},
					},
					"drivers": map[string]interface{}{
						"d1": map[string]interface{}{
							"name": "d1",
							"type": "null",
						},
					},
				},
			},
		},
	}
	svc.config = newCfg
	svc.config.ResetStage()

	time.Sleep(100 * time.Millisecond)
	assert.NotPanics(t, svc.Reload, "service reload should not panic when emitter is removed")
	time.Sleep(100 * time.Millisecond)

	daemon.ShutDown()
}

func TestServiceEmitterCrashing(t *testing.T) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}

	stream := make(chan *dipper.Message, 1)
	emitterStream := make(chan *dipper.Message, 1)
	svc := &Service{
		name: "testsvc",
		driverRuntimes: map[string]*driver.Runtime{
			"driver:d1": {
				Feature: "d1",
				Handler: &driver.NullDriverHandler{},
				State:   driver.DriverAlive,
				Stream:  stream,
			},
			"emitter": {
				Feature: "test-emitter",
				Handler: &driver.NullDriverHandler{
					SendMessageFunc: func(*dipper.Message) {
						if emitterStream == nil {
							panic("emitter crashed")
						}
					},
				},
				State:  driver.DriverAlive,
				Stream: emitterStream,
			},
		},
		Route: func(m *dipper.Message) []RoutedMessage {
			return nil
		},
	}
	daemon.Emitters["testsvc"] = svc

	daemon.ShuttingDown = false
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic if emitter crashes")
	}()

	go func() {
		daemon.Children.Add(1)
		defer daemon.Children.Done()

		assert.NotPanics(t, func() {
			for i := 0; i < 50; i++ {
				select {
				case stream <- &dipper.Message{
					Channel: "test",
					Subject: "noerror",
				}:
					dipper.Logger.Infof("written msg no. %+v", i)
					time.Sleep(10 * time.Millisecond)
				default:
					dipper.Logger.Infof("unable to write, server shutdown")
				}
			}
		}, "sending message to service should not panic when emitter crashes")
	}()
	time.Sleep(100 * time.Millisecond)
	// mark it as failed to avoid restarting the driver
	svc.driverRuntimes["emitter"].State = driver.DriverFailed
	// crash emitter
	close(emitterStream)
	emitterStream = nil
	time.Sleep(100 * time.Millisecond)

	daemon.ShutDown()
}

func TestServiceReplaceEmitter(t *testing.T) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}

	stream := make(chan *dipper.Message, 1)
	emitterStream := make(chan *dipper.Message, 1)
	svc := &Service{
		name: "testsvc",
		driverRuntimes: map[string]*driver.Runtime{
			"driver:d1": {
				Feature: "driver:d1",
				Handler: driver.NewNullDriver(&driver.Meta{
					Name: "d1",
					Type: "null",
				}),
				State:  driver.DriverAlive,
				Stream: stream,
			},
			"emitter": {
				Feature: "test-emitter",
				Handler: &driver.NullDriverHandler{
					SendMessageFunc: func(*dipper.Message) {
						if emitterStream == nil {
							panic("emitter crashed")
						}
					},
				},
				State:  driver.DriverAlive,
				Stream: emitterStream,
			},
		},
		Route: func(m *dipper.Message) []RoutedMessage {
			return nil
		},
	}
	daemon.Emitters["testsvc"] = svc

	daemon.ShuttingDown = false
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic if emitter is changed")
	}()

	go func() {
		daemon.Children.Add(1)
		defer daemon.Children.Done()

		assert.NotPanics(t, func() {
			for i := 0; i < 50; i++ {
				select {
				case stream <- &dipper.Message{
					Channel: "test",
					Subject: "noerror",
				}:
					dipper.Logger.Infof("written msg no. %+v", i)
					time.Sleep(10 * time.Millisecond)
				default:
					dipper.Logger.Infof("unable to write, server shutdown")
				}
			}
		}, "sending message to service should not panic when emitter is changed")
	}()

	newCfg := &config.Config{
		Services: []string{"testsvc"},
		Staged: &config.DataSet{
			Drivers: map[string]interface{}{
				"daemon": map[string]interface{}{
					"featureMap": map[string]interface{}{
						"emitter": "emitter2",
					},
					"features": map[string]interface{}{
						"global": []interface{}{
							map[string]interface{}{
								"name": "driver:d1",
							},
							map[string]interface{}{
								"name": "emitter",
							},
						},
					},
					"drivers": map[string]interface{}{
						"d1": map[string]interface{}{
							"name": "d1",
							"type": "null",
						},
						"emitter2": map[string]interface{}{
							"name": "emitter2",
							"type": "null",
						},
					},
				},
			},
		},
	}
	svc.config = newCfg
	svc.config.ResetStage()

	time.Sleep(100 * time.Millisecond)
	assert.NotPanics(t, svc.Reload, "service reload should not panic when emitter is changed")
	time.Sleep(100 * time.Millisecond)

	daemon.ShutDown()
}
