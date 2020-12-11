// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package dipper is a library used for developing drivers for Honeydipper.
package dipper

import "sync"

// WaitGroupDone is a way to safely decrement the counter for a WaitGroup.
func WaitGroupDone(wg *sync.WaitGroup) {
	defer func() {
		_ = recover()
	}()

	wg.Done()
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
