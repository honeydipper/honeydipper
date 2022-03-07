// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package dipper

import (
	"io"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/pkg/dipper/mock_dipper"
	"github.com/stretchr/testify/assert"
)

func wrapDecryptAll(t *testing.T, doc string, expect func(io.Reader, *RPCCallerBase)) map[string]interface{} {
	var data map[string]interface{}
	assert.NoError(t, yaml.Unmarshal([]byte(doc), &data))

	O, I := io.Pipe()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStub := mock_dipper.NewMockRPCCallerStub(ctrl)
	c := &RPCCallerBase{}
	mockStub.EXPECT().GetName().AnyTimes().Return("mockCaller")
	mockStub.EXPECT().GetStream(gomock.Any()).AnyTimes().Return(I)
	c.Init(mockStub, "rpc", "call")

	done := make(chan struct{})
	go func() {
		assert.NotPanics(t, func() { expect(O, c) }, "should not panic when sending and receiving messages")
		assert.PanicsWithError(t, "invalid message envelope: io: read/write on closed pipe", func() { FetchRawMessage(O) }, "should contain no more messages")
		close(done)
	}()

	DecryptAll(c, data)

	O.Close()
	I.Close()
	<-done

	return data
}

func TestDecryptAll(t *testing.T) {
	doc := `
data:
  item1: not encrypted
  item2: ENC[noexist,YWFiYmNjZGQ=]
`

	expect := func(O io.Reader, c *RPCCallerBase) {
		assert.Equalf(t,
			Message{
				Channel: "rpc",
				Subject: "call",
				IsRaw:   true,
				Size:    8,
				Payload: []byte("aabbccdd"),
				Labels: map[string]string{
					"caller":  "-",
					"feature": "driver:noexist",
					"method":  "decrypt",
					"rpcID":   "0",
				},
			},
			*FetchRawMessage(O),
			"should make call to decrypt item2",
		)
		c.HandleReturn(&Message{
			Channel: "rpc",
			Subject: "return",
			IsRaw:   true,
			Size:    18,
			Payload: []byte("decrypted aabbccdd"),
			Labels: map[string]string{
				"caller":  "-",
				"feature": "driver:noexist",
				"method":  "decrypt",
				"rpcID":   "0",
				"status":  "success",
			},
		})
	}

	data := wrapDecryptAll(t, doc, expect)
	assert.Equal(t, "decrypted aabbccdd", MustGetMapDataStr(data, "data.item2"), "data.item2 should container decrypted data")
	assert.Equal(t, "not encrypted", MustGetMapDataStr(data, "data.item1"), "data.item1 should remain unchanged")
}

func TestDecryptAllWithDeferred(t *testing.T) {
	doc := `
data:
  item1: not encrypted
  item3:
    item4: ENC[deferred,driver1,YWFiYmNjZGQ=]
`

	expect := func(O io.Reader, c *RPCCallerBase) {
	}

	data := wrapDecryptAll(t, doc, expect)
	assert.Equal(t, "ENC[driver1,YWFiYmNjZGQ=]", MustGetMapDataStr(data, "data.item3.item4"), "item4 should be stripped off one deferred flag")
	assert.Equal(t, "not encrypted", MustGetMapDataStr(data, "data.item1"), "data.item1 should remain unchanged")
}

func TestDecryptAllWithLookUp(t *testing.T) {
	doc := `
data:
  item1: not encrypted
  item2: LOOKUP[kvstore,foo]
`

	expect := func(O io.Reader, c *RPCCallerBase) {
		assert.Equalf(t,
			Message{
				Channel: "rpc",
				Subject: "call",
				IsRaw:   true,
				Size:    3,
				Payload: []byte("foo"),
				Labels: map[string]string{
					"caller":  "-",
					"feature": "driver:kvstore",
					"method":  "lookup",
					"rpcID":   "0",
				},
			},
			*FetchRawMessage(O),
			"should make call to lookup for item2",
		)
		c.HandleReturn(&Message{
			Channel: "rpc",
			Subject: "return",
			IsRaw:   true,
			Size:    3,
			Payload: []byte("bar"),
			Labels: map[string]string{
				"caller":  "-",
				"feature": "driver:kvstore",
				"method":  "lookup",
				"rpcID":   "0",
				"status":  "success",
			},
		})
	}

	data := wrapDecryptAll(t, doc, expect)
	assert.Equal(t, "bar", MustGetMapDataStr(data, "data.item2"), "data.item2 should container decrypted data")
	assert.Equal(t, "not encrypted", MustGetMapDataStr(data, "data.item1"), "data.item1 should remain unchanged")
}
