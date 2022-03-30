// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package api

import (
	"os"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		// f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0777)
		f, _ := os.Create("test.log")
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}

type TestStep struct {
	Feature         string
	Method          string
	ExpectedMessage interface{}
	ReturnMessage   interface{}
	Err             error
}

type ReturnMessage struct {
	Delay time.Duration
	Msg   *dipper.Message
}
