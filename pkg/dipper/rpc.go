// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import (
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	// TimeoutError indicates a timeout error
	TimeoutError Error = "timeout"

	// RPCError indicates errors happened during RPC call
	RPCError Error = "rpc error"

	// DefaultRPCTimeout is the default timeout in seconds for RPC calls
	DefaultRPCTimeout time.Duration = 10
)

// RPCHandler : a type of functions that handle RPC calls between drivers.
type RPCHandler func(string, string, []byte)

// RPCCallerStub is an interface which every RPC caller should implement.
type RPCCallerStub interface {
	GetName() string
	GetStream(feature string) io.Writer
}

// RPCCaller defines all method required for making rpc alls.
type RPCCaller interface {
	Call(feature string, method string, params interface{}) ([]byte, error)
	CallNoWait(feature string, method string, params interface{}) error
	CallRaw(feature string, method string, params []byte) ([]byte, error)
	CallRawNoWait(feature string, method string, params []byte, rpcID string) (ret error)
	GetName() string
}

// RPCCallerBase : an object that makes RPC calls.
type RPCCallerBase struct {
	Parent  RPCCallerStub
	Channel string
	Subject string
	Result  map[string]chan interface{}
	Lock    sync.Mutex
	Counter int
}

// Init : initializing rpc caller.
func (c *RPCCallerBase) Init(parent RPCCallerStub, channel string, subject string) {
	c.Result = map[string]chan interface{}{}
	InitIDMap(&c.Result)
	c.Counter = 0
	c.Channel = channel
	c.Subject = subject
	c.Parent = parent
}

// GetName returns the name of the call for logging purpose.
func (c *RPCCallerBase) GetName() string {
	return c.Parent.GetName()
}

// Call : making a RPC call to another driver with structured data.
func (c *RPCCallerBase) Call(feature string, method string, params interface{}) ([]byte, error) {
	ret, err := c.CallRaw(feature, method, SerializeContent(params))
	return ret, err
}

// CallNoWait : making a RPC call to another driver with structured data not expecting any return.
func (c *RPCCallerBase) CallNoWait(feature string, method string, params interface{}) error {
	return c.CallRawNoWait(feature, method, SerializeContent(params), "skip")
}

// CallRaw : making a RPC call to another driver with raw data.
func (c *RPCCallerBase) CallRaw(feature string, method string, params []byte) ([]byte, error) {
	// keep track the call in the map
	result := make(chan interface{}, 1)
	rpcID := IDMapPut(&c.Result, result)
	defer IDMapDel(&c.Result, rpcID)

	if err := c.CallRawNoWait(feature, method, params, rpcID); err != nil {
		return nil, err
	}

	// waiting for the result to come back
	select {
	case msg := <-result:
		if msg == nil {
			return nil, nil
		} else if e, ok := msg.(error); ok {
			return nil, e
		}
		return msg.([]byte), nil
	case <-time.After(time.Second * DefaultRPCTimeout):
		return nil, TimeoutError
	}
}

// CallRawNoWait : making a RPC call to another driver with raw data not expecting return.
func (c *RPCCallerBase) CallRawNoWait(feature string, method string, params []byte, rpcID string) (ret error) {
	defer func() {
		if r := recover(); r != nil {
			ret = r.(error)
		}
	}()

	if rpcID == "" {
		rpcID = "skip"
	}

	out := c.Parent.GetStream(feature)
	if out == nil {
		return fmt.Errorf("feature not available: %s: %w", feature, RPCError)
	}

	// making the call by sending a message
	SendMessage(out, &Message{
		Channel: c.Channel,
		Subject: c.Subject,
		Labels: map[string]string{
			"rpcID":   rpcID,
			"feature": feature,
			"method":  method,
			"caller":  "-",
		},
		Payload: params,
		IsRaw:   true,
	})

	return nil
}

// HandleReturn : receiving return of a RPC call.
func (c *RPCCallerBase) HandleReturn(m *Message) {
	rpcID := m.Labels["rpcID"]
	result := IDMapGet(&c.Result, rpcID).(chan interface{})

	reason, ok := m.Labels["error"]

	if ok {
		result <- fmt.Errorf("reason: %s: %w", reason, RPCError)
	} else {
		result <- m.Payload
	}
}

// RPCProvider : an interface for providing RPC handling feature.
type RPCProvider struct {
	RPCHandlers   map[string]MessageHandler
	DefaultReturn io.Writer
	Channel       string
	Subject       string
}

// Init : initializing rpc provider.
func (p *RPCProvider) Init(channel string, subject string, defaultWriter io.Writer) {
	p.RPCHandlers = map[string]MessageHandler{}
	p.DefaultReturn = defaultWriter
	p.Channel = channel
	p.Subject = subject
}

// ReturnError : return error to rpc caller.
func (p *RPCProvider) ReturnError(call *Message, reason string) {
	returnTo := call.ReturnTo
	if returnTo == nil {
		returnTo = p.DefaultReturn
	}
	SendMessage(returnTo, &Message{
		Channel: p.Channel,
		Subject: p.Subject,
		Labels: map[string]string{
			"rpcID":  call.Labels["rpcID"],
			"caller": call.Labels["caller"],
			"error":  reason,
		},
	})
}

// Return : return a value to rpc caller.
func (p *RPCProvider) Return(call *Message, retval *Message) {
	returnTo := call.ReturnTo
	if returnTo == nil {
		returnTo = p.DefaultReturn
	}
	SendMessage(returnTo, &Message{
		Channel: p.Channel,
		Subject: p.Subject,
		Labels: map[string]string{
			"rpcID":  call.Labels["rpcID"],
			"caller": call.Labels["caller"],
		},
		Payload: retval.Payload,
		IsRaw:   retval.IsRaw,
	})
}

// Router : route the message to rpc handlers.
func (p *RPCProvider) Router(msg *Message) {
	method := msg.Labels["method"]
	f := p.RPCHandlers[method]

	if msg.Labels["rpcID"] != "skip" {
		msg.Reply = make(chan Message, 1)

		go func() {
			defer close(msg.Reply)
			select {
			case reply := <-msg.Reply:
				if reason, ok := reply.Labels["error"]; ok {
					p.ReturnError(msg, reason)
				} else {
					p.Return(msg, &reply)
				}
			case <-time.After(time.Second * DefaultRPCTimeout):
				p.ReturnError(msg, "timeout")
			}
		}()

		defer func() {
			if r := recover(); r != nil {
				msg.Reply <- Message{
					Labels: map[string]string{
						"error": fmt.Sprintf("%+v", r),
					},
				}
				panic(r)
			}
		}()
	}
	f(msg)
}
