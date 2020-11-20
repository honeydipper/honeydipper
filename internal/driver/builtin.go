// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuiltinDriver are compiled and delivered with daemon binary in the same container image.
type BuiltinDriver struct {
	meta *Meta
}

// BuiltinPath is the path where the builtin drivers are kept. It will try using $HONEYDIPPER_DRIVERS_BUILTIN by default.
// If $HONEYDIPPER_DRIVERS_BUILTIN is not set, will try using $GOPATH/bin.  If $GOPATH is not defined, use "/opt/honeydipper/driver/builtin".
var BuiltinPath string

func builtinPath() string {
	if BuiltinPath == "" {
		driverPath, ok := os.LookupEnv("HONEYDIPPER_DRIVERS_BUILTIN")
		if ok {
			BuiltinPath = driverPath
		} else if gopath, ok := os.LookupEnv("GOPATH"); ok {
			BuiltinPath = filepath.Join(gopath, "bin")
		} else {
			BuiltinPath = "/opt/honeydipper/drivers/builtin"
		}
	}

	return BuiltinPath
}

// Acquire function is used for getting the driver from sources and validate, for.
// builtin drivers, just make sure the name is valid.
func (d *BuiltinDriver) Acquire() {
	shortName, ok := d.meta.HandlerData["shortName"].(string)
	if !ok || shortName == "" {
		panic(fmt.Errorf("%w: shortName is missing for builtin driver: %s", ErrDriverError, d.meta.Name))
	}
	if strings.ContainsRune(shortName, os.PathSeparator) {
		panic(fmt.Errorf("%w: shortName has path separator in driver: %s", ErrDriverError, d.meta.Name))
	}

	d.meta.Executable = filepath.Join(builtinPath(), shortName)
}

// Prepare function is used for preparing the arguments when calling the executable.
// for the driver.
func (d *BuiltinDriver) Prepare() {
	var argsList []interface{}

	args, ok := d.meta.HandlerData["arguments"]
	if ok {
		argsList, ok = args.([]interface{})
	} else {
		argsList = []interface{}{}
		ok = true
	}

	if !ok {
		panic(fmt.Errorf("%w: arguments in driver %s should be a list of strings", ErrDriverError, d.meta.Name))
	}

	for _, v := range argsList {
		d.meta.Arguments = append(d.meta.Arguments, fmt.Sprintf("%+v", v))
	}
}

// Meta function exposes the metadata used for this driver handler.
func (d *BuiltinDriver) Meta() *Meta {
	return d.meta
}

// NewBuiltinDriver creates a handler for the builtin driver specified in the meta info.
func NewBuiltinDriver(m *Meta) *BuiltinDriver {
	return &BuiltinDriver{meta: m}
}
