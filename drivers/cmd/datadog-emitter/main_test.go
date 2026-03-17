// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package main

import (
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	mock_driver "github.com/honeydipper/honeydipper/v3/drivers/cmd/datadog-emitter/mock_datadog-emitter"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

const daemonIDString = "1.1.1.1"

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		logFile, err := os.Create("test.log")
		if err != nil {
			panic(err)
		}
		defer logFile.Close()
		dipper.GetLogger("test", "INFO", logFile, logFile)
	}
	driver = &dipper.Driver{Service: "test"}
	m.Run()
}

func TestIncrCmd(t *testing.T) {
	ctrl := gomock.NewController(t)
	_mockedstatsd := mock_driver.NewMockvirtualStatsd(ctrl)
	dogstatsd = _mockedstatsd
	daemonID = daemonIDString

	inMsg := &dipper.Message{
		Payload: map[string]interface{}{
			"name": "test.test.counter",
			"tags": []interface{}{
				"app:test-app",
			},
		},
		Reply: make(chan dipper.Message, 10),
	}
	_mockedstatsd.EXPECT().Incr(gomock.Eq("test.test.counter"), gomock.Eq([]string{"daemon-id:1.1.1.1", "app:test-app"}), float64(1)).Times(1).Return(nil)
	assert.NotPanics(t, func() { counterIncr(inMsg) }, "counterIncr should not panic")
	outMsg := <-inMsg.Reply
	assert.Equal(t, dipper.Message{}, outMsg, "counterIncr should return an empty message upon success")
}

func TestGaugeCmd(t *testing.T) {
	ctrl := gomock.NewController(t)
	_mockedstatsd := mock_driver.NewMockvirtualStatsd(ctrl)
	dogstatsd = _mockedstatsd
	daemonID = daemonIDString

	inMsg := &dipper.Message{
		Payload: map[string]interface{}{
			"name":  "test.test.counter",
			"value": "0.6",
			"tags": []interface{}{
				"app:test-app",
			},
		},
		Reply: make(chan dipper.Message, 10),
	}
	_mockedstatsd.EXPECT().Gauge(gomock.Eq("test.test.counter"), float64(0.6), gomock.Eq([]string{"daemon-id:1.1.1.1", "app:test-app"}), float64(1)).Times(1).Return(nil)
	assert.NotPanics(t, func() { gaugeSet(inMsg) }, "gaugeSet should not panic")
	outMsg := <-inMsg.Reply
	assert.Equal(t, dipper.Message{}, outMsg, "gaugeSet should return an empty message upon success")
}

func TestLoadOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	_mockedstatsd := mock_driver.NewMockvirtualStatsd(ctrl)
	mockedstatsd = _mockedstatsd
	dogstatsd = _mockedstatsd
	daemonID = daemonIDString

	driver.Options = map[string]interface{}{
		"data": map[string]interface{}{
			"statsdHost": "2.2.2.2",
			"statsdPort": "1234",
		},
	}

	_mockedstatsd.EXPECT().Close().Times(1)
	_mockedstatsd.EXPECT().Event(gomock.Any()).Times(1).Return(nil)
	assert.NotPanics(t, func() { loadOptions(&dipper.Message{}) }, "loadOptions should not panic")
}
