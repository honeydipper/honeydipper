package dipper

import (
	"github.com/op/go-logging"
	"os"
)

var log *logging.Logger

func init() {
	var backend = logging.NewLogBackend(os.Stderr, "", 0)
	var format = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} %{module}.%{shortfunc} ▶ %{level:.4s} %{id:03x} %{message}%{color:reset}`,
	)
	var backendFormatter = logging.NewBackendFormatter(backend, format)
	var backendLeveled = logging.AddModuleLevel(backendFormatter)

	backendLeveled.SetLevel(logging.DEBUG, "")
	logging.SetBackend(backendLeveled)
}

// GetLogger : getting a logger for the module
func GetLogger(module string) *logging.Logger {
	log = logging.MustGetLogger(module)
	return log
}
