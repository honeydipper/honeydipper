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
	"fmt"
	"os"
	"testing"

	"cloud.google.com/go/pubsub"
	pubsubv2 "cloud.google.com/go/pubsub/v2"
	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	testProject          = "test"
	testSubscriptionName = "pubsub-test"
	testTopicID          = "test-topic"
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

func setupFakePubsub(t *testing.T) *pstest.Server {
	t.Helper()

	srv := pstest.NewServer()
	t.Cleanup(func() { srv.Close() })

	conn, err := grpc.Dial(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()
	topicName := fmt.Sprintf("projects/%s/topics/%s", testProject, testTopicID)
	adminClient, err := pubsubv2.NewClient(ctx, testProject, option.WithGRPCConn(conn))
	require.NoError(t, err)
	t.Cleanup(func() { adminClient.Close() })
	_, err = adminClient.TopicAdminClient.CreateTopic(ctx, &pb.Topic{Name: topicName})
	require.NoError(t, err)

	getPubsubClient = func(_ context.Context, _, proj string) *pubsubv2.Client {
		c, err := pubsubv2.NewClient(context.Background(), proj, option.WithGRPCConn(conn))
		if err != nil {
			panic(err)
		}

		return c
	}
	t.Cleanup(func() { getPubsubClient = newPubsubClient })

	return srv
}

func setupTestDriver(t *testing.T) {
	t.Helper()
	driver = dipper.NewDriver("operator", "gcloud-pubsub",
		dipper.DriverWithReader(&bytes.Buffer{}),
		dipper.DriverWithWriter(&bytes.Buffer{}),
	)
}

func TestPublishStringData(t *testing.T) {
	setupTestDriver(t)
	srv := setupFakePubsub(t)

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"project": testProject,
			"topic":   testTopicID,
			"data":    "hello world",
		},
		Reply: make(chan dipper.Message, 1),
	}

	publish(msg)

	reply := <-msg.Reply
	payload := reply.Payload.(map[string]interface{})
	assert.NotEmpty(t, payload["messageID"])

	msgs := srv.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, []byte("hello world"), msgs[0].Data)
}

func TestPublishJSONData(t *testing.T) {
	setupTestDriver(t)
	srv := setupFakePubsub(t)

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"project": testProject,
			"topic":   testTopicID,
			"data": map[string]interface{}{
				"key": "value",
				"num": 42,
			},
		},
		Reply: make(chan dipper.Message, 1),
	}

	publish(msg)

	reply := <-msg.Reply
	payload := reply.Payload.(map[string]interface{})
	assert.NotEmpty(t, payload["messageID"])

	msgs := srv.Messages()
	require.Len(t, msgs, 1)

	var decoded map[string]interface{}
	err := json.Unmarshal(msgs[0].Data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "value", decoded["key"])
}

func TestPublishWithAttributes(t *testing.T) {
	setupTestDriver(t)
	srv := setupFakePubsub(t)

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"project": testProject,
			"topic":   testTopicID,
			"data":    "test",
			"attributes": map[string]interface{}{
				"env":    "production",
				"source": "honeydipper",
			},
		},
		Reply: make(chan dipper.Message, 1),
	}

	publish(msg)

	reply := <-msg.Reply
	payload := reply.Payload.(map[string]interface{})
	assert.NotEmpty(t, payload["messageID"])

	msgs := srv.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "production", msgs[0].Attributes["env"])
	assert.Equal(t, "honeydipper", msgs[0].Attributes["source"])
}

func TestPublishMissingProject(t *testing.T) {
	setupTestDriver(t)
	setupFakePubsub(t)

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"topic": testTopicID,
			"data":  "test",
		},
		Reply: make(chan dipper.Message, 1),
	}

	assert.Panics(t, func() { publish(msg) })
}

func TestPublishMissingTopic(t *testing.T) {
	setupTestDriver(t)
	setupFakePubsub(t)

	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"project": testProject,
			"data":    "test",
		},
		Reply: make(chan dipper.Message, 1),
	}

	assert.Panics(t, func() { publish(msg) })
}

func TestStartOperatorSkipsSubscriptions(t *testing.T) {
	driver = dipper.NewDriver("operator", "gcloud-pubsub",
		dipper.DriverWithReader(&bytes.Buffer{}),
		dipper.DriverWithWriter(&bytes.Buffer{}),
	)

	assert.NotPanics(t, func() {
		start(&dipper.Message{})
	})
	assert.Nil(t, subscriberConfigs)
}
