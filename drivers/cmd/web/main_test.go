// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package main

import (
	"bytes"
	"flag"
	"os"
	"strconv"
	"testing"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
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

func TestSendRequestMultipleQueryValue(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").
		Get("/test").
		MatchParam("key_multi", "string1").
		MatchParam("key_multi", "string2").
		MatchParam("key_single", "only_string").
		Reply(200).
		JSON(map[string]string{"foo": "bar"})

	request := &dipper.Message{
		Channel: "event",
		Subject: "command",
		Payload: map[string]interface{}{
			"URL": "http://example.com/test",
			"form": map[string]interface{}{
				"key_single": "only_string",
				"key_multi": []interface{}{
					"string1",
					"string2",
				},
			},
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

func TestSendRequestPostJSONString(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").
		Post("/test").
		MatchType("application/json").
		Body(bytes.NewBufferString(`{"incoming": "foobar"}`)).
		Reply(200).
		JSON(map[string]string{"foo": "bar"})

	request := &dipper.Message{
		Channel: "event",
		Subject: "command",
		Payload: map[string]interface{}{
			"method": "POST",
			"URL":    "http://example.com/test",
			"header": map[string]interface{}{
				"Content-Type": "application/json",
			},
			"content": `{"incoming": "foobar"}`,
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

func TestSendRequestPostJSON(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").
		Post("/test").
		MatchType("application/json").
		Body(bytes.NewBufferString(`{"incoming":"foobar"}`)).
		Reply(200).
		JSON(map[string]string{"foo": "bar"})

	request := &dipper.Message{
		Channel: "event",
		Subject: "command",
		Payload: map[string]interface{}{
			"method": "POST",
			"URL":    "http://example.com/test",
			"header": map[string]interface{}{
				"Content-Type": "application/json",
			},
			"content": map[string]interface{}{
				"incoming": "foobar",
			},
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

func TestSendRequestPostJSONForm(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").
		Post("/test").
		MatchType("application/json").
		Body(bytes.NewBufferString(`{"incoming":"foobar"}`)).
		Reply(200).
		JSON(map[string]string{"foo": "bar"})

	request := &dipper.Message{
		Channel: "event",
		Subject: "command",
		Payload: map[string]interface{}{
			"method": "POST",
			"URL":    "http://example.com/test",
			"header": map[string]interface{}{
				"Content-Type": "application/json",
			},
			"form": map[string]interface{}{
				"incoming": "foobar",
			},
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

func TestSendRequestPostForm(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").
		Post("/test").
		Body(bytes.NewBufferString(`post_field1=string1&post_field2=string2`)).
		Reply(200).
		JSON(map[string]string{"foo": "bar"})

	request := &dipper.Message{
		Channel: "event",
		Subject: "command",
		Payload: map[string]interface{}{
			"method": "POST",
			"URL":    "http://example.com/test",
			"content": map[string]interface{}{
				"post_field1": "string1",
				"post_field2": "string2",
			},
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

func TestSendRequestPostFormForm(t *testing.T) {
	defer gock.Off()

	initFlags()
	flag.Usage()
	gock.New("http://example.com").
		Post("/test").
		Body(bytes.NewBufferString(`post_field1=string1&post_field2=string2`)).
		Reply(200).
		AddHeader("Set-Cookie", "mycookie=123").
		JSON(map[string]string{"foo": "bar"})

	request := &dipper.Message{
		Channel: "event",
		Subject: "command",
		Payload: map[string]interface{}{
			"method": "POST",
			"URL":    "http://example.com/test",
			"form": map[string]interface{}{
				"post_field1": "string1",
				"post_field2": "string2",
			},
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
