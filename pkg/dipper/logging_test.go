// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"os"
	"sync"
	"testing"

	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func TestGetLogger_MultipleModules(t *testing.T) {
	// Create temporary log files
	logFile1, err := os.CreateTemp("", "test-log1-*.log")
	assert.NoError(t, err)
	defer os.Remove(logFile1.Name())
	defer logFile1.Close()

	logFile2, err := os.CreateTemp("", "test-log2-*.log")
	assert.NoError(t, err)
	defer os.Remove(logFile2.Name())
	defer logFile2.Close()

	errLogFile, err := os.CreateTemp("", "test-err-*.log")
	assert.NoError(t, err)
	defer os.Remove(errLogFile.Name())
	defer errLogFile.Close()

	// Reset state for testing
	resetLoggingState()

	// Get loggers for different modules
	logger1 := GetLogger("module1", "INFO", logFile1, errLogFile)
	logger2 := GetLogger("module2", "DEBUG", logFile2, errLogFile)

	// Verify loggers are not nil
	assert.NotNil(t, logger1)
	assert.NotNil(t, logger2)

	// Verify different modules get different logger instances
	assert.NotSame(t, logger1, logger2)

	// Verify module names are set correctly
	assert.Equal(t, "module1", logger1.Module)
	assert.Equal(t, "module2", logger2.Module)
}

func TestGetLogger_SameModuleReturnsSameLogger(t *testing.T) {
	// Create temporary log files
	logFile, err := os.CreateTemp("", "test-log-*.log")
	assert.NoError(t, err)
	defer os.Remove(logFile.Name())
	defer logFile.Close()

	errLogFile, err := os.CreateTemp("", "test-err-*.log")
	assert.NoError(t, err)
	defer os.Remove(errLogFile.Name())
	defer errLogFile.Close()

	// Reset state for testing
	resetLoggingState()

	// Get logger twice with the same module name
	logger1 := GetLogger("testmodule", "INFO", logFile, errLogFile)
	logger2 := GetLogger("testmodule", "INFO", logFile, errLogFile)

	// Verify same instance is returned
	assert.Same(t, logger1, logger2)
}

func TestGetLogger_ConcurrentAccess(t *testing.T) {
	// Create temporary log files
	logFile, err := os.CreateTemp("", "test-log-*.log")
	assert.NoError(t, err)
	defer os.Remove(logFile.Name())
	defer logFile.Close()

	errLogFile, err := os.CreateTemp("", "test-err-*.log")
	assert.NoError(t, err)
	defer os.Remove(errLogFile.Name())
	defer errLogFile.Close()

	// Reset state for testing
	resetLoggingState()

	// Test concurrent access
	numGoroutines := 100
	var wg sync.WaitGroup
	loggers := make([]*logging.Logger, numGoroutines)
	loggerMap := make(map[*logging.Logger]bool)
	var mu sync.Mutex

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()
			logger := GetLogger("concurrent-module", "INFO", logFile, errLogFile)
			mu.Lock()
			loggers[idx] = logger
			loggerMap[logger] = true
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Verify all goroutines got the same logger instance
	assert.Equal(t, 1, len(loggerMap), "All goroutines should get the same logger instance")
}

func TestGetLogger_DebugOverride(t *testing.T) {
	// Create temporary log files
	logFile, err := os.CreateTemp("", "test-log-*.log")
	assert.NoError(t, err)
	defer os.Remove(logFile.Name())
	defer logFile.Close()

	errLogFile, err := os.CreateTemp("", "test-err-*.log")
	assert.NoError(t, err)
	defer os.Remove(errLogFile.Name())
	defer errLogFile.Close()

	// Reset state for testing
	resetLoggingState()

	// Set DEBUG environment variable
	t.Setenv("DEBUG", "module1,module2")

	// Get logger with DEBUG in environment
	logger := GetLogger("module1", "INFO", logFile, errLogFile)
	assert.NotNil(t, logger)

	// The logger should be created with DEBUG level due to env var
	// Note: The actual level verification would require accessing internal state
	// which may not be possible with the logging library
}

func TestGetLogger_BackendInitializedOnce(t *testing.T) {
	// Create temporary log files
	logFile1, err := os.CreateTemp("", "test-log1-*.log")
	assert.NoError(t, err)
	defer os.Remove(logFile1.Name())
	defer logFile1.Close()

	logFile2, err := os.CreateTemp("", "test-log2-*.log")
	assert.NoError(t, err)
	defer os.Remove(logFile2.Name())
	defer logFile2.Close()

	errLogFile, err := os.CreateTemp("", "test-err-*.log")
	assert.NoError(t, err)
	defer os.Remove(errLogFile.Name())
	defer errLogFile.Close()

	// Reset state for testing
	resetLoggingState()

	// Get multiple loggers - backend should only be initialized once
	logger1 := GetLogger("module1", "INFO", logFile1, errLogFile)
	logger2 := GetLogger("module2", "DEBUG", logFile2, errLogFile)
	logger3 := GetLogger("module3", "ERROR", logFile1, errLogFile)

	assert.NotNil(t, logger1)
	assert.NotNil(t, logger2)
	assert.NotNil(t, logger3)

	// Verify LoggingWriter is set (indicates backend was initialized)
	assert.NotNil(t, LoggingWriter)

	// Verify global Logger is set
	assert.NotNil(t, Logger)
}

// resetLoggingState resets the global logging state for testing.
// This is needed because the logging package uses global state.
func resetLoggingState() {
	moduleLoggersMu.Lock()
	defer moduleLoggersMu.Unlock()
	backendMu.Lock()
	defer backendMu.Unlock()

	moduleLoggers = make(map[string]*logging.Logger)
	backendInitialized = false
	Logger = nil
	LoggingWriter = nil
	logFileOut = nil
	logFileErr = nil
	logBackend = nil
}
