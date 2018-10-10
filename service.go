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
	log.Printf("starting service %s\n", s.name)
	for _, feature := range RequiredFeatures[s.name] {
		if runtime, err := loadFeature(s.config, s.name, feature); err == nil {
			s.driverRuntimes[feature] = runtime
		} else {
			log.Fatalf("failed to load service [%s] required feature [%s]", s.name, feature)
		}
	}

	additionalFeatures := []string{}
	if cfgItem, ok := s.config.getDriverData("daemon.features.global"); ok {
		additionalFeatures = cfgItem.([]string)
	}
	if cfgItem, ok := s.config.getDriverData(fmt.Sprintf("daemon.features.%s", s.name)); ok {
		additionalFeatures = append(additionalFeatures, cfgItem.([]string)...)
	}

	for _, feature := range additionalFeatures {
		if _, ok := s.driverRuntimes[feature]; !ok {
			if runtime, err := loadFeature(s.config, s.name, feature); err == nil {
				s.driverRuntimes[feature] = runtime
			}
		}
	}
}

// Services : a catalog of running services in this daemon process
var Services = map[string]*Service{}
