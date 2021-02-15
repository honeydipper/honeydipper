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
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/internal/service"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4"
)

var bootstrapPath string

func TestIntegrationStart(t *testing.T) {
	if !t.Run("starting up daemon", intTestDaemonStartup) {
		t.FailNow()
	}
	defer t.Run("shutting down daemon", intTestDaemonShutdown)
	t.Run("checking services", intTestServices)
	t.Run("checking processes", intTestProcesses)
	t.Run("checking API calls", intTestMakingAPICall)
	t.Run("checking crashed driver", intTestDriverCrash)
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
		repo, err := git.PlainOpen("../..")
		if err != nil {
			panic(err)
		}
		currentBranch, err := repo.Head()
		if err != nil {
			panic(err)
		}
		ref := strings.Split(string(currentBranch.Name()), "/")
		workingBranch = ref[len(ref)-1]
	}
	cfg := config.Config{
		Services: []string{"engine", "receiver", "operator", "api"},
		InitRepo: config.RepoInfo{
			Repo:   "../..",
			Branch: workingBranch,
			Path:   "/cmd/honeydipper/test_fixtures/bootstrap",
		},
	}
	go func() {
		daemon.OnStart = func() {
			service.StartEngine(&cfg)
			service.StartReceiver(&cfg)
			service.StartOperator(&cfg)
			service.StartAPI(&cfg)
		}
		daemon.Run(&cfg)
	}()
}

func intTestServices(t *testing.T) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	timeout := time.After(time.Second * 30)

waitForServices:
	for {
		fmt.Println("waiting for services")
		select {
		case <-ticker.C:
			if len(service.Services) == 4 {
				break waitForServices
			}
		case <-timeout:
			break waitForServices
		}
	}

	assert.True(t, runtime.NumGoroutine() > 10, "running goroutine should be more than 10")

	_, ok := service.Services["receiver"]
	assert.True(t, ok, "receiver service should be running")
	_, ok = service.Services["engine"]
	assert.True(t, ok, "engine service should be running")
	_, ok = service.Services["operator"]
	assert.True(t, ok, "operator service should be running")
	_, ok = service.Services["api"]
	assert.True(t, ok, "api service should be running")
	assert.True(t, len(service.Services) == 4, "there should be 4 services running")
}

func intTestProcesses(t *testing.T) {
	var (
		pidstr []byte
		err    error
		pids   []string
	)
	ppid := strconv.Itoa(os.Getpid())
	fmt.Println("PID:", ppid)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	timeout := time.After(time.Second * 30)

waitForProcesses:
	for {
		fmt.Println("waiting for processes")
		select {
		case <-ticker.C:
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				pidstr, err = exec.CommandContext(ctx, "/usr/bin/pgrep", "-P", ppid).Output()
			}()
			if err == nil {
				pids = strings.Split(string(pidstr), "\n")
				if len(pids) >= 18 {
					break waitForProcesses
				}
			}
		case <-timeout:
			break waitForProcesses
		}
	}

	assert.Nil(t, err, "should be able to run pgrep to find all child processes")
	assert.Lenf(t, pids, 18, "expecting 17 child processes for honeydipper process")
}

func intTestMakingAPICall(t *testing.T) {
	// making an api call with wrong credentials
	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://localhost:9000/api/events", nil)
	assert.NoErrorf(t, err, "creating http request should not receive error")
	req.Header.Add("Authorization", "bearer wrongcredentials")
	resp, err := client.Do(req)
	defer resp.Body.Close()
	assert.NoErrorf(t, err, "api call should not receive error")
	assert.Equalf(t, 401, resp.StatusCode, "api call should fail with bad creds")

	// making an api call with correct credentials
	req, err = http.NewRequest("GET", "http://localhost:9000/api/events", nil)
	assert.NoErrorf(t, err, "creating http request should not receive error")
	req.Header.Add("Authorization", "bearer abcdefg")
	resp, err = client.Do(req)
	defer resp.Body.Close()
	assert.NoErrorf(t, err, "api call should not receive error")
	assert.Equalf(t, 200, resp.StatusCode, "api call should succeed with correct creds")
}

func intTestDaemonShutdown(t *testing.T) {
	graceful := make(chan bool)
	go func() {
		daemon.ShutDown()
		graceful <- true
	}()
	select {
	case <-graceful:
	case <-time.After(time.Second * 10):
		t.Errorf("service not shutdown after 10 seconds")
	}
}

func intTestDriverCrash(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var (
		pidstr []byte
		err    error
	)
	if runtime.GOOS == "darwin" {
		pidstr, err = exec.CommandContext(ctx, "pgrep", "-a", "honeydipper.test").Output()
	} else {
		pidstr, err = exec.CommandContext(ctx, "pgrep", "honeydipper.test").Output()
	}
	assert.Nil(t, err, "should be able to run pgrep to find honeydipper process")
	ppid := strings.Split(string(pidstr), "\n")[0]
	pidstr, err = exec.CommandContext(ctx, "/usr/bin/pgrep", "-P", ppid, "gcloud-dataflow").Output()
	assert.Nil(t, err, "should be able to run pgrep to find gcloud-dataflow driver processes")
	childpid := strings.Split(string(pidstr), "\n")[0]
	_, err = exec.CommandContext(ctx, "/bin/kill", childpid).Output()
	assert.Nil(t, err, "should be able to simulate a driver crash by killing the process")

	pidstr = nil
	for pidstr == nil {
		select {
		case <-ctx.Done():
			break
		default:
			time.Sleep(time.Second)
			pidstr, err = exec.CommandContext(ctx, "/usr/bin/pgrep", "-P", ppid, "gcloud-dataflow").Output()
			if err != nil {
				pidstr = nil
			}
		}
	}
	assert.Nil(t, err, "should be able to run pgrep to find new gcloud-dataflow driver processes")
	newchildpid := strings.Split(string(pidstr), "\n")[0]
	assert.NotEqual(t, "", newchildpid, "new driver pid should not be blank")
	assert.NotEqual(t, childpid, newchildpid, "new driver pid should be different than the old pid")
}
