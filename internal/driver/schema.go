package driver

import (
	"github.com/honeyscience/honeydipper/internal/config"
)

// DriverStates represents driver states.
const (
	DriverLoading = iota
	DriverReloading
	DriverAlive
	DriverFailed
)

// Handler is an interface that a driver implement to manage the lifecycle of itself.
type Handler interface {
	// Provides access to the driver definition data.
	Driver() *config.Driver
	// This will be executed before the driver process is started.
	PreStart(string, *Runtime)
}
