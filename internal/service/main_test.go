// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package service

import (
	"os"
	"testing"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}
