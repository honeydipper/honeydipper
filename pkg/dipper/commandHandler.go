// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

const (
	// SUCCESS means the workflow finished successfully
	SUCCESS = "success"
	// FAILURE means the workflow finished with failure status
	FAILURE = "failure"
	// ERROR means the workflow run into errors and could not complete
	ERROR = "error"
)

// CommandProvider : an interface for providing Command handling feature
type CommandProvider struct {
	Commands     map[string]MessageHandler
	ReturnWriter io.Writer
	Channel      string
	Subject      string
}

// Init : initializing rpc provider
func (p *CommandProvider) Init(channel string, subject string, defaultWriter io.Writer) {
	p.Commands = map[string]MessageHandler{}
	p.ReturnWriter = defaultWriter
	p.Channel = channel
	p.Subject = subject
}

// ReturnError sends an error message return to caller and create an error
func (p *CommandProvider) ReturnError(call *Message, pattern string, args ...interface{}) error {
	errText := fmt.Sprintf(pattern, args...)
	p.Return(call, &Message{
		Labels: map[string]string{
			"error": errText,
		},
	})
	Logger.Warningf("[operator] %s", errText)
	return Error(errText)
}

// Return : return a value to rpc caller
func (p *CommandProvider) Return(call *Message, retval *Message) {
	defer func() {
		call.Reply = nil
	}()

	if _, ok := call.Labels["sessionID"]; !ok {
		return
	}

	retMsg := &Message{
		Channel: p.Channel,
		Subject: p.Subject,
		Labels:  call.Labels,
	}
	delete(retMsg.Labels, "backoff_ms")
	delete(retMsg.Labels, "retry")
	delete(retMsg.Labels, "timeout")
	if status, ok := retval.Labels["status"]; ok {
		retMsg.Labels["status"] = status
		if status != SUCCESS {
			retMsg.Labels["reason"] = retval.Labels["reason"]
		}
	} else {
		if reason, ok := retval.Labels["error"]; ok {
			retMsg.Labels["status"] = "error"
			retMsg.Labels["reason"] = reason
		} else {
			retMsg.Labels["status"] = SUCCESS
		}
	}
	retMsg.Payload = retval.Payload
	retMsg.IsRaw = retval.IsRaw
	SendMessage(p.ReturnWriter, retMsg)
}

// Router : route the message to rpc handlers
func (p *CommandProvider) Router(msg *Message) {
	method := msg.Labels["method"]
	f, ok := p.Commands[method]
	if !ok {
		panic(p.ReturnError(msg, "[operator] cmd not defined: %s", method))
	}

	retry, timeout, backoff := p.UnpackLabels(msg)

	var attempt func(chan Message)
	attempt = func(replyChannel chan Message) {
		msg.Reply = replyChannel
		m := *msg

		go func() {
			defer func() {
				close(replyChannel)
				replyChannel = nil
			}()

			Logger.Debugf("[operaotr] cmd labels %+v", m.Labels)

			select {
			case reply := <-replyChannel:
				if _, ok := reply.Labels["no-timeout"]; ok {
					reply = <-m.Reply
				}

				_, hasError := reply.Labels["error"]
				if status, ok := reply.Labels["status"]; (hasError || (ok && status != SUCCESS)) && retry > 0 {
					Logger.Debugf("[operaotr] %d retry left for method %s", retry, method)
					retry--
					time.Sleep(backoff * time.Millisecond)
					backoff *= 2
					go attempt(make(chan Message, 1))
				} else {
					p.Return(msg, &reply)
				}
			case <-time.After(time.Second * timeout):
				_ = p.ReturnError(msg, "timeout")
			}
		}()

		defer func() {
			if r := recover(); r != nil && replyChannel != nil {
				replyChannel <- Message{
					Labels: map[string]string{
						"error": fmt.Sprintf("%+v", r),
					},
				}
			}
		}()
		f(&m)
	}

	attempt(make(chan Message, 1))
}

// UnpackLabels loads necessary variables out of the labels
func (p *CommandProvider) UnpackLabels(msg *Message) (retry int, timeout, backoffms time.Duration) {
	var err error

	retryStr, _ := msg.Labels["retry"]
	if retryStr != "" {
		retry, err = strconv.Atoi(retryStr)
		if err != nil {
			panic(p.ReturnError(msg, "[operator] invalid retry: %s", retryStr))
		}
	}

	backoffStr, _ := msg.Labels["backoff_ms"]
	if backoffStr != "" {
		backoffVal, err := strconv.Atoi(backoffStr)
		if err != nil {
			panic(p.ReturnError(msg, "[operator] invalid backoff_ms: %s", backoffStr))
		}
		backoffms = time.Duration(backoffVal)
	} else {
		backoffms = 1000
	}

	timeoutStr, _ := msg.Labels["timeout"]
	if timeoutStr != "" {
		timeoutVal, err := strconv.Atoi(timeoutStr)
		if err != nil {
			panic(p.ReturnError(msg, "[operator] invalid timeout: %s", timeoutStr))
		}
		timeout = time.Duration(timeoutVal)
	} else {
		timeout = 30
	}

	return retry, timeout, backoffms
}
