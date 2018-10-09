package main

import (
	"log"
	"os/exec"
	"strings"
)

// GoDriver : a driver type that runs a golang program process
type GoDriver struct {
	Driver
	Package    string
	Executable string
	Arguments  []string
}

// NewGoDriver : create a driver object to run the golang program process
func NewGoDriver(data map[string]interface{}) GoDriver {
	pack, ok := data["Package"].(string)
	if !ok {
		log.Panic("Package is not sepcified in driver")
	}

	cmd, ok := data["Executable"].(string)
	if !ok {
		packParts := strings.Split(pack, "/")
		cmd = packParts[len(packParts)-1]
	}

	args, ok := data["Arguments"].([]string)
	if !ok {
		args = []string{}
	}

	driver := GoDriver{
		Package:    pack,
		Executable: cmd,
		Arguments:  args,
	}
	driver.Type = "go"
	return driver
}

func (g *GoDriver) start(runtime *DriverRuntime) {
	check := exec.Command("go", "list", g.Package)
	if err := check.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			install := exec.Command("go", "get", g.Package)
			if err := install.Run(); err != nil {
				log.Panic("Unable to install the go package for driver")
			}
		}
	}

	run := exec.Command(g.Executable, g.Arguments...)
	if input, err := run.StdoutPipe(); err != nil {
		log.Panicf("Unable to link to driver stdout %v", err)
	} else {
		runtime.input = &(input)
	}
	if output, err := run.StdinPipe(); err != nil {
		log.Panicf("Unable to link to driver stdin %v", err)
	} else {
		runtime.output = &(output)
	}
	if err := run.Start(); err != nil {
		log.Panicf("Failed to start driver %v", err)
	}
	runtime.driver = &g.Driver
}
