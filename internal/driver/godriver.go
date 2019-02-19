// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package driver

import (
	"github.com/honeyscience/honeydipper/internal/config"
	"github.com/honeyscience/honeydipper/pkg/dipper"
	"os/exec"
	"strings"
)

// GoDriver is a driver type that runs a golang executable program.
type GoDriver struct {
	driver  *config.Driver
	Package string
}

// NewGoDriver creates a driver object to run the golang executable program.
func NewGoDriver(data map[string]interface{}) GoDriver {
	drv := NewDriver(data)

	pack, ok := data["Package"].(string)
	if !ok {
		dipper.Logger.Panic("Package is not sepcified in driver")
	}

	if drv.Executable == "" {
		packParts := strings.Split(pack, "/")
		drv.Executable = packParts[len(packParts)-1]
	}

	drv.Type = "go"

	godriver := GoDriver{
		driver:  &drv,
		Package: pack,
	}
	return godriver
}

// PreStart will be executed before the driver process is started.
func (g GoDriver) PreStart(service string, runtime *Runtime) {
	dipper.Logger.Infof("[%s] pre-start dirver %s", service, runtime.Meta.Name)
	check := execCommand("go", "list", g.Package)
	outp, err := check.CombinedOutput()
	if err != nil {
		//nolint:gosimple
		if _, ok := err.(*exec.ExitError); ok {
			install := execCommand("go", "get", g.Package)
			if outp, err := install.CombinedOutput(); err != nil {
				dipper.Logger.Panicf("[%s] Unable to install the go package for driver [%s] %+v", service, runtime.Meta.Name, string(outp))
			}
		} else {
			dipper.Logger.Panicf("[%s] driver [%s] prestart failed %+v %+v", service, runtime.Meta.Name, err, string(outp))
		}
	}
}

// Driver provides access to the driver definition data.
func (g GoDriver) Driver() *config.Driver {
	return g.driver
}
