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

func TestGetenv(t *testing.T) {
	assert.NotNil(t, Getenv(), "Getenv should not return nil")
	assert.NotContains(t, Getenv(), "TEST_ENV", "TEST_ENV should not present before injection")

	// reset
	_envs = nil
	t.Setenv("HD_TEST_ENV", "present")
	assert.NotNil(t, Getenv(), "Getenv should not return nil the 2nd time")
	assert.Contains(t, Getenv(), "TEST_ENV", "TEST_ENV should present after injection")
}
