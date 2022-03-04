// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package driver

// Meta holds the meta information about the driver itself.
type Meta struct {
	Name        string
	Type        string
	Executable  string
	Arguments   []string
	HandlerData map[string]interface{}
}

// DriverStates represents driver states.
const (
	DriverLoading = iota
	DriverReloading
	DriverAlive
	DriverFailed
)
