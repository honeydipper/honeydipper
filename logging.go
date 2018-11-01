package main

import (
	"github.com/op/go-logging"
	"os"
)

var log = logging.MustGetLogger("honeydipper")

var backend = logging.NewLogBackend(os.Stderr, "", 0)
var format = logging.MustStringFormatter(
	`%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
)
var backendFormatter = logging.NewBackendFormatter(backend, format)
var backendLeveled = logging.AddModuleLevel(backendFormatter)

func init() {
	backendLeveled.SetLevel(logging.DEBUG, "")
	logging.SetBackend(backendLeveled)
}
