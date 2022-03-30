// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if Logger == nil {
		logFile, err := os.Create("test.log")
		if err != nil {
			panic(err)
		}
		GetLogger("test", "INFO", logFile, logFile)
	}
	os.Exit(m.Run())
}
