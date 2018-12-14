package dipper

import (
	"fmt"
	"io"
	"time"
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

// ReturnError : return error to rpc caller
func (p *CommandProvider) ReturnError(call *Message, reason string) {
	retMsg := &Message{
		Channel: p.Channel,
		Subject: p.Subject,
		Labels:  call.Labels,
	}
	retMsg.Labels["status"] = "failure"
	retMsg.Labels["reason"] = reason
	SendMessage(p.ReturnWriter, retMsg)
}

// Return : return a value to rpc caller
func (p *CommandProvider) Return(call *Message, retval *Message) {
	retMsg := &Message{
		Channel: p.Channel,
		Subject: p.Subject,
		Labels:  call.Labels,
	}
	retMsg.Labels["status"] = "success"
	retMsg.Payload = retval.Payload
	retMsg.IsRaw = retval.IsRaw
	SendMessage(p.ReturnWriter, retMsg)
}

// Router : route the message to rpc handlers
func (p *CommandProvider) Router(msg *Message) {
	method := msg.Labels["method"]
	f := p.Commands[method]
	msg.Reply = make(chan Message, 1)

	go func() {
		defer close(msg.Reply)
		select {
		case reply := <-msg.Reply:
			if _, ok := msg.Labels["sessionID"]; ok {
				if reason, ok := reply.Labels["error"]; ok {
					p.ReturnError(msg, reason)
				} else {
					p.Return(msg, &reply)
				}
			}
		case <-time.After(time.Second * 10):
			if _, ok := msg.Labels["sessionID"]; ok {
				p.ReturnError(msg, "timeout")
			}
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
	f(msg)
}
