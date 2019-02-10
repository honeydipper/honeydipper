// +build !integration

package driver

import (
	"os"
	"os/exec"
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
