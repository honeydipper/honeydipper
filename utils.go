package main

import (
	"fmt"
	"github.com/mitchellh/mapstructure"
	"log"
	"strings"
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

	log.Printf("loading feature %s.%s\n", service, feature)
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

	driverRuntime := DriverRuntime{&driverMeta, &driverData, nil, nil, nil}

	switch Type, _ := getMapDataStr(driverMeta.Data, "Type"); Type {
	case "go":
		driver := NewGoDriver(driverMeta.Data.(map[string]interface{}))
		driver.start(service, &driverRuntime)
	}

	return driverRuntime, nil
}

func getMapData(from interface{}, path string) (ret interface{}, ok bool) {
	components := strings.Split(path, ".")
	var current = from
	for _, component := range components {
		if current, ok = current.(map[string]interface{})[component]; !ok {
			return nil, ok
		}
	}

	return current, true
}

func getMapDataStr(from interface{}, path string) (ret string, ok bool) {
	if data, ok := getMapData(from, path); ok {
		str, ok := data.(string)
		return str, ok
	}

	return "", ok
}

func forEachRecursive(prefixes []interface{}, from interface{}, routine func(key []interface{}, val string)) {
	if str, ok := from.(string); ok {
		routine(prefixes, str)
	} else if mp, ok := from.(map[interface{}]interface{}); ok {
		for key, value := range mp {
			childParts := prefixes
			forEachRecursive(append(childParts, key), value, routine)
		}
	}
}
