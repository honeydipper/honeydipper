// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package api

import (
	"fmt"
	"io"
	"strconv"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// DefaultAPILockAttemptMS is the time for attempting to acquire a lock.
const DefaultAPILockAttemptMS = 10

// DefaultAPILockExpireMS is the API candidate lock to expire.
const DefaultAPILockExpireMS = 1000

// Response is used for responding to the api service.
type Response struct {
	EventBus io.Writer
	Request  *dipper.Message
	Acked    bool
}

// Ack acks a call.
func (resp *Response) Ack() {
	dipper.SendMessage(resp.EventBus, &dipper.Message{
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
	dipper.SendMessage(resp.EventBus, &dipper.Message{
		Channel: "eventbus",
		Subject: "api",
		Labels: map[string]string{
			"type": "result",
			"uuid": resp.Request.Labels["uuid"],
			"from": resp.Request.Labels["from"],
		},
		Payload: data,
	})
}

// ReturnError returns an error to the API service.
func (resp *Response) ReturnError(err error) {
	dipper.SendMessage(resp.EventBus, &dipper.Message{
		Channel: "eventbus",
		Subject: "api",
		Labels: map[string]string{
			"type":  "result",
			"uuid":  resp.Request.Labels["uuid"],
			"from":  resp.Request.Labels["from"],
			"error": err.Error(),
		},
	})
}

// Lock is to compete for the right to handle a API call.
func (resp *Response) Lock(caller dipper.RPCCaller, def Def) bool {
	_, err := caller.Call("locker", "lock", map[string]interface{}{
		"name":       fmt.Sprintf("api_candidate:%s", resp.Request.Labels["uuid"]),
		"attempt_ms": DefaultAPILockAttemptMS,
		"expire":     strconv.Itoa(DefaultAPILockExpireMS) + "ms",
	})

	return err == nil
}

// ResponseFactory provides functions to create new api Response.
type ResponseFactory struct {
	DefsByName map[string]Def
}

// NewResponseFactory creates a new response factory.
func NewResponseFactory() *ResponseFactory {
	r := &ResponseFactory{}
	r.DefsByName = GetDefsByName()
	return r
}

// NewResponse provides a function to create new api Response.
func (rf *ResponseFactory) NewResponse(caller dipper.RPCCaller, eventbus io.Writer, m *dipper.Message) *Response {
	resp := &Response{
		EventBus: eventbus,
		Request:  m,
	}

	method := m.Labels["fn"]
	def, ok := rf.DefsByName[method]
	if !ok {
		dipper.Logger.Warningf("Unknown API method: %s", method)
		return nil
	}
	switch def.reqType {
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

	return resp
}
