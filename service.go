package main

import (
	"errors"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"log"
	"syscall"
	"time"
)

// NewService : create a Service with given config and name
func NewService(cfg *Config, name string) *Service {
	return &Service{
		name:           name,
		config:         cfg,
		driverRuntimes: map[string]DriverRuntime{},
		expects:        map[string][]func(*Message){},
	}
}

func (s *Service) loadFeature(feature string) (rerr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Resuming after error: %v\n", r)
			log.Printf("skip loading feature: %s.%s", s.name, feature)
			if err, ok := r.(error); ok {
				rerr = err
			} else {
				rerr = errors.New(fmt.Sprint(r))
			}
		}
	}()

	log.Printf("loading feature %s.%s\n", s.name, feature)
	featureMap := map[string](map[string]string){}
	if cfgItem, ok := s.config.getDriverData("daemon.featureMap"); ok {
		featureMap = cfgItem.(map[string](map[string]string))
	}
	driverName, ok := featureMap[s.name][feature]
	if !ok {
		driverName, ok = featureMap["global"][feature]
	}
	if !ok {
		driverName, ok = Defaults[feature]
	}
	if !ok {
		panic("driver not defined for the feature")
	}

	driverData, _ := s.config.getDriverData(driverName)

	driverMeta, ok := BuiltinDrivers[driverName]
	if !ok {
		if cfgItem, ok := s.config.getDriverData(fmt.Sprintf("daemon.drivers.%s", driverName)); ok {
			err := mapstructure.Decode(cfgItem, &driverMeta)
			if err != nil {
				panic("invalid driver metadata")
			}
		} else {
			panic("unable to get driver metadata")
		}
	}

	driverRuntime := DriverRuntime{
		meta: &driverMeta,
		data: &driverData,
	}

	switch Type, _ := getMapDataStr(driverMeta.Data, "Type"); Type {
	case "go":
		godriver := NewGoDriver(driverMeta.Data.(map[string]interface{})).Driver
		driverRuntime.driver = &godriver
		driverRuntime.start(s.name)
	default:
		panic(fmt.Sprintf("unknown driver type %s", Type))
	}

	s.driverRuntimes[feature] = driverRuntime
	return nil
}

func (s *Service) start() {
	log.Printf("starting service %s\n", s.name)
	go s.serviceLoop()
	for _, feature := range RequiredFeatures[s.name] {
		if err := s.loadFeature(feature); err != nil {
			log.Fatalf("failed to load service [%s] required feature [%s]", s.name, feature)
		}

		driverName := s.driverRuntimes[feature].meta.Name
		s.addExpect(
			"state:alive:"+driverName,
			func(*Message) {},
			5*time.Second,
			func() { log.Fatalf("failed to start driver %s.%s", s.name, driverName) },
		)
	}

	additionalFeatures := []string{}
	if cfgItem, ok := s.config.getDriverData("daemon.features.global"); ok {
		additionalFeatures = cfgItem.([]string)
	}
	if cfgItem, ok := s.config.getDriverData(fmt.Sprintf("daemon.features.%s", s.name)); ok {
		additionalFeatures = append(additionalFeatures, cfgItem.([]string)...)
	}

	for _, feature := range additionalFeatures {
		if driverRuntime, ok := s.driverRuntimes[feature]; !ok {
			if err := s.loadFeature(feature); err != nil {
				log.Printf("skip feature %s.%s error %v", s.name, feature, err)
			}
			driverName := driverRuntime.meta.Name
			s.addExpect(
				"state:alive:"+driverName,
				func(*Message) {},
				5*time.Second,
				func() { log.Printf("failed to start driver %s.%s", s.name, driverName) },
			)
		}
	}
}

func (s *Service) serviceLoop() {
	max := 0
	fds := &syscall.FdSet{}
	timeout := &syscall.Timeval{}
	timeout.Sec = 1
	for {
		fdZero(fds)
		for _, runtime := range s.driverRuntimes {
			fdSet(fds, runtime.input)
			if runtime.input > max {
				max = runtime.input
			}
		}

		err := syscall.Select(max+1, fds, nil, nil, timeout)
		if err != nil {
			log.Printf("select error")
			time.Sleep(time.Second)
		} else {
			for _, runtime := range s.driverRuntimes {
				if fdIsSet(fds, runtime.input) {
					msgs := runtime.fetchMessages()
					log.Printf("incoming message from %s.%s %+v", s.name, runtime.meta.Name, msgs)
					go s.process(msgs, &runtime)
				}
			}
		}
	}
}

func (s *Service) process(msgs []Message, runtime *DriverRuntime) {
	// process expect
	for _, msg := range msgs {
		expectKey := fmt.Sprintf("%s:%s:%s", msg.Channel, msg.Subject, runtime.meta.Name)
		if expects, ok := s.deleteExpect(expectKey); ok {
			for _, f := range expects {
				go f(&msg)
			}
		}
	}

	for _, msg := range msgs {
		key := fmt.Sprintf("%s:%s", msg.Channel, msg.Subject)
		// responder
		if responders, ok := s.responders[key]; ok {
			for _, f := range responders {
				go f(runtime, &msg)
			}
		}

		go func() {
			// transformer
			if transformers, ok := s.transformers[key]; ok {
				for _, f := range transformers {
					msg = f(runtime, &msg)
				}
			}

			// router
			// routedMsgs := s.Route(&msg)
			// for _, routedMsg := range routedMsgs {
			// 	routedMsg.driverRuntime.sendMessage(routedMsg.message)
			// }
		}()
	}
}

func (s *Service) addExpect(expectKey string, processor func(*Message), timeout time.Duration, except func()) {
	defer s.expectLock.Unlock()
	s.expectLock.Lock()
	s.expects[expectKey] = append(s.expects[expectKey], processor)
	go func() {
		time.Sleep(timeout)
		if expects, ok := s.isExpecting(expectKey); ok {
			func() {
				defer s.expectLock.Unlock()
				s.expectLock.Lock()
				if len(expects) > 1 {
					for i, p := range expects {
						if &p == &processor {
							expects = append(expects[:i], expects[i+1:]...)
							break
						}
					}
				} else {
					delete(s.expects, expectKey)
				}
			}()
			except()
		}
	}()
}

func (s *Service) isExpecting(expectKey string) ([]func(*Message), bool) {
	defer s.expectLock.Unlock()
	s.expectLock.Lock()
	ret, ok := s.expects[expectKey]
	return ret, ok
}

func (s *Service) deleteExpect(expectKey string) ([]func(*Message), bool) {
	defer s.expectLock.Unlock()
	s.expectLock.Lock()
	ret, ok := s.expects[expectKey]
	if ok {
		delete(s.expects, expectKey)
	}
	return ret, ok
}
