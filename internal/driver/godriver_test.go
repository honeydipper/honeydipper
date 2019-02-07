// +build !integration

package driver

import (
	"github.com/honeyscience/honeydipper/internal/config"
	"github.com/stretchr/testify/assert"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNewGoDriver(t *testing.T) {
	assert.Panics(t, func() { _ = NewGoDriver(map[string]interface{}{}) }, "NewGoDriver should panic when missing 'Package'")

	testGoDriver := NewGoDriver(map[string]interface{}{
		"Package": "test.com/test",
	})
	assert.Equal(t, "go", testGoDriver.Driver().Type, "NewGoDriver should return a driver with 'Type' = 'go'")
	assert.Equal(t, "test", testGoDriver.Driver().Executable, "NewGoDriver with .../test should return a driver with 'Executable' = 'test'")

	testGoDriver = NewGoDriver(map[string]interface{}{
		"Package":    "test.com/test",
		"Executable": "test2",
	})
	assert.Equal(t, "test2", testGoDriver.Driver().Executable, "NewGoDriver should be able to override executable with 'Executable' = 'test2'")
}

func TestGoDriverPreStart(t *testing.T) {
	mockDriverRuntime := Runtime{
		Meta: &config.DriverMeta{
			Name: "mock",
		},
	}
	mockDriver := GoDriver{
		driver: &config.Driver{
			Type:       "go",
			Executable: "testbinary",
		},
		Package: "test.com/test",
	}

	func() {
		defer func() { execCommand = exec.Command }()
		execCommand = generateFakeExecCommand("TestGoDriverPreStartProcess")
		mockDriver.PreStart("testservice", &mockDriverRuntime)
	}()
	assert.Equal(t, 2, fakeExecCommandCount, "godriver preStart should call go list and go get when package is not installed")

	mockDriver2 := GoDriver{
		driver: &config.Driver{
			Type:       "go",
			Executable: "testbinary",
		},
		Package: "test.com/test1",
	}

	func() {
		defer func() { execCommand = exec.Command }()
		execCommand = generateFakeExecCommand("TestGoDriverPreStartProcess")
		mockDriver2.PreStart("testservice", &mockDriverRuntime)
	}()
	assert.Equal(t, 1, fakeExecCommandCount, "godriver preStart should skip 'go get' when package is installed")

	mockDriver3 := GoDriver{
		driver: &config.Driver{
			Type:       "go",
			Executable: "testbinary",
		},
		Package: "test.com/error",
	}
	assert.Panics(t, func() {
		defer func() { execCommand = exec.Command }()
		execCommand = generateFakeExecCommand("TestGoDriverPreStartProcess")
		mockDriver3.PreStart("testservice", &mockDriverRuntime)
	}, "godriver preStart should panic when not able to install go driver")
}

func TestGoDriverPreStartProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if strings.Join(os.Args[3:6], " ") == "go list test.com/test" {
		os.Exit(1)
	}
	if strings.Join(os.Args[3:6], " ") == "go list test.com/error" {
		os.Exit(1)
	}
	if strings.Join(os.Args[3:6], " ") == "go get test.com/error" {
		os.Exit(1)
	}
	// some code here to check arguments perhaps?
	os.Exit(0)
}
