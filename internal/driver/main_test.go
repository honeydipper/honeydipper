// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package driver

import (
	"os"
	"testing"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test driver", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}
