// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"os"
	"strconv"
	"testing"

	"github.com/h2non/gock"
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

func TestSendRequest(t *testing.T) {

	defer gock.Off()

	gock.New("http://example.com").
		Get("/test").
		Reply(200).
		JSON(map[string]string{"foo": "bar"})

	request := &dipper.Message{
		Channel: "event",
		Subject: "command",
		Payload: map[string]interface{}{
			"URL": "http://example.com/test",
		},
		Reply: make(chan dipper.Message, 1),
	}
	sendRequest(request)
	response := <-request.Reply
	assert.Equal(t, "200", response.Payload.(map[string]interface{})["status_code"])
	assert.NotContains(t, response.Labels, "error")
	mapKey, _ := response.Payload.(map[string]interface{})["json"].(map[string]interface{})["foo"]
	assert.Equal(t, "bar", mapKey, "JSON data miss-match")
}

func TestRecieveListJson(t *testing.T) {

	defer gock.Off()

	gock.New("http://example.com").
		Get("/test").
		Reply(200).
		JSON([]string{"foo", "bar"})

	request := &dipper.Message{
		Channel: "event",
		Subject: "command",
		Payload: map[string]interface{}{
			"URL": "http://example.com/test",
		},
		Reply: make(chan dipper.Message, 1),
	}
	sendRequest(request)
	response := <-request.Reply
	assert.Equal(t, "200", response.Payload.(map[string]interface{})["status_code"])
	assert.NotContains(t, response.Labels, "error")
	firstElement := response.Payload.(map[string]interface{})["json"].([]interface{})[0]
	assert.Equal(t, "foo", firstElement, "JSON data mismatch")
}

func TestSendRequestInvalid(t *testing.T) {

	defer gock.Off()

	// Test that we get an error with various >= 400 status codes
	parameters := []int{
		400, 403, 500, 503,
	}

	for _, i := range parameters {
		gock.New("http://example.com").
			Get("/test").
			Reply(i).
			BodyString("This is broken")

		request := &dipper.Message{
			Channel: "event",
			Subject: "command",
			Payload: map[string]interface{}{
				"URL": "http://example.com/test",
			},
			Reply: make(chan dipper.Message, 1),
		}
		sendRequest(request)
		response := <-request.Reply
		assert.Equal(t, strconv.Itoa(i), response.Payload.(map[string]interface{})["status_code"])
		assert.Contains(t, response.Labels, "error")
		assert.NotNil(t, response.Labels["error"])
	}
}
