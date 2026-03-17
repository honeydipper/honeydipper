// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package dipper

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper/mock_dipper"
	"github.com/stretchr/testify/assert"
)

func TestRPCCallRaw(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	curr := 0
	c := RPCCallerBase{}
	var m *Message

	expect := []MessageReceiver{
		&NullReceiver{
			SendMessageFunc: func(msg *Message) {
				m = msg
			},
		},
	}

	mockStub := mock_dipper.NewMockRPCCallerStub(ctrl)
	mockStub.EXPECT().GetName().AnyTimes().Return("mockCaller")
	mockStub.EXPECT().GetReceiver(gomock.AssignableToTypeOf("")).AnyTimes().DoAndReturn(func(x string) MessageReceiver {
		ret := expect[curr]
		curr++

		return ret
	})
	c.Init(mockStub, "rpc", "call")
	assert.NotPanicsf(t, func() { c.CallRaw("target", "testmethod", []byte("hello world")) }, "CallRaw should not panic")
	time.Sleep(time.Second / 10)

	assert.Equal(t, "rpc", m.Channel, "rpc call sends message through rpc channel")
	assert.Equal(t, "call", m.Subject, "rpc uses callee and method and prefix for subject")
	assert.Equal(t, 4, len(m.Labels), "rpc call use labels to specify feature and method")

	lv, ok := m.Labels["caller"]
	assert.True(t, ok, "rpc caller present")
	assert.Equal(t, "-", lv, "rpc caller should be -")
	lv, ok = m.Labels["rpcID"]
	assert.True(t, ok, "rpc rpcID present")
	assert.Equal(t, "0", lv, "rpcID should be 0")
	lv, ok = m.Labels["feature"]
	assert.True(t, ok, "rpc feature present")
	assert.Equal(t, "target", lv, "feature should be target")
	lv, ok = m.Labels["method"]
	assert.True(t, ok, "rpc method present")

	assert.Equal(t, "testmethod", lv, "method should be testmethod")
	received, ok := m.Payload.([]byte)
	assert.True(t, ok, "rpc call payload should be byte array")
	assert.Equal(t, "hello world", string(received), "rpc should be unchanged")
}
