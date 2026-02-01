// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

// DefaultAPILockAttempt is the time for attempting to acquire a lock.
const DefaultAPILockAttempt = "20ms"

// DefaultAPILockExpireMS is the API candidate lock to expire.
const DefaultAPILockExpireMS = 1000

// Response is used for responding to the api service.
type Response struct {
	EventBus dipper.MessageReceiver
	Request  *dipper.Message
	Acked    bool

	Factrory *ResponseFactory
}

// Ack acks a call.
func (resp *Response) Ack() {
	resp.EventBus.SendMessage(&dipper.Message{
		Channel: "eventbus",
		Subject: "api",
		Labels: map[string]string{
			"type": "ack",
			"uuid": resp.Request.Labels["uuid"],
			"from": resp.Request.Labels["from"],
		},
	})
	resp.Acked = true
}

// Return returns data to api service.
func (resp *Response) Return(data interface{}) {
	resp.EventBus.SendMessage(&dipper.Message{
		Channel: "eventbus",
		Subject: "api",
		Labels: map[string]string{
			"type": "result",
			"uuid": resp.Request.Labels["uuid"],
			"from": resp.Request.Labels["from"],
		},
		Payload: data,
	})
	resp.Factrory.Live.Done()
}

// ReturnError returns an error to the API service.
func (resp *Response) ReturnError(err error) {
	resp.EventBus.SendMessage(&dipper.Message{
		Channel: "eventbus",
		Subject: "api",
		Labels: map[string]string{
			"type":  "result",
			"uuid":  resp.Request.Labels["uuid"],
			"from":  resp.Request.Labels["from"],
			"error": err.Error(),
		},
	})
	resp.Factrory.Live.Done()
}

// Lock is to compete for the right to handle a API call.
func (resp *Response) Lock(caller dipper.RPCCaller, def Def) bool {
	_, err := caller.Call("locker", "lock", map[string]interface{}{
		"name":   fmt.Sprintf("api_candidate:%s", resp.Request.Labels["uuid"]),
		"expire": strconv.Itoa(DefaultAPILockExpireMS) + "ms",
	}, "timeout", DefaultAPILockAttempt)

	return err == nil
}

// ResponseFactory provides functions to create new api Response.
type ResponseFactory struct {
	DefsByName map[string]Def
	Live       sync.WaitGroup
}

// NewResponseFactory creates a new response factory.
func NewResponseFactory() *ResponseFactory {
	r := &ResponseFactory{}
	r.DefsByName = GetDefsByName()

	return r
}

// NewResponse provides a function to create new api Response.
func (rf *ResponseFactory) NewResponse(caller dipper.RPCCaller, eventbus dipper.MessageReceiver, m *dipper.Message) *Response {
	resp := &Response{
		EventBus: eventbus,
		Request:  m,
		Factrory: rf,
	}

	method := m.Labels["fn"]
	def, ok := rf.DefsByName[method]
	if !ok {
		dipper.Logger.Warningf("Unknown API method: %s", method)

		return nil
	}
	switch def.ReqType {
	case TypeAll:
		go func() {
			defer dipper.SafeExitOnError("failed to send ack for api [%s]", method)
			resp.Ack()
		}()
	case TypeFirst:
		if !resp.Lock(caller, def) {
			return nil
		}
	case TypeMatch:
		// leave it to the function to send ack
	}

	rf.Live.Add(1)

	return resp
}
