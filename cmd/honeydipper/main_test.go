// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"os"
	"testing"

	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		logFile, err := os.Create("test.log")
		if err != nil {
			panic(err)
		}
		defer logFile.Close()
		dipper.GetLogger("test", "INFO", logFile, logFile)
	}
	os.Exit(m.Run())
}
