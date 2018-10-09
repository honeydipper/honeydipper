package main

import (
	"log"
)

var engineRuntime struct {
	driverRuntimes map[string]DriverRuntime
}

var requiredEngineFunctions = []string{
	"eventbus",
}

func startEngine(cfg *Config) {
	engineRuntime.driverRuntimes = map[string]DriverRuntime{}
	for _, lookup := range requiredEngineFunctions {
		if runtime, err := loadFunction(cfg, "engine", lookup); err == nil {
			engineRuntime.driverRuntimes[lookup] = runtime
		} else {
			log.Fatalf("failed to load required function [%s]", lookup)
		}
	}

	additionalFunctions := []string{}
	if cfgItem, ok := cfg.getDriverData("daemon.functions.global"); ok {
		additionalFunctions = cfgItem.([]string)
	}
	if cfgItem, ok := cfg.getDriverData("daemon.functions.engine"); ok {
		additionalFunctions = append(additionalFunctions, cfgItem.([]string)...)
	}

	for _, lookup := range additionalFunctions {
		if _, ok := engineRuntime.driverRuntimes[lookup]; !ok {
			if runtime, err := loadFunction(cfg, "engine", lookup); err == nil {
				engineRuntime.driverRuntimes[lookup] = runtime
			}
		}
	}
}
