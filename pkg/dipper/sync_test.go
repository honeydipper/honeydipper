// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package dipper

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWaitGroupWaitTimeout(t *testing.T) {
	subj := &sync.WaitGroup{}
	subj.Add(3)
	done := false
	go func() {
		WaitGroupWaitTimeout(subj, time.Millisecond*10)
		done = true
	}()
	assert.Eventually(t, func() bool { return done }, time.Second, time.Millisecond*5, "WaitGroupWaitTimeout should eventually quit.")
}

func TestWaitGroupWait(t *testing.T) {
	subj := &sync.WaitGroup{}
	subj.Add(3)
	done := false
	go func() {
		WaitGroupDone(subj)
		WaitGroupWait(subj)
		done = true
	}()
	assert.Never(t, func() bool { return done }, time.Second, time.Millisecond*5, "WaitGroupWait should never quit until the wait group is done.")
	WaitGroupDoneAll(subj)
}
