// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"cloud.google.com/go/pubsub"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

const (
	testProject          = "test"
	testSubscriptionName = "pubsub-test"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		logFile, err := os.Create("test.log")
		if err != nil {
			panic(err)
		}
		defer logFile.Close()
		dipper.Logger = dipper.GetLogger("test", "INFO", logFile, logFile)
	}
	driver = &dipper.Driver{Service: "test"}
	m.Run()
}

func TestMsgHandlerMatchJsonRule(t *testing.T) {
	messages := []struct {
		Message map[string]string
		Want    map[string]interface{}
	}{
		{
			Message: map[string]string{
				"key1": "value1",
			},
			Want: map[string]interface{}{
				"project":          testProject,
				"subscriptionName": testSubscriptionName,
				"json": map[string]interface{}{
					"key1": "value1",
				},
			},
		},
		{
			Message: map[string]string{
				"key2": "value2",
			},
			Want: map[string]interface{}{
				"project":          testProject,
				"subscriptionName": testSubscriptionName,
				"json": map[string]interface{}{
					"key2": "value2",
				},
			},
		},
	}

	testConfig := &SubscriberConfig{
		Project:          testProject,
		SubscriptionName: testSubscriptionName,
		Conditions: []interface{}{
			map[string]interface{}{
				"project":          testProject,
				"subscriptionName": testSubscriptionName,
				"json": map[string]interface{}{
					"key1": "value1",
				},
			},
			map[string]interface{}{
				"project":          testProject,
				"subscriptionName": testSubscriptionName,
				"json": map[string]interface{}{
					"key2": "value2",
				},
			},
		},
	}

	msgFuncTest := msgHandlerBuilder(testConfig)
	ctx := context.Background()

	for _, m := range messages {
		byteMsg, err := json.Marshal(m.Message)
		if err != nil {
			panic(err)
		}

		pbMsg := &pubsub.Message{
			Data: byteMsg,
		}
		buffer := &bytes.Buffer{}
		driver.Out = buffer

		msgFuncTest(ctx, pbMsg)
		dmsg := dipper.FetchMessage(buffer)
		data := dmsg.Payload.(map[string]interface{})["data"]
		assert.Equalf(t, m.Want, data, "Driver message Payload dis-match")
	}
}

func TestMsgHandlerMatchTextRule(t *testing.T) {
	msg := "test message"

	messages := []struct {
		Message string
		Want    map[string]interface{}
	}{
		{
			Message: msg,
			Want: map[string]interface{}{
				"project":          testProject,
				"subscriptionName": testSubscriptionName,
				"text":             msg,
			},
		},
	}

	testConfig := &SubscriberConfig{
		Project:          testProject,
		SubscriptionName: testSubscriptionName,
		Conditions: []interface{}{
			map[string]interface{}{
				"project":          testProject,
				"subscriptionName": testSubscriptionName,
				"text":             msg,
			},
		},
	}

	msgFuncTest := msgHandlerBuilder(testConfig)
	ctx := context.Background()

	for _, m := range messages {
		pbMsg := &pubsub.Message{
			Data: []byte(m.Message),
		}
		buffer := &bytes.Buffer{}
		driver.Out = buffer

		msgFuncTest(ctx, pbMsg)
		dmsg := dipper.FetchMessage(buffer)
		data := dmsg.Payload.(map[string]interface{})["data"]
		assert.Equalf(t, m.Want, data, "Driver message Payload dis-match")
	}
}

func TestMsgHandlerDontMatchJsonRule(t *testing.T) {
	messages := []map[string]string{
		{},
		{"key1": "value2"},
	}
	testConfig := &SubscriberConfig{
		Project:          testProject,
		SubscriptionName: testSubscriptionName,
		Conditions: []interface{}{
			map[string]interface{}{
				"project":          testProject,
				"subscriptionName": testSubscriptionName,
				"json": map[string]interface{}{
					"key1": "value1",
				},
			},
		},
	}

	msgFuncTest := msgHandlerBuilder(testConfig)
	ctx := context.Background()

	for _, m := range messages {
		byteMsg, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}

		pbMsg := &pubsub.Message{
			Data: byteMsg,
		}
		buffer := &bytes.Buffer{}
		driver.Out = buffer

		msgFuncTest(ctx, pbMsg)
		assert.Equalf(t, 0, buffer.Len(), "Driver buffer is not empty")
	}
}
