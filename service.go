package main

import (
	"fmt"
	"log"
)

// Service : service is a collection of daemon's feature
type Service struct {
	name           string
	config         *Config
	driverRuntimes map[string]DriverRuntime
}

// NewService : create a Service with given config and name
func NewService(cfg *Config, name string) *Service {
	return &Service{name, cfg, map[string]DriverRuntime{}}
}

func (s *Service) start() {
	for _, lookup := range RequiredFunctions[s.name] {
		if runtime, err := loadFunction(s.config, s.name, lookup); err == nil {
			s.driverRuntimes[lookup] = runtime
		} else {
			log.Fatalf("failed to load service [%s] required function [%s]", s.name, lookup)
		}
	}

	additionalFunctions := []string{}
	if cfgItem, ok := s.config.getDriverData("daemon.functions.global"); ok {
		additionalFunctions = cfgItem.([]string)
	}
	if cfgItem, ok := s.config.getDriverData(fmt.Sprintf("daemon.functions.%s", s.name)); ok {
		additionalFunctions = append(additionalFunctions, cfgItem.([]string)...)
	}

	for _, lookup := range additionalFunctions {
		if _, ok := s.driverRuntimes[lookup]; !ok {
			if runtime, err := loadFunction(s.config, s.name, lookup); err == nil {
				s.driverRuntimes[lookup] = runtime
			}
		}
	}
}

// Services : a catalog of running services in this daemon process
var Services = map[string]*Service{}
