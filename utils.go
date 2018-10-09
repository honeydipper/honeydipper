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

func loadFeature(cfg *Config, service string, feature string) (ret DriverRuntime, rerr interface{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Resuming after error: %v\n", r)
			log.Printf("skip loading feature: %s.%s", service, feature)
			rerr = r
		}
	}()

	featureMap := map[string](map[string]string){}
	if cfgItem, ok := cfg.getDriverData("daemon.featureMap"); ok {
		featureMap = cfgItem.(map[string](map[string]string))
	}
	driverName, ok := featureMap[service][feature]
	if !ok {
		driverName, ok = featureMap["global"][feature]
	}
	if !ok {
		driverName, ok = Defaults[feature]
	}
	if !ok {
		return DriverRuntime{}, fmt.Sprintf("unable to find a driver for [%s]", feature)
	}

	driverData, _ := cfg.getDriverData(driverName)

	driverMeta, ok := BuiltinDrivers[driverName]
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
