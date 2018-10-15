package main

import (
	"log"
	"os/exec"
	"strings"
)

// GoDriver : a driver type that runs a golang program process
type GoDriver struct {
	Driver
	Package string
}

// NewGoDriver : create a driver object to run the golang program process
func NewGoDriver(data map[interface{}]interface{}) GoDriver {
	driver := NewDriver(data)

	pack, ok := data["Package"].(string)
	if !ok {
		log.Panic("Package is not sepcified in driver")
	}

	if driver.Executable == "" {
		packParts := strings.Split(pack, "/")
		driver.Executable = packParts[len(packParts)-1]
	}

	godriver := GoDriver{
		Driver:  driver,
		Package: pack,
	}
	godriver.Type = "go"
	godriver.PreStart = godriver.preStart
	return godriver
}

func (g *GoDriver) preStart(service string, runtime *DriverRuntime) {
	log.Printf("[%s] pre-start dirver %s", service, runtime.meta.Name)
	check := exec.Command("go", "list", g.Package)
	if err := check.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			install := exec.Command("go", "get", g.Package)
			if err := install.Run(); err != nil {
				log.Panic("Unable to install the go package for driver")
			}
		}
	}
}
