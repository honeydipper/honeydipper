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

Supported services include engine, receiver, operator, docgen and configcheck.
Docgen and configcheck are auxiliary services used for helping users managing their
honeydipper configurations. They can not be combined, and honeydipper exits after
completing the desired auxiliary tasks instead of running as a daemon.

If services are not specified, honeydipper will load all non-auxiliary services and
run as a daemon.

Configcheck service is used for validating local uncommitted config files.

Docgen service is used for generating documents input for sphinx.

See below for a listed environment variables that can be used.

REPO:           required, the bootstrap config repo or directory
BRANCH:         defaults to master, the branch to use in the bootstrap repo
BOOTSTRAP_PATH: defaults to /, the path from where to load init.yaml

CHECK_REMOTE:   defaults to false, when running config check specify if load and check remote repos

DOCSRC:         defaults to docs/src, specify the source files for docgen
DOCDST:         defaults to docs/dst, specify the directory to store generated files for docgen

`
		fmt.Printf(msg, os.Args[0])
	}
}

func initEnv() {
	initFlags()
	flag.Parse()
	cfg = config.Config{InitRepo: config.RepoInfo{}, Services: flag.Args()}

loop:
	for _, s := range cfg.Services {
		switch s {
		case "configCheck":
			cfg.Services = []string{s}
			cfg.IsConfigCheck = true
			if _, ok := os.LookupEnv("CHECK_REMOTE"); ok {
				cfg.CheckRemote = true
			}
			break loop
		case "docgen":
			cfg.Services = []string{s}
			cfg.IsDocGen = true
			if val, ok := os.LookupEnv("DOCSRC"); ok {
				cfg.DocSrc = val
			} else {
				cfg.DocSrc = "docs/src"
			}
			if val, ok := os.LookupEnv("DOCDST"); ok {
				cfg.DocDst = val
			} else {
				cfg.DocDst = "docs/dst"
			}
			break loop
		}
	}
	getLogger()

	if !cfg.IsDocGen {
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
	switch {
	case cfg.IsConfigCheck:
		exitCode := 1
		defer func() {
			os.Exit(exitCode)
		}()
		cfg.Bootstrap("/tmp")
		exitCode = runConfigCheck(&cfg)
	case cfg.IsDocGen:
		runDocGen(&cfg)
	default:
		cfg.OnChange = reload
		daemon.OnStart = start
		daemon.Run(&cfg)
	}
}
