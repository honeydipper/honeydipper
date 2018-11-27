package dipper

import (
	"flag"
	"github.com/op/go-logging"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	logFile, err := os.Create("test.log")
	if err != nil {
		panic(err)
	}
	var backend = logging.NewLogBackend(logFile, "", 0)
	var format = logging.MustStringFormatter(
		`%{time:15:04:05.000} %{module}.%{shortfunc} ▶ %{level:.4s} %{id:03x} %{message}`,
	)
	var backendFormatter = logging.NewBackendFormatter(backend, format)
	var backendLeveled = logging.AddModuleLevel(backendFormatter)

	backendLeveled.SetLevel(logging.DEBUG, "")
	logging.SetBackend(backendLeveled)
	log = logging.MustGetLogger("dipperTest")
	os.Exit(m.Run())
}
