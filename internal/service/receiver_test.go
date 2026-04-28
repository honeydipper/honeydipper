// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestReceiverFeatures(t *testing.T) {
}

func TestReceiverRoute(t *testing.T) {
	receiver = &Service{
		driverRuntimes: map[string]*driver.Runtime{
			"eventbus": {
				Feature: "eventbus",
				Handler: &driver.NullDriverHandler{},
			},
		},
	}

	msg := &dipper.Message{
		Channel: "eventbus",
		Subject: "message",
		Payload: map[string]interface{}{"foo": "bar"},
		Labels:  map[string]string{"x": "1"},
	}

	routes := receiverRoute(msg)
	assert.Len(t, routes, 1)
	assert.Equal(t, "message", routes[0].message.Subject)
	assert.Equal(t, "1", routes[0].message.Labels["x"])
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, routes[0].message.Payload)
}
