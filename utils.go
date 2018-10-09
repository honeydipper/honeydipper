package main

import (
	"fmt"
	"github.com/mitchellh/mapstructure"
	"log"
)

func safeExitOnError(args ...interface{}) {
	if r := recover(); r != nil {
		log.Printf("Resuming after error: %v\n", r)
		log.Printf(args[0].(string), args[1:]...)
	}
}

func loadFunction(cfg *Config, service string, lookup string) (ret DriverRuntime, rerr interface{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Resuming after error: %v\n", r)
			log.Printf("skip loading function: %s.%s", service, lookup)
			rerr = r
		}
	}()

	functionMap := map[string](map[string]string){}
	if cfgItem, ok := cfg.getDriverData("daemon.functionMap"); ok {
		functionMap = cfgItem.(map[string](map[string]string))
	}
	driverName, ok := functionMap[service][lookup]
	if !ok {
		driverName, ok = functionMap["global"][lookup]
	}
	if !ok {
		driverName, ok = Defaults[lookup]
	}
	if !ok {
		return DriverRuntime{}, fmt.Sprintf("unable to find a driver for [%s]", lookup)
	}

	driverData, _ := cfg.getDriverData(driverName)

	driverMeta, ok := builtinDrivers[driverName]
	if !ok {
		var cfgItem interface{}
		if cfgItem, ok = cfg.getDriverData(fmt.Sprintf("daemon.drivers.%s", driverName)); ok {
			err := mapstructure.Decode(cfgItem, &driverMeta)
			if err != nil {
				ok = false
			}
		}
	}
	if !ok {
		return DriverRuntime{}, fmt.Sprintf("driver metadata is not defined for [%s]", driverName)
	}

	// execute
	return DriverRuntime{&driverMeta, &driverData, 0, 0, 0}, nil
}
