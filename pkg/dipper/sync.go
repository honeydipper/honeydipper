// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package dipper is a library used for developing drivers for Honeydipper.
package dipper

import (
	"sync"
	"time"
)

// WaitGroupDone is a way to safely decrement the counter for a WaitGroup.
func WaitGroupDone(wg *sync.WaitGroup) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	wg.Done()
	ok = true

	return ok
}

// WaitGroupDoneAll is a way to safely release the whole WaitGroup.
func WaitGroupDoneAll(wg *sync.WaitGroup) {
	defer func() {
		_ = recover()
	}()

	for {
		wg.Done()
	}
}

// WaitGroupWait is a wrapper for safely waiting for a WaitGroup.
func WaitGroupWait(wg *sync.WaitGroup) {
	defer func() {
		_ = recover()
	}()

	wg.Wait()
}

// WaitGroupWaitTimeout is a wrapper for safely waiting for a WaitGroup with a timeout.
func WaitGroupWaitTimeout(wg *sync.WaitGroup, t time.Duration) {
	allDone := make(chan interface{})
	go func() {
		WaitGroupWait(wg)
		close(allDone)
	}()

	timer := time.NewTimer(t)
	defer timer.Stop()

	select {
	case <-timer.C:
		WaitGroupDoneAll(wg)
	case <-allDone:
	}
}
