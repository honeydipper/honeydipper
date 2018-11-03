package dipper

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"strings"
	"testing"
	"time"
)

func TestRPCCallRaw(t *testing.T) {
	var b bytes.Buffer
	c := &RPCCaller{Name: "driver:test"}
	go c.RPCCallRaw(&b, "target.testmethod", []byte("hello world"))
	time.Sleep(time.Second / 10)
	var channel, subject string
	var size int
	fmt.Fscanln(&b, &channel, &subject, &size)
	assert.Equal(t, "rpc", channel, "rpc call sends message through rpc channel")
	assert.Equal(t, "target.testmethod.", subject[:18], "rpc uses callee and method and prefix for subject")
	assert.Equal(t, 11, size, "rpc call raw sends the bytes as payload")
	received, err := ioutil.ReadAll(&b)
	assert.Nil(t, err, "rpc call payload should be readable")
	assert.Equal(t, "hello world", string(received), "rpc should be unchanged")

	b = bytes.Buffer{}
	c.Name = "service:testservice"
	go c.RPCCallRaw(&b, "target.testmethod", []byte("h2"))
	time.Sleep(time.Second / 10)
	fmt.Fscanln(&b, &channel, &subject, &size)
	parts := strings.Split(subject, ".")
	assert.Equal(t, 4, len(parts), "rpc invoked by service should have 4 parts in subject")
	assert.Equal(t, "service", parts[3], "rpc invoked by service should have suffix as 'service'")
}
