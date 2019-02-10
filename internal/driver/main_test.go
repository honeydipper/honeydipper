package driver

import (
	"os"
	"testing"

	"github.com/honeyscience/honeydipper/pkg/dipper"
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
