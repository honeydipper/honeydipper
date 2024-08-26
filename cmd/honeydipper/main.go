// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package honeydipper is an event-driven, rule based orchestration platform tailor towards
// DevOps and system engineering workflows.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"

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
Honeydipper configurations. They can not be combined, and Honeydipper exits after
completing the desired auxiliary tasks instead of running as a daemon.

If services are not specified, Honeydipper will load all non-auxiliary services and
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
	if len(cfg.Services) == 0 {
		cfg.Services = []string{"engine", "receiver", "operator", "api"}
	}

loop:
	for _, s := range cfg.Services {
		switch s {
		case "configcheck":
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
		case "job":
			cfg.IsJobMode = true
			cfg.Services = []string{"engine", "operator"}

			break loop
		}
	}
	getLogger()

	switch {
	case cfg.IsDocGen:
		// DocGen doesnot require init repo.
	case cfg.IsJobMode:
		jobFile, ok := os.LookupEnv("JOB_FILE")
		if !ok {
			log.Fatal("JOB_FILE variable is required to start honeydipper in job mode.")
		}
		cfg.InitRepo = config.RepoInfo{
			Repo:     path.Dir(jobFile),
			Branch:   "",
			Path:     "/",
			InitFile: path.Base(jobFile),
		}
	default:
		var ok bool
		if cfg.InitRepo.Repo, ok = os.LookupEnv("REPO"); !ok {
			log.Fatal("REPO environment variable is required to bootstrap honeydipper")
		}
		if cfg.InitRepo.Path, ok = os.LookupEnv("BOOTSTRAP_PATH"); !ok {
			cfg.InitRepo.Path = "/"
		}
		cfg.InitRepo.Branch, _ = os.LookupEnv("BRANCH")
		cfg.InitRepo.InitFile, _ = os.LookupEnv("BOOTSTRAP_FILE")
	}
}

func start() {
	// reset logging with configured loglevel
	getLogger()

	for _, s := range cfg.Services {
		switch s {
		case "engine":
			service.StartEngine(&cfg)
		case "receiver":
			service.StartReceiver(&cfg)
		case "operator":
			service.StartOperator(&cfg)
		case "api":
			service.StartAPI(&cfg)
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
	levelstr, ok := cfg.GetStagedDriverDataStr("daemon.loglevel")
	if !ok {
		levelstr = "INFO"
	}
	dipper.Logger = nil
	if cfg.IsConfigCheck {
		// suppress logging for less cluttered output for configcheck
		//nolint:gomnd
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
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
		exitCode = loadAndRunConfigCheck(&cfg)
	case cfg.IsDocGen:
		runDocGen(&cfg)
	case cfg.IsJobMode:
		daemon.OnStart = start
		daemon.Run(&cfg)
	default:
		cfg.OnChange = reload
		daemon.OnStart = start
		daemon.Run(&cfg)
	}
}
