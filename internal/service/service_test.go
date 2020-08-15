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
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}

	svc := &Service{
		name: "testsvc",
		driverRuntimes: map[string]*driver.Runtime{
			"d1": {
				State: driver.DriverAlive,
				Handler: driver.NewDriver(map[string]interface{}{
					"name": "testdriver1",
					"type": "builtin",
					"handlerData": map[string]interface{}{
						"shortName": "testdriver1",
					},
				}),
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

	svc.driverRuntimes["d1"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
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
	svc.driverRuntimes["d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
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
	svc.driverRuntimes["d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
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
	svc.driverRuntimes["d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
	// injecting error in process
	svc.driverRuntimes["d1"].Handler = nil
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic in process itself")
	}()
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
		Channel: "test",
		Subject: "error3",
	}
	// recover the service object to avoid crash during quiting
	svc.driverRuntimes["d1"].Handler = driver.NewDriver(map[string]interface{}{
		"name": "testdriver1",
		"type": "builtin",
		"handlerData": map[string]interface{}{
			"shortName": "testdriver1",
		},
	})
	time.Sleep(30 * time.Millisecond)
	// quiting faster by send an extra message
	svc.driverRuntimes["d1"].Stream <- dipper.Message{
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

	svc := &Service{
		name: "testsvc",
		driverRuntimes: map[string]*driver.Runtime{
			"driver:d1": {
				State: driver.DriverAlive,
				Handler: driver.NewDriver(map[string]interface{}{
					"name": "d1",
					"type": "builtin",
					"handlerData": map[string]interface{}{
						"shortName": "testdriver1",
					},
				}),
			},
			"emitter": {
				State: driver.DriverAlive,
				Handler: driver.NewDriver(map[string]interface{}{
					"name": "test-emitter",
					"type": "builtin",
					"handlerData": map[string]interface{}{
						"shortName": "testdriver1",
					},
				}),
				Feature: "emitter",
			},
		},
		Route: func(m *dipper.Message) []RoutedMessage {
			return nil
		},
	}
	daemon.Emitters["testsvc"] = svc

	daemon.ShuttingDown = false
	svc.driverRuntimes["driver:d1"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["driver:d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
	svc.driverRuntimes["emitter"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["emitter"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND|os.O_WRONLY, 0o777)
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic if emitter is removed")
	}()

	go func() {
		daemon.Children.Add(1)
		defer daemon.Children.Done()

		assert.NotPanics(t, func() {
			for i := 0; i < 50; i = i + 1 {
				select {
				case svc.driverRuntimes["driver:d1"].Stream <- dipper.Message{
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
		DataSet: &config.DataSet{
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
							"type": "builtin",
						},
					},
				},
			},
		},
	}
	svc.config = newCfg

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

	svc := &Service{
		name: "testsvc",
		driverRuntimes: map[string]*driver.Runtime{
			"driver:d1": {
				State: driver.DriverAlive,
				Handler: driver.NewDriver(map[string]interface{}{
					"name": "d1",
					"type": "builtin",
					"handlerData": map[string]interface{}{
						"shortName": "testdriver1",
					},
				}),
			},
			"emitter": {
				State: driver.DriverAlive,
				Handler: driver.NewDriver(map[string]interface{}{
					"name": "test-emitter",
					"type": "builtin",
					"handlerData": map[string]interface{}{
						"shortName": "testdriver1",
					},
				}),
				Feature: "emitter",
			},
		},
		Route: func(m *dipper.Message) []RoutedMessage {
			return nil
		},
	}
	daemon.Emitters["testsvc"] = svc

	daemon.ShuttingDown = false
	svc.driverRuntimes["driver:d1"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["driver:d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
	svc.driverRuntimes["emitter"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["emitter"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND|os.O_WRONLY, 0o777)
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic if emitter crashes")
	}()

	go func() {
		daemon.Children.Add(1)
		defer daemon.Children.Done()

		assert.NotPanics(t, func() {
			for i := 0; i < 50; i = i + 1 {
				select {
				case svc.driverRuntimes["driver:d1"].Stream <- dipper.Message{
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
	svc.driverRuntimes["emitter"].Output.Close()
	close(svc.driverRuntimes["emitter"].Stream)
	time.Sleep(100 * time.Millisecond)

	daemon.ShutDown()
}

func TestServiceReplaceEmitter(t *testing.T) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}

	svc := &Service{
		name: "testsvc",
		driverRuntimes: map[string]*driver.Runtime{
			"driver:d1": {
				State: driver.DriverAlive,
				Handler: driver.NewDriver(map[string]interface{}{
					"name": "d1",
					"type": "builtin",
					"handlerData": map[string]interface{}{
						"shortName": "testdriver1",
					},
				}),
			},
			"emitter": {
				State: driver.DriverAlive,
				Handler: driver.NewDriver(map[string]interface{}{
					"name": "test-emitter",
					"type": "builtin",
					"handlerData": map[string]interface{}{
						"shortName": "testdriver1",
					},
				}),
				Feature: "emitter",
			},
		},
		Route: func(m *dipper.Message) []RoutedMessage {
			return nil
		},
	}
	daemon.Emitters["testsvc"] = svc

	daemon.ShuttingDown = false
	svc.driverRuntimes["driver:d1"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["driver:d1"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
	svc.driverRuntimes["emitter"].Stream = make(chan dipper.Message, 1)
	svc.driverRuntimes["emitter"].Output, _ = os.OpenFile(os.DevNull, os.O_APPEND|os.O_WRONLY, 0o777)
	go func() {
		assert.NotPanics(t, svc.serviceLoop, "service loop should recover panic if emitter is changed")
	}()

	go func() {
		daemon.Children.Add(1)
		defer daemon.Children.Done()

		assert.NotPanics(t, func() {
			for i := 0; i < 50; i = i + 1 {
				select {
				case svc.driverRuntimes["driver:d1"].Stream <- dipper.Message{
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
		DataSet: &config.DataSet{
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
						},
						"emitter2": map[string]interface{}{
							"name": "emitter2",
						},
					},
				},
			},
		},
	}
	svc.config = newCfg

	time.Sleep(100 * time.Millisecond)
	assert.NotPanics(t, svc.Reload, "service reload should not panic when emitter is changed")
	time.Sleep(100 * time.Millisecond)

	daemon.ShutDown()
}
