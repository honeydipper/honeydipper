// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

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
