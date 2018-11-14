// +build integration

package main

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

var bootstrapPath string

func TestIntegrationStart(t *testing.T) {
	if !t.Run("initialize a repo", intTestInitRepo) {
		t.FailNow()
	}
	if !t.Run("starting up daemon", intTestDaemonStartup) {
		t.FailNow()
	}
	defer t.Run("shutting down daemon", intTestDaemonShutdown)
	t.Run("checking services", intTestServices)
	t.Run("checking drivers", intTestDrivers)
	t.Run("checking processes", intTestProcesses)
}

func intTestInitRepo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmdOutput, err := exec.CommandContext(ctx, "test_fixtures/bootstrap/setup.sh").CombinedOutput()
	assert.Nil(t, err, "Needs to init a test repo to bootstrap test daemon")
	bootstrapPath = strings.TrimSpace(string(cmdOutput))
}

func intTestDaemonStartup(t *testing.T) {
	config = Config{
		initRepo: RepoInfo{
			Repo:   "file://" + bootstrapPath,
			Branch: "master",
			Path:   "/",
		},
	}
	go func() {
		config.bootstrap("/tmp")
		start()
	}()

	time.Sleep(time.Second * 5)
	assert.True(t, runtime.NumGoroutine() > 10, "running goroutine should be more than 10")
}

func intTestServices(t *testing.T) {
	_, ok := Services["receiver"]
	assert.True(t, ok, "receiver service should be running")
	_, ok = Services["engine"]
	assert.True(t, ok, "engine service should be running")
	_, ok = Services["operator"]
	assert.True(t, ok, "operator service should be running")
	assert.True(t, len(Services) == 3, "there should be 3 services running")
}

func intTestDrivers(t *testing.T) {
	r := Services["receiver"]
	assert.Equal(t, 2, len(r.driverRuntimes), "receiver should have 2 drivers")
	e := Services["engine"]
	assert.Equal(t, 1, len(e.driverRuntimes), "engine should have 1 drivers")
	o := Services["operator"]
	assert.Equal(t, 4, len(o.driverRuntimes), "operator should have 4 drivers")
}

func intTestProcesses(t *testing.T) {
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		pidstr, err := exec.CommandContext(ctx, "pgrep", "honeydipper").Output()
		assert.Nil(t, err, "should be able to run pgrep to find all honeydipper processes")
		pids := strings.Split(string(pidstr), "\n")
		assert.Lenf(t, pids, 9, "expecting 9 processes with honeydipper name")
	}()
}

func intTestDaemonShutdown(t *testing.T) {
	var graceful = make(chan bool)
	go func() {
		shutDown()
		graceful <- true
	}()
	select {
	case <-graceful:
	case <-time.After(time.Second * 5):
		t.Errorf("service not shutdown after 5 seconds")
	}
}
