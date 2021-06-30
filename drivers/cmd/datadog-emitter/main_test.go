// Copyright 2021 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package main

import (
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	mock_driver "github.com/honeydipper/honeydipper/drivers/cmd/datadog-emitter/mock_datadog-emitter"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

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
	daemonID = "1.1.1.1"

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
	daemonID = "1.1.1.1"

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
	daemonID = "1.1.1.1"

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
