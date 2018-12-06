// +build !integration

package dipper

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"testing"
	"time"
)

func TestRPCCallRaw(t *testing.T) {
	var b bytes.Buffer
	c := RPCCaller{}
	c.Init("rpc", "call")
	go c.CallRaw(&b, "target", "testmethod", []byte("hello world"))
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
	fmt.Fscanln(&b, &lname, &vl)
	assert.Equal(t, "caller", lname, "rpc caller present")
	assert.Equal(t, 1, vl, "caller name should be 1 character")
	lval = make([]byte, vl)
	io.ReadFull(&b, lval)
	assert.Equal(t, "-", string(lval), "rpc caller should be -")
	fmt.Fscanln(&b, &lname, &vl)
	assert.Equal(t, "rpcID", lname, "rpc rpcID present")
	assert.Equal(t, 1, vl, "rpcID should be 1 character")
	lval = make([]byte, vl)
	io.ReadFull(&b, lval)
	assert.Equal(t, "0", string(lval), "rpcID should be 0")
	fmt.Fscanln(&b, &lname, &vl)
	assert.Equal(t, "feature", lname, "rpc feature present")
	assert.Equal(t, 6, vl, "rpc feature should be 6 characters")
	lval = make([]byte, vl)
	io.ReadFull(&b, lval)
	assert.Equal(t, "target", string(lval), "rpc feature correct")
	fmt.Fscanln(&b, &lname, &vl)
	assert.Equal(t, "method", lname, "rpc method present")
	assert.Equal(t, 10, vl, "rpc method should be 10 characters")
	lval = make([]byte, vl)
	io.ReadFull(&b, lval)
	assert.Equal(t, "testmethod", string(lval), "rpc method correct")
	received, err := ioutil.ReadAll(&b)
	assert.Nil(t, err, "rpc call payload should be readable")
	assert.Equal(t, "hello world", string(received), "rpc should be unchanged")
}
