package dipper

import (
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

// RPCCaller : an object that makes RPC calls
type RPCCaller struct {
	Name    string
	RPCSig  map[string]chan interface{}
	RPCLock sync.Mutex
}

// RPCCallRaw : making a RPC call to another driver with raw data
func (c *RPCCaller) RPCCallRaw(out io.Writer, method string, params []byte) ([]byte, error) {
	var rpcID string
	sig := make(chan interface{}, 1)
	func() {
		c.RPCLock.Lock()
		defer c.RPCLock.Unlock()
		for ok := true; ok; _, ok = c.RPCSig[rpcID] {
			rpcID = RandString(6)
		}
		if c.RPCSig == nil {
			c.RPCSig = map[string]chan interface{}{}
		}
		c.RPCSig[rpcID] = sig
	}()
	defer func() {
		c.RPCLock.Lock()
		defer c.RPCLock.Unlock()
		if _, ok := c.RPCSig[rpcID]; ok {
			delete(c.RPCSig, rpcID)
		}
	}()

	subject := method + "." + rpcID
	if strings.HasPrefix(c.Name, "service:") {
		subject = subject + ".service"
	}
	SendRawMessage(out, "rpc", subject, params)

	select {
	case msg := <-sig:
		if e, ok := msg.(error); ok {
			return nil, e
		}
		return msg.([]byte), nil
	case <-time.After(time.Second * 10):
		return nil, errors.New("timeout")
	}
}

// RPCCall : making a RPC call to another driver
func (c *RPCCaller) RPCCall(out io.Writer, method string, params interface{}) ([]byte, error) {
	return c.RPCCallRaw(out, method, SerializeContent(params))
}

// HandleRPCReturn : receiving return of a RPC call
func (c *RPCCaller) HandleRPCReturn(m *Message) {
	log.Debugf("[%s] handling rpc return", c.Name)
	parts := strings.Split(m.Subject, ".")
	if parts[0] == "service" {
		parts = parts[1:]
	}
	rpcID := parts[0]
	hasErr := len(parts) > 1
	var sig chan interface{}
	var ok bool
	func() {
		c.RPCLock.Lock()
		defer c.RPCLock.Unlock()
		sig, ok = c.RPCSig[rpcID]
	}()
	if !ok {
		log.Warningf("[%s] rpcID not found or expired %s", c.Name, rpcID)
	} else {
		if hasErr {
			m = DeserializePayload(m)
			reason, _ := GetMapDataStr(m.Payload, "reason")
			sig <- errors.New(reason)
		} else {
			sig <- m.Payload
		}
		func() {
			c.RPCLock.Lock()
			defer c.RPCLock.Unlock()
			if _, ok := c.RPCSig[rpcID]; ok {
				delete(c.RPCSig, rpcID)
			}
		}()
	}
}
