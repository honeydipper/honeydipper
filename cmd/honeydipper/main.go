// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package honeydipper is an event-driven, rule based orchestration platform tailor towards
// DevOps and system engineering workflows.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/internal/service"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

var cfg config.Config

func initFlags() {
	flag.Usage = func() {
		msg := `
Usage:  %v [ -h ] service1 service2 ...

  -h            print this help message and quit

Supported services include engine, receiver, operator and configcheck.
If not specified, the daemon will load all services except configcheck.
Configcheck service is used for validating local uncommitted config files, and
other services will be ignored when using configcheck service.

See below for a listed environment variables that can be used.

REPO:           required, the bootstrap config repo or directory
BRANCH:         defaults to master, the branch to use in the bootstrap repo
BOOTSTRAP_PATH: defaults to /, the path from where to load init.yaml
CHECK_REMOTE:   defaults to false, when running config check specify if load and check remote repos

`
		fmt.Printf(msg, os.Args[0])
	}
}

func initEnv() {
	initFlags()
	flag.Parse()
	cfg = config.Config{InitRepo: config.RepoInfo{}, Services: flag.Args()}

	for _, s := range cfg.Services {
		if s == "configcheck" {
			cfg.Services = []string{s}
			cfg.IsConfigCheck = true
			if _, ok := os.LookupEnv("CHECK_REMOTE"); ok {
				cfg.CheckRemote = true
			}
			break
		}
	}
	getLogger()

	var ok bool
	if cfg.InitRepo.Repo, ok = os.LookupEnv("REPO"); !ok {
		log.Fatal("REPO environment variable is required to bootstrap honeydipper")
	}
	if cfg.InitRepo.Branch, ok = os.LookupEnv("BRANCH"); !ok {
		cfg.InitRepo.Branch = "master"
	}
	if cfg.InitRepo.Path, ok = os.LookupEnv("BOOTSTRAP_PATH"); !ok {
		cfg.InitRepo.Path = "/"
	}
}

func start() {
	// reset logging with configured loglevel
	getLogger()

	services := cfg.Services
	if len(services) == 0 {
		services = []string{"engine", "receiver", "operator"}
	}
	for _, s := range services {
		switch s {
		case "engine":
			service.StartEngine(&cfg)
		case "receiver":
			service.StartReceiver(&cfg)
		case "operator":
			service.StartOperator(&cfg)
		default:
			dipper.Logger.Fatalf("'%v' service is not implemented", s)
		}
	}
}

func reload() {
	// reset logging in case logging config changed
	getLogger()

	for _, service := range service.Services {
		go service.Reload()
	}
}

func getLogger() {
	levelstr, ok := cfg.GetDriverDataStr("daemon.loglevel")
	if !ok {
		levelstr = "INFO"
	}
	dipper.Logger = nil
	if cfg.IsConfigCheck {
		// suppress logging for less cluttered output for configcheck
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0777)
		dipper.GetLogger("daemon", levelstr, f, f)
	} else {
		dipper.GetLogger("daemon", levelstr)
	}
}

func main() {
	initEnv()
	if cfg.IsConfigCheck {
		exitCode := 0
		defer func() {
			os.Exit(exitCode)
		}()
		cfg.Bootstrap("/tmp")
		if runConfigCheck(&cfg) {
			// has error
			exitCode = 1
		}
	} else {
		cfg.OnChange = reload
		daemon.OnStart = start
		daemon.Run(&cfg)
	}
}
