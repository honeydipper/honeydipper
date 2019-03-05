// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package main

import (
	"context"
	"fmt"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/internal/service"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

var bootstrapPath string

func TestIntegrationStart(t *testing.T) {
	if !t.Run("starting up daemon", intTestDaemonStartup) {
		t.FailNow()
	}
	defer t.Run("shutting down daemon", intTestDaemonShutdown)
	t.Run("checking services", intTestServices)
	t.Run("checking processes", intTestProcesses)
}

func intTestDaemonStartup(t *testing.T) {
	if dipper.Logger == nil {
		logFile, err := os.Create("test.log")
		if err != nil {
			panic(err)
		}
		dipper.GetLogger("test", "INFO", logFile, logFile)
	}
	workingBranch, ok := os.LookupEnv("CIRCLE_BRANCH")
	if !ok {
		workingBranch = "master"
	}
	cfg := config.Config{
		InitRepo: config.RepoInfo{
			Repo:   "..",
			Branch: workingBranch,
			Path:   "/test/test_fixtures/bootstrap",
		},
	}
	go func() {
		daemon.OnStart = func() {
			service.StartEngine(&cfg)
			service.StartReceiver(&cfg)
			service.StartOperator(&cfg)
		}
		daemon.Run(&cfg)
	}()

	time.Sleep(time.Second * 5)
	assert.True(t, runtime.NumGoroutine() > 10, "running goroutine should be more than 10")
}

func intTestServices(t *testing.T) {
	_, ok := service.Services["receiver"]
	assert.True(t, ok, "receiver service should be running")
	_, ok = service.Services["engine"]
	assert.True(t, ok, "engine service should be running")
	_, ok = service.Services["operator"]
	assert.True(t, ok, "operator service should be running")
	assert.True(t, len(service.Services) == 3, "there should be 3 services running")
}

func intTestProcesses(t *testing.T) {
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		pidstr, err := exec.CommandContext(ctx, "pgrep", "test.test").Output()
		fmt.Printf("pids %+v", pidstr)
		fmt.Printf("error %+v", err)
		assert.Nil(t, err, "should be able to run pgrep to find honeydipper process")
		ppid := strings.Split(string(pidstr), "\n")[0]
		pidstr, err = exec.CommandContext(ctx, "/usr/bin/pgrep", "-P", ppid).Output()
		assert.Nil(t, err, "should be able to run pgrep to find all child processes")
		pids := strings.Split(string(pidstr), "\n")
		assert.Lenf(t, pids, 10, "expecting 10 child processes for honeydipper process")
	}()
}

func intTestDaemonShutdown(t *testing.T) {
	var graceful = make(chan bool)
	go func() {
		daemon.ShutDown()
		graceful <- true
	}()
	select {
	case <-graceful:
	case <-time.After(time.Second * 5):
		t.Errorf("service not shutdown after 5 seconds")
	}
}
