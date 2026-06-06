// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"io"
	"os"
	"strings"
	"sync"

	"github.com/op/go-logging"
	"golang.org/x/term"
)

// Logger provides methods to log to the configured logger backend.
//
// Deprecated: Use GetLogger to get a logger for a specific module.
var Logger *logging.Logger

// LoggingWriter is the writer used for sending logs.
var LoggingWriter io.Writer

// moduleLoggers stores loggers for different modules, ensuring uniqueness by module name.
var (
	moduleLoggers   = make(map[string]*logging.Logger)
	moduleLoggersMu sync.RWMutex
)

// backendInitialized tracks whether the global backend has been initialized.
var (
	backendInitialized bool
	backendMu          sync.Mutex
	logFileOut         *os.File
	logFileErr         *os.File
	logBackend         logging.Backend
)

func createLogBackend(level logging.Level, logFile *os.File, module string) logging.Backend {
	backend := logging.NewLogBackend(logFile, "", 0)

	formatStr := `%{time:15:04:05.000} %{module}.%{shortfunc} ▶ %{level:.4s} %{id:03x} %{message}`
	if term.IsTerminal(int(logFile.Fd())) {
		formatStr = `%{color}` + formatStr + `%{color:reset}`
	}
	format := logging.MustStringFormatter(formatStr)

	backendFormatter := logging.NewBackendFormatter(backend, format)
	backendLeveled := logging.AddModuleLevel(backendFormatter)
	backendLeveled.SetLevel(level, module)

	return backendLeveled
}

// GetLogger returns a logger for the specified module.
// The logger is unique by module name - calling with the same module name
// will return the same logger instance. Different modules can have different
// log prefixes even if they are in the same process.
//
// The backend is initialized on the first call. Subsequent calls with different
// log files will not re-initialize the backend. The verbosity parameter is
// only used on the first call or when the module's logger is first created.
//
// Returns the logger for the specified module.
func GetLogger(module string, verbosity string, logFiles ...*os.File) *logging.Logger {
	// Fast path: check if we already have a logger for this module (read lock)
	moduleLoggersMu.RLock()
	if logger, ok := moduleLoggers[module]; ok {
		moduleLoggersMu.RUnlock()
		// Always update global Logger for backward compatibility
		Logger = logger

		return logger
	}
	moduleLoggersMu.RUnlock()

	// Need to create a new logger for this module
	moduleLoggersMu.Lock()
	defer moduleLoggersMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have created it)
	if logger, ok := moduleLoggers[module]; ok {
		// Always update global Logger for backward compatibility
		Logger = logger

		return logger
	}

	// Check for debug mode - allow per-module debug override via DEBUG env var
	if debug, ok := os.LookupEnv("DEBUG"); ok {
		if debug == "*" || strings.Contains(","+debug+",", ","+module+",") {
			verbosity = "DEBUG"
		}
	}

	// Initialize the backend only once
	backendMu.Lock()
	if !backendInitialized {
		errLog := os.Stderr
		if len(logFiles) > 1 {
			errLog = logFiles[1]
		}
		log := os.Stdout
		if len(logFiles) > 0 {
			log = logFiles[0]
		}

		// Store the log files for potential future use
		logFileOut = log
		logFileErr = errLog

		// Determine default level based on DEBUG env var
		defaultLevel := logging.INFO
		if debug, ok := os.LookupEnv("DEBUG"); ok && debug == "*" {
			defaultLevel = logging.DEBUG
		}

		// Create backends - always include error log backend with WARNING level
		// Using empty string for module applies to all modules
		errBackend := createLogBackend(logging.WARNING, errLog, "")
		logBackend = createLogBackend(defaultLevel, log, "")

		// Set the global backend (affects all loggers)
		LoggingWriter = log
		logging.SetBackend(errBackend, logBackend)

		backendInitialized = true
	}
	backendMu.Unlock()

	// Get or create logger for this module
	logger := logging.MustGetLogger(module)

	// Set the level for this specific module using SetLevel
	level, err := logging.LogLevel(verbosity)
	if err != nil {
		panic(err)
	}

	// Get the backend and set the level for this specific module
	// We need to access the backend's SetLevel method for the specific module
	if lb, ok := logBackend.(interface{ SetLevel(logging.Level, string) }); ok {
		lb.SetLevel(level, module)
	}

	moduleLoggers[module] = logger

	// Always update the global Logger for backward compatibility
	Logger = logger

	return logger
}

// GetLogFiles returns the current log files being used by the backend.
// This is useful for testing or for modules that need to know the log destination.
func GetLogFiles() (*os.File, *os.File) {
	backendMu.Lock()
	defer backendMu.Unlock()

	return logFileOut, logFileErr
}
