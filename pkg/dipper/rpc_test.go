// +build !integration

package dipper

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRPCCallRaw(t *testing.T) {
	var b bytes.Buffer
	c := RPCCaller{}
	c.Init("rpc", "call")
	go PanicError(c.CallRaw(&b, "target", "testmethod", []byte("hello world")))
	time.Sleep(time.Second / 10)
	var channel, subject string
	var size, numlabels int
	fmt.Fscanln(&b, &channel, &subject, &numlabels, &size)
	assert.Equal(t, "rpc", channel, "rpc call sends message through rpc channel")
	assert.Equal(t, "call", subject, "rpc uses callee and method and prefix for subject")
	assert.Equal(t, 11, size, "rpc call raw sends the bytes as payload")
	assert.Equal(t, 4, numlabels, "rpc call use labels to specify feature and method")
	var lname string
	var lval []byte
	var vl int
	labels := map[string]string{}
	fmt.Fscanln(&b, &lname, &vl)
	if vl > 0 {
		lval = make([]byte, vl)
		PanicError(io.ReadFull(&b, lval))
		labels[lname] = string(lval)
	} else {
		labels[lname] = ""
	}
	fmt.Fscanln(&b, &lname, &vl)
	if vl > 0 {
		lval = make([]byte, vl)
		PanicError(io.ReadFull(&b, lval))
		labels[lname] = string(lval)
	} else {
		labels[lname] = ""
	}
	fmt.Fscanln(&b, &lname, &vl)
	if vl > 0 {
		lval = make([]byte, vl)
		PanicError(io.ReadFull(&b, lval))
		labels[lname] = string(lval)
	} else {
		labels[lname] = ""
	}
	fmt.Fscanln(&b, &lname, &vl)
	if vl > 0 {
		lval = make([]byte, vl)
		PanicError(io.ReadFull(&b, lval))
		labels[lname] = string(lval)
	} else {
		labels[lname] = ""
	}
	lv, ok := labels["caller"]
	assert.True(t, ok, "rpc caller present")
	assert.Equal(t, "-", lv, "rpc caller should be -")
	lv, ok = labels["rpcID"]
	assert.True(t, ok, "rpc rpcID present")
	assert.Equal(t, "0", lv, "rpcID should be 0")
	lv, ok = labels["feature"]
	assert.True(t, ok, "rpc feature present")
	assert.Equal(t, "target", lv, "feature should be target")
	lv, ok = labels["method"]
	assert.True(t, ok, "rpc method present")
	assert.Equal(t, "testmethod", lv, "method should be testmethod")
	received, err := ioutil.ReadAll(&b)
	assert.Nil(t, err, "rpc call payload should be readable")
	assert.Equal(t, "hello world", string(received), "rpc should be unchanged")
}
