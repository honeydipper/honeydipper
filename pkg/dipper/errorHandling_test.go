// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package dipper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeExitOnError(t *testing.T) {
	testFunc := func() {
		defer SafeExitOnError("test found error")
		panic("test error")
	}
	assert.NotPanics(t, testFunc, "testFunc should not panic with SafeExitOnError")

	var caught any
	testFuncWithHandler := func() {
		defer SafeExitOnError(func(r any) {
			caught = r
		})
		panic("test error")
	}
	assert.NotPanics(t, testFuncWithHandler, "testFuncWithHandler should not panic with SafeExitOnError")
	assert.Equal(t, "test error", caught, "testFuncWithHandler should run the handler")
}
