package driver

import (
	"github.com/honeyscience/honeydipper/pkg/dipper"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		logFile, err := os.Create("test.log")
		if err != nil {
			panic(err)
		}
		dipper.GetLogger("test", "INFO", logFile, logFile)
	}
	os.Exit(m.Run())
}
