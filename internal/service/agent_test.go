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

func TestCreateActivations(t *testing.T) {
	atomic.StoreInt64(&agentMatchCounter, 0)

	agent = &Service{ready: make(chan struct{})}
	close(agent.ready)

	msg := &dipper.Message{Payload: map[string]interface{}{
		"agent":  "support-bot",
		"prompt": "help user",
	}}

	assert.NotPanics(t, func() { createActivations(nil, msg) })
	assert.Equal(t, int64(1), atomic.LoadInt64(&agentMatchCounter))
}
