// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package driver

import (
	"github.com/honeydipper/honeydipper/internal/config"
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
