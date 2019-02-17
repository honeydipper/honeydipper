// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import (
	"os"

	"github.com/op/go-logging"
	"golang.org/x/crypto/ssh/terminal"
)

// Logger provides methods to log to the configured logger backend.
var Logger *logging.Logger
var logBackends []logging.Backend

func initLogBackend(level logging.Level, logFile *os.File) logging.Backend {
	var backend = logging.NewLogBackend(logFile, "", 0)

	var formatStr = `%{time:15:04:05.000} %{module}.%{shortfunc} ▶ %{level:.4s} %{id:03x} %{message}`
	if terminal.IsTerminal(int(logFile.Fd())) {
		formatStr = `%{color}` + formatStr + `%{color:reset}`
	}
	var format = logging.MustStringFormatter(formatStr)

	var backendFormatter = logging.NewBackendFormatter(backend, format)
	var backendLeveled = logging.AddModuleLevel(backendFormatter)
	backendLeveled.SetLevel(level, "")

	return backendLeveled
}

// GetLogger : getting a logger for the module
func GetLogger(module string, verbosity string, logFiles ...*os.File) *logging.Logger {
	errLog := os.Stderr
	if len(logFiles) > 1 {
		errLog = logFiles[1]
	}
	logBackends = []logging.Backend{initLogBackend(logging.WARNING, errLog)}
	level, err := logging.LogLevel(verbosity)
	if err != nil {
		panic(err)
	}

	log := os.Stdout
	if len(logFiles) > 0 {
		log = logFiles[0]
	}
	if level > logging.WARNING {
		logBackends = append(logBackends, initLogBackend(level, log))
	}
	logging.SetBackend(logBackends...)
	Logger = logging.MustGetLogger(module)
	return Logger
}
