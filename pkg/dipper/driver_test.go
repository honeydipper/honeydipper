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

	"github.com/stretchr/testify/assert"
)

func TestDriverStartWaitsForPendingOptions(t *testing.T) {
	d := &Driver{}

	// no options pending, should not block
	done := make(chan struct{})
	go func() {
		d.waitOptions()
		close(done)
	}()
	assert.Eventually(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond*5, "waitOptions should not block when no options are pending")

	// options pending, should block until finished
	d.beginOptions()
	waited := make(chan struct{})
	go func() {
		d.waitOptions()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("waitOptions should block while options are pending")
	case <-time.After(time.Millisecond * 50):
	}

	d.finishOptions()
	assert.Eventually(t, func() bool {
		select {
		case <-waited:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond*5, "waitOptions should return once options are applied")

	// a later options message re-arms the gate
	d.beginOptions()
	rearmed := make(chan struct{})
	go func() {
		d.waitOptions()
		close(rearmed)
	}()
	select {
	case <-rearmed:
		t.Fatal("waitOptions should block again for a new options message")
	case <-time.After(time.Millisecond * 50):
	}
	d.finishOptions()
	assert.Eventually(t, func() bool {
		select {
		case <-rearmed:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond*5)
}
