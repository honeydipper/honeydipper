// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"sync/atomic"
	"testing"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

type fakeCacheCaller struct {
	data map[string][]byte
}

func (f *fakeCacheCaller) Call(feature string, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	if f.data == nil {
		f.data = map[string][]byte{}
	}
	if feature != "cache" {
		return []byte{}, nil
	}
	payload, _ := params.(map[string]any)
	key, _ := payload["key"].(string)

	switch method {
	case "load":
		return f.data[key], nil
	case "save":
		val, _ := payload["value"].(string)
		f.data[key] = []byte(val)

		return []byte{}, nil
	case "del":
		delete(f.data, key)

		return []byte{}, nil
	default:
		return []byte{}, nil
	}
}

func (f *fakeCacheCaller) CallNoWait(feature string, method string, params interface{}, labelsKV ...string) error {
	_, _ = f.Call(feature, method, params, labelsKV...)

	return nil
}

func (f *fakeCacheCaller) CallRaw(feature string, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return []byte{}, nil
}

func (f *fakeCacheCaller) CallRawNoWait(feature string, method string, params []byte, rpcID string, labelsKV ...string) error {
	return nil
}

func (f *fakeCacheCaller) GetName() string {
	return "fake-cache"
}

func TestCreateActivations(t *testing.T) {
	atomic.StoreInt64(&agentMatchCounter, 0)

	agent = &Service{ready: make(chan struct{})}
	close(agent.ready)
	orig := persistActivationFn
	persistActivationFn = func(caller dipper.RPCCaller, msg *dipper.Message, agentName string) (*activationPersistResult, error) {
		return &activationPersistResult{SessionID: "s1", TurnID: "t1"}, nil
	}
	defer func() { persistActivationFn = orig }()

	msg := &dipper.Message{Payload: map[string]interface{}{
		"agent":  "support-bot",
		"prompt": "help user",
	}}

	assert.NotPanics(t, func() { createActivations(nil, msg) })
	assert.Equal(t, int64(1), atomic.LoadInt64(&agentMatchCounter))
}

func TestPersistAgentActivation_ReusesSessionOnSameEvent(t *testing.T) {
	caller := &fakeCacheCaller{data: map[string][]byte{}}
	msg := &dipper.Message{
		Labels: map[string]string{"eventID": "evt-1", "sourceSessionID": "wf-1"},
		Payload: map[string]any{
			"agent": "support-bot",
		},
	}

	first, err := persistAgentActivation(caller, msg, "support-bot")
	assert.NoError(t, err)
	assert.NotEmpty(t, first.SessionID)
	assert.NotEmpty(t, first.TurnID)

	second, err := persistAgentActivation(caller, msg, "support-bot")
	assert.NoError(t, err)
	assert.Equal(t, first.SessionID, second.SessionID)
	assert.NotEqual(t, first.TurnID, second.TurnID)
}

func TestPersistAgentActivation_UsesCtxConversationID(t *testing.T) {
	caller := &fakeCacheCaller{data: map[string][]byte{}}
	msg1 := &dipper.Message{
		Labels: map[string]string{"eventID": "evt-1"},
		Payload: map[string]any{
			"agent": "support-bot",
			"ctx": map[string]any{
				"conversation_id": "thread-123",
			},
		},
	}
	msg2 := &dipper.Message{
		Labels: map[string]string{"eventID": "evt-2"},
		Payload: map[string]any{
			"agent": "support-bot",
			"ctx": map[string]any{
				"conversation_id": "thread-123",
			},
		},
	}

	first, err := persistAgentActivation(caller, msg1, "support-bot")
	assert.NoError(t, err)
	second, err := persistAgentActivation(caller, msg2, "support-bot")
	assert.NoError(t, err)

	assert.Equal(t, first.SessionID, second.SessionID)
	assert.NotEqual(t, first.TurnID, second.TurnID)
}
