// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package api

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/honeydipper/honeydipper/pkg/dipper/mock_dipper"
	"github.com/stretchr/testify/assert"
)

type ResponseTestCase struct {
	defsByName     map[string]Def
	shouldLock     bool
	name           string
	lockingError   error
	msg            *dipper.Message
	mockAPI        func(*Response, *ResponseTestCase)
	returnMessages []ReturnMessage
	noResponse     bool
}

func responseTest(t *testing.T, c *ResponseTestCase) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRPCCaller := mock_dipper.NewMockRPCCaller(ctrl)

	eventbusO, eventbusI := io.Pipe()
	defer eventbusO.Close()
	defer eventbusI.Close()
	waitForMsg := func(delay time.Duration) *dipper.Message {
		var m *dipper.Message
		msgAvailable := make(chan byte)
		go func() {
			defer recover()
			defer close(msgAvailable)
			m = dipper.FetchMessage(eventbusO)
			m.Size = 0 // ignore Size property, only used for raw msg
		}()
		select {
		case <-msgAvailable:
		case <-time.After(delay):
		}

		return m
	}

	if c.shouldLock {
		mockRPCCaller.EXPECT().Call(gomock.Eq("locker"), gomock.Eq("lock"), gomock.Eq(map[string]interface{}{
			"name":       fmt.Sprintf("api_candidate:%s", c.msg.Labels["uuid"]),
			"attempt_ms": 10,
			"expire":     "1000ms",
		})).Return(nil, c.lockingError)
	}

	factory := NewResponseFactory()
	factory.DefsByName = c.defsByName

	resp := factory.NewResponse(mockRPCCaller, eventbusI, c.msg)
	if c.noResponse {
		assert.Nil(t, resp)
	} else {
		go c.mockAPI(resp, c)

		for i, st := range c.returnMessages {
			if m := waitForMsg(st.Delay); m != nil {
				assert.Equal(t, st.Msg, m)
			} else {
				assert.Fail(t, fmt.Sprintf("timeout at step %d", i))
			}
		}
	}
}

func TestTypeAllAPIResponse(t *testing.T) {
	c := &ResponseTestCase{
		defsByName: map[string]Def{
			"test": {Name: "test", ReqType: TypeAll},
		},
		msg: &dipper.Message{
			Labels: map[string]string{
				"uuid": "test-uuid1",
				"from": "test-node1",
				"fn":   "test",
			},
			Payload: map[string]interface{}{
				"foo": "bar",
			},
		},
		mockAPI: func(r *Response, c *ResponseTestCase) {
			assert.Equal(t, "bar", dipper.MustGetMapData(r.Request.Payload, "foo"))
			time.Sleep(3 * time.Millisecond)
			r.Return(map[string]interface{}{
				"foo2": "bar2",
			})
		},
		returnMessages: []ReturnMessage{
			{
				Delay: 3000 * time.Millisecond,
				Msg: &dipper.Message{
					Channel: "eventbus",
					Subject: "api",
					Labels: map[string]string{
						"from": "test-node1",
						"type": "ack",
						"uuid": "test-uuid1",
					},
				},
			},
			{
				Delay: 3000 * time.Millisecond,
				Msg: &dipper.Message{
					Channel: "eventbus",
					Subject: "api",
					Labels: map[string]string{
						"from": "test-node1",
						"type": "result",
						"uuid": "test-uuid1",
					},
					Payload: map[string]interface{}{
						"foo2": "bar2",
					},
				},
			},
		},
	}

	responseTest(t, c)
}

func TestTypeFirstAPIResponse(t *testing.T) {
	c := &ResponseTestCase{
		defsByName: map[string]Def{
			"test": {Name: "test", ReqType: TypeFirst},
		},
		shouldLock: true,
		msg: &dipper.Message{
			Labels: map[string]string{
				"uuid": "test-uuid1",
				"from": "test-node1",
				"fn":   "test",
			},
			Payload: map[string]interface{}{
				"foo": "bar",
			},
		},
		mockAPI: func(r *Response, c *ResponseTestCase) {
			assert.Equal(t, "bar", dipper.MustGetMapData(r.Request.Payload, "foo"))
			time.Sleep(3 * time.Millisecond)
			r.Return(map[string]interface{}{
				"foo2": "bar2",
			})
		},
		returnMessages: []ReturnMessage{
			{
				Delay: 3000 * time.Millisecond,
				Msg: &dipper.Message{
					Channel: "eventbus",
					Subject: "api",
					Labels: map[string]string{
						"from": "test-node1",
						"type": "result",
						"uuid": "test-uuid1",
					},
					Payload: map[string]interface{}{
						"foo2": "bar2",
					},
				},
			},
		},
	}

	responseTest(t, c)
}

func TestTypeMatchAPIResponse(t *testing.T) {
	c := &ResponseTestCase{
		defsByName: map[string]Def{
			"test": {Name: "test", ReqType: TypeMatch},
		},
		msg: &dipper.Message{
			Labels: map[string]string{
				"uuid": "test-uuid1",
				"from": "test-node1",
				"fn":   "test",
			},
			Payload: map[string]interface{}{
				"foo": "bar",
			},
		},
		mockAPI: func(r *Response, c *ResponseTestCase) {
			assert.Equal(t, "bar", dipper.MustGetMapData(r.Request.Payload, "foo"))
			time.Sleep(3 * time.Millisecond)
			r.Ack()
			time.Sleep(3 * time.Millisecond)
			r.Return(map[string]interface{}{
				"foo2": "bar2",
			})
		},
		returnMessages: []ReturnMessage{
			{
				Delay: 3000 * time.Millisecond,
				Msg: &dipper.Message{
					Channel: "eventbus",
					Subject: "api",
					Labels: map[string]string{
						"from": "test-node1",
						"type": "ack",
						"uuid": "test-uuid1",
					},
				},
			},
			{
				Delay: 3000 * time.Millisecond,
				Msg: &dipper.Message{
					Channel: "eventbus",
					Subject: "api",
					Labels: map[string]string{
						"from": "test-node1",
						"type": "result",
						"uuid": "test-uuid1",
					},
					Payload: map[string]interface{}{
						"foo2": "bar2",
					},
				},
			},
		},
	}

	responseTest(t, c)
}

func TestTypeMatchAPIResponseReturnError(t *testing.T) {
	c := &ResponseTestCase{
		defsByName: map[string]Def{
			"test": {Name: "test", ReqType: TypeMatch},
		},
		msg: &dipper.Message{
			Labels: map[string]string{
				"uuid": "test-uuid1",
				"from": "test-node1",
				"fn":   "test",
			},
			Payload: map[string]interface{}{
				"foo": "bar",
			},
		},
		mockAPI: func(r *Response, c *ResponseTestCase) {
			assert.Equal(t, "bar", dipper.MustGetMapData(r.Request.Payload, "foo"))
			time.Sleep(3 * time.Millisecond)
			r.Ack()
			time.Sleep(3 * time.Millisecond)
			r.ReturnError(fmt.Errorf("test error"))
		},
		returnMessages: []ReturnMessage{
			{
				Delay: 3000 * time.Millisecond,
				Msg: &dipper.Message{
					Channel: "eventbus",
					Subject: "api",
					Labels: map[string]string{
						"from": "test-node1",
						"type": "ack",
						"uuid": "test-uuid1",
					},
				},
			},
			{
				Delay: 3000 * time.Millisecond,
				Msg: &dipper.Message{
					Channel: "eventbus",
					Subject: "api",
					Labels: map[string]string{
						"from":  "test-node1",
						"type":  "result",
						"uuid":  "test-uuid1",
						"error": "test error",
					},
				},
			},
		},
	}

	responseTest(t, c)
}

func TestAPIResponseUnknownAPI(t *testing.T) {
	c := &ResponseTestCase{
		defsByName: map[string]Def{
			"test": {Name: "test", ReqType: TypeMatch},
		},
		msg: &dipper.Message{
			Labels: map[string]string{
				"uuid": "test-uuid1",
				"from": "test-node1",
				"fn":   "test1",
			},
			Payload: map[string]interface{}{
				"foo": "bar",
			},
		},
		noResponse: true,
	}

	responseTest(t, c)
}

func TestAPIResponseLockFailure(t *testing.T) {
	c := &ResponseTestCase{
		defsByName: map[string]Def{
			"test": {Name: "test", ReqType: TypeFirst},
		},
		shouldLock:   true,
		lockingError: fmt.Errorf("busy"),
		msg: &dipper.Message{
			Labels: map[string]string{
				"uuid": "test-uuid1",
				"from": "test-node1",
				"fn":   "test",
			},
			Payload: map[string]interface{}{
				"foo": "bar",
			},
		},
		noResponse: true,
	}

	responseTest(t, c)
}
