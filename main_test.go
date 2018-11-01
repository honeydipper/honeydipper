package main

import (
	"github.com/op/go-logging"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	logFile, err := os.Create("test.log")
	if err != nil {
		panic(err)
	}
	var backend = logging.NewLogBackend(logFile, "", 0)
	var format = logging.MustStringFormatter(
		`%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x} %{message}`,
	)
	var backendFormatter = logging.NewBackendFormatter(backend, format)
	var backendLeveled = logging.AddModuleLevel(backendFormatter)
	backendLeveled.SetLevel(logging.DEBUG, "")

	log.Warning("application output is written in test.log")
	logging.SetBackend(backendLeveled)
	os.Exit(m.Run())
}
