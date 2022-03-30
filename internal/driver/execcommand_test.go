// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package driver

import (
	"os"
	"os/exec"
	"testing"
)

var fakeExecCommandCount = 0

func generateFakeExecCommand(fname string) func(string, ...string) *exec.Cmd {
	fakeExecCommandCount = 0
	fakeExecCommand := func(command string, args ...string) *exec.Cmd {
		cs := []string{"--test.run=" + fname, "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		fakeExecCommandCount++
		return cmd
	}
	return fakeExecCommand
}

func TestExecCommandDummy(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// some code here to check arguments perhaps?
	os.Exit(0)
}
