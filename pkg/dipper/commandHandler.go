// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/go-errors/errors"
)

const (
	// SUCCESS means the workflow finished successfully.
	SUCCESS = "success"
	// FAILURE means the workflow finished with failure status.
	FAILURE = "failure"
	// ERROR means the workflow run into errors and could not complete.
	ERROR = "error"
)

// CommandProvider : an interface for providing Command handling feature.
type CommandProvider struct {
	Live               sync.WaitGroup
	Commands           map[string]MessageHandler
	DefaultTimeout     map[string]string
	ReturnWriter       io.Writer
	Channel            string
	Subject            string
	InterruptedSubject string
	Handler            context.Context
}

// Init : initializing rpc provider.
func (p *CommandProvider) Init(channel string, subject string, interruptedSubject string, defaultWriter io.Writer, handler context.Context) {
	p.Commands = map[string]MessageHandler{}
	p.DefaultTimeout = map[string]string{}
	p.ReturnWriter = defaultWriter
	p.Channel = channel
	p.Subject = subject
	p.InterruptedSubject = interruptedSubject
	p.Handler = handler
}

// ReturnInterrupted returns an message to caller so the interrupted call can be resumed later.
func (p *CommandProvider) ReturnInterrupted(call *Message) {
	defer func() {
		if call.Reply != nil {
			call.Reply = nil
			p.Live.Done()
		}
	}()

	msg := *call
	msg.Channel = p.Channel
	msg.Subject = p.InterruptedSubject

	Logger.Warningf("[operator] interrupted, sessionID: %+v", msg)

	SendMessage(p.ReturnWriter, &msg)
}

// ReturnError sends an error message return to caller and create an error.
func (p *CommandProvider) ReturnError(call *Message, pattern string, args ...interface{}) error {
	errText := fmt.Sprintf(pattern, args...)
	p.Return(call, &Message{
		Labels: map[string]string{
			"error": errText,
		},
	})
	Logger.Warningf("[operator] %s", errText)

	return errors.New(errText)
}

// Return : return a value to rpc caller.
func (p *CommandProvider) Return(call *Message, retval *Message) {
	defer func() {
		if call.Reply != nil {
			call.Reply = nil
			p.Live.Done()
		}
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

type commandWrapper struct {
	msg           *Message
	method        string
	provider      *CommandProvider
	f             MessageHandler
	retry         int
	timeout       time.Duration
	backoff       time.Duration
	interruptible bool
}

func (w *commandWrapper) attempt(replyChannel chan Message) {
	w.msg.Reply = replyChannel
	m := *w.msg
	// make a copy of Labels so the function call won't
	// tamper with the original one.
	m.Labels = map[string]string{}
	for k, v := range w.msg.Labels {
		m.Labels[k] = v
	}

	go func() {
		defer func() {
			close(replyChannel)
			replyChannel = nil
		}()

		Logger.Debugf("[operaotr] cmd labels %+v", m.Labels)

		apiTimer := time.NewTimer(w.timeout)
		defer apiTimer.Stop()

		interruption := context.Background().Done()
		if w.interruptible {
			interruption = w.provider.Handler.Done()
		}

		select {
		case reply := <-replyChannel:
			if _, ok := reply.Labels["no-timeout"]; ok {
				select {
				case reply = <-replyChannel:
				case <-w.provider.Handler.Done():
					w.provider.ReturnInterrupted(w.msg)

					return
				}
			}

			_, hasError := reply.Labels["error"]
			if status, ok := reply.Labels["status"]; (hasError || (ok && status != SUCCESS)) && w.retry > 0 {
				Logger.Debugf("[operaotr] %d retry left for method %s", w.retry, w.method)
				w.retry--
				time.Sleep(w.backoff * time.Millisecond)
				w.backoff *= 2
				go w.attempt(make(chan Message, 1))
			} else {
				w.provider.Return(w.msg, &reply)
			}
		case <-interruption:
			w.provider.ReturnInterrupted(w.msg)
		case <-apiTimer.C:
			_ = w.provider.ReturnError(w.msg, "timeout")
		}
	}()

	defer func() {
		if r := recover(); r != nil && replyChannel != nil {
			if e, ok := r.(error); ok {
				if errors.Is(e, context.Canceled) && w.interruptible && errors.Is(w.provider.Handler.Err(), context.Canceled) {
					Logger.Warningf("Command interrupted: %+v", *w.msg)

					return
				}
			}
			Logger.Warningf("Resuming after command error: %v", r)
			Logger.Warning(errors.Wrap(r, 1).ErrorStack())
			replyChannel <- Message{
				Labels: map[string]string{
					"error": fmt.Sprintf("%+v", r),
				},
			}
		}
	}()
	w.f(&m)
}

// Router : route the message to rpc handlers.
func (p *CommandProvider) Router(msg *Message) {
	method := msg.Labels["method"]
	interruptible := false
	f, ok := p.Commands[method]
	if !ok {
		f, ok = p.Commands[method+"|interruptible"]
		if ok {
			interruptible = true
			//nolint:goconst
			msg.Labels["interruptible"] = "true"
		}
	}
	if !ok {
		panic(p.ReturnError(msg, "[operator] cmd not defined: %s", method))
	}

	retry, timeout, backoff := p.UnpackLabels(msg)
	w := &commandWrapper{
		msg:           msg,
		f:             f,
		provider:      p,
		retry:         retry,
		timeout:       timeout,
		backoff:       backoff,
		interruptible: interruptible,
	}

	p.Live.Add(1)
	w.attempt(make(chan Message, 1))
}

// UnpackLabels loads necessary variables out of the labels.
func (p *CommandProvider) UnpackLabels(msg *Message) (retry int, timeout, backoffms time.Duration) {
	var err error

	retryStr := msg.Labels["retry"]
	if retryStr != "" {
		retry, err = strconv.Atoi(retryStr)
		if err != nil {
			panic(p.ReturnError(msg, "[operator] invalid retry: %s", retryStr))
		}
	}

	backoffStr := msg.Labels["backoff_ms"]
	if backoffStr != "" {
		backoffVal, err := strconv.Atoi(backoffStr)
		if err != nil {
			panic(p.ReturnError(msg, "[operator] invalid backoff_ms: %s", backoffStr))
		}
		backoffms = time.Duration(backoffVal)
	} else {
		backoffms = 1000
	}

	timeoutStr := msg.Labels["timeout"]
	if timeoutStr == "" {
		if timeoutStr = p.DefaultTimeout[msg.Labels["method"]]; timeoutStr != "" {
			msg.Labels["timeout"] = timeoutStr
		}
	}

	if timeoutStr != "" {
		timeout = Must(time.ParseDuration(timeoutStr)).(time.Duration)
	} else {
		timeout = 30 * time.Second
		msg.Labels["timeout"] = "30s"
	}

	return retry, timeout, backoffms
}
