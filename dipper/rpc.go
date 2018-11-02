package dipper

import (
	"errors"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

// RPCCaller : an object that makes RPC calls
type RPCCaller struct {
	Sender  interface{}
	RPCSig  map[string]chan interface{}
	RPCLock sync.Mutex
}

// FeatureProvider : an object that can get the comm channel for a feature, usually a service
type FeatureProvider interface {
	// GetFeatureComm : get the output writer channel of a feature for communicating
	GetFeatureComm(feature string) io.Writer
}

// RPCCallRaw : making a RPC call to another driver with raw data
func (c *RPCCaller) RPCCallRaw(method string, params []byte) ([]byte, error) {
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
	if driver, ok := c.Sender.(*Driver); ok {
		driver.SendRawMessage("rpc", method+"."+rpcID, params)
	} else if provider, ok := c.Sender.(FeatureProvider); ok {
		parts := strings.SplitN(method, ".", 2)
		output := provider.GetFeatureComm(parts[0])
		SendRawMessage(output, "rpc", parts[1]+"."+rpcID+".service", params)
	} else {
		log.Panicf("unable to convert to a driver or feature provider")
	}

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
func (c *RPCCaller) RPCCall(method string, params interface{}) ([]byte, error) {
	return c.RPCCallRaw(method, SerializeContent(params))
}

// HandleRPCReturn : receiving return of a RPC call
func (c *RPCCaller) HandleRPCReturn(m *Message) {
	log.Printf("handling rpc return")
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
		log.Printf("rpcID not found or expired %s", rpcID)
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
