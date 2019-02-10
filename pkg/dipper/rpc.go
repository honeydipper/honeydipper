package dipper

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// RPCCaller : an object that makes RPC calls
type RPCCaller struct {
	Channel string
	Subject string
	Result  map[string]chan interface{}
	Lock    sync.Mutex
	Counter int
}

// Init : initializing rpc caller
func (c *RPCCaller) Init(channel string, subject string) {
	c.Result = map[string]chan interface{}{}
	InitIDMap(&c.Result)
	c.Counter = 0
	c.Channel = channel
	c.Subject = subject
}

// Call : making a RPC call to another driver with structured data
func (c *RPCCaller) Call(out io.Writer, feature string, method string, params interface{}) ([]byte, error) {
	ret, err := c.CallRaw(out, feature, method, SerializeContent(params))
	return ret, err
}

// CallNoWait : making a RPC call to another driver with structured data not expecting any return
func (c *RPCCaller) CallNoWait(out io.Writer, feature string, method string, params interface{}) {
	c.CallRawNoWait(out, feature, method, SerializeContent(params), "skip")
}

// CallRaw : making a RPC call to another driver with raw data
func (c *RPCCaller) CallRaw(out io.Writer, feature string, method string, params []byte) ([]byte, error) {

	// keep track the call in the map
	var result = make(chan interface{}, 1)
	var rpcID = IDMapPut(&c.Result, result)

	// clean up the call from the map when done
	defer IDMapDel(&c.Result, rpcID)

	c.CallRawNoWait(out, feature, method, params, rpcID)

	// waiting for the result to come back
	select {
	case msg := <-result:
		if e, ok := msg.(error); ok {
			return nil, e
		}
		return msg.([]byte), nil
	case <-time.After(time.Second * 10): // TODO: make timeout configurable
		return nil, errors.New("timeout")
	}
}

// CallRawNoWait : making a RPC call to another driver with raw data not expecting return
func (c *RPCCaller) CallRawNoWait(out io.Writer, feature string, method string, params []byte, rpcID string) {
	if rpcID == "" {
		rpcID = "skip"
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
}

// HandleReturn : receiving return of a RPC call
func (c *RPCCaller) HandleReturn(m *Message) {
	rpcID := m.Labels["rpcID"]
	result := c.Result[rpcID]

	reason, ok := m.Labels["error"]

	if ok {
		result <- errors.New(reason)
	} else {
		result <- m.Payload
	}
}

// RPCProvider : an interface for providing RPC handling feature
type RPCProvider struct {
	RPCHandlers   map[string]MessageHandler
	DefaultReturn io.Writer
	Channel       string
	Subject       string
}

// Init : initializing rpc provider
func (p *RPCProvider) Init(channel string, subject string, defaultWriter io.Writer) {
	p.RPCHandlers = map[string]MessageHandler{}
	p.DefaultReturn = defaultWriter
	p.Channel = channel
	p.Subject = subject
}

// ReturnError : return error to rpc caller
func (p *RPCProvider) ReturnError(call *Message, reason string) {
	var returnTo = call.ReturnTo
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

// Return : return a value to rpc caller
func (p *RPCProvider) Return(call *Message, retval *Message) {
	var returnTo = call.ReturnTo
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

// Router : route the message to rpc handlers
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
			case <-time.After(time.Second * 10):
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
