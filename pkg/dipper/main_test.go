package dipper

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if Logger == nil {
		logFile, err := os.Create("test.log")
		if err != nil {
			panic(err)
		}
		GetLogger("test", "INFO", logFile, logFile)
	}
	os.Exit(m.Run())
}
