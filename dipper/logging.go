package dipper

import (
	"github.com/op/go-logging"
	"golang.org/x/crypto/ssh/terminal"
	"os"
)

var log *logging.Logger
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
func GetLogger(module string, verbosity string, logFile *os.File) *logging.Logger {
	logBackends = []logging.Backend{initLogBackend(logging.WARNING, os.Stderr)}
	level, err := logging.LogLevel(verbosity)
	if err != nil {
		panic(err)
	}

	if level > logging.WARNING {
		logBackends = append(logBackends, initLogBackend(level, logFile))
	}
	logging.SetBackend(logBackends...)
	log = logging.MustGetLogger(module)
	return log
}
