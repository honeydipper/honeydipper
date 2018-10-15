package main

import (
	"errors"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/mitchellh/mapstructure"
	"log"
	"reflect"
	"syscall"
	"time"
)

// NewService : create a Service with given config and name
func NewService(cfg *Config, name string) *Service {
	service := &Service{
		name:           name,
		config:         cfg,
		driverRuntimes: map[string]*DriverRuntime{},
		expects:        map[string][]func(*dipper.Message){},
		responders:     map[string][]func(*DriverRuntime, *dipper.Message){},
	}

	service.responders["state:cold"] = []func(*DriverRuntime, *dipper.Message){coldReload}

	return service
}

func coldReload(d *DriverRuntime, m *dipper.Message) {
	s, _ := Services[d.service]
	s.checkDeleteDriverRuntime(d.feature, d)
	(*d.output).Close()
	s.reloadFeature(d.feature)
}

func (s *Service) reloadFeature(feature string) (affected bool, driverName string, rerr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Resuming after error: %v\n", r)
			log.Printf("[%s] skip reloading feature: %s", s.name, feature)
			if err, ok := r.(error); ok {
				rerr = err
			} else {
				rerr = errors.New(fmt.Sprint(r))
			}
		}
	}()
	log.Printf("[%s] reloading feature %s\n", s.name, feature)

	oldRuntime := s.getDriverRuntime(feature)

	featureMap, ok := s.config.getDriverData("daemon.featureMap")
	if ok {
		driverName, ok = dipper.GetMapDataStr(featureMap, s.name+"."+feature)
		if !ok {
			driverName, ok = dipper.GetMapDataStr(featureMap, "global."+feature)
		}
	}
	if !ok {
		driverName, ok = Defaults[feature]
	}
	if !ok {
		panic("driver not defined for the feature")
	}
	log.Printf("[%s] mapping feature %s to driver %s", s.name, feature, driverName)

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

	log.Printf("[%s] driver %s meta %v", s.name, driverName, driverMeta)
	driverRuntime := DriverRuntime{
		feature: feature,
		meta:    &driverMeta,
		data:    &driverData,
	}

	if oldRuntime != nil && reflect.DeepEqual(*oldRuntime.meta, *driverRuntime.meta) {
		if reflect.DeepEqual(*oldRuntime.data, *driverRuntime.data) {
			log.Printf("[%s] driver not affected: %s", s.name, driverName)
		} else {
			// hot reload
			affected = true
			oldRuntime.data = driverRuntime.data
			oldRuntime.sendOptions()
		}
	} else {
		// cold reload
		affected = true
		switch Type, _ := dipper.GetMapDataStr(driverMeta.Data, "Type"); Type {
		case "go":
			godriver := NewGoDriver(driverMeta.Data.(map[interface{}]interface{})).Driver
			driverRuntime.driver = &godriver
			driverRuntime.start(s.name)
		default:
			panic(fmt.Sprintf("unknown driver type %s", Type))
		}

		s.setDriverRuntime(feature, &driverRuntime)
		go func(s *Service, feature string, runtime *DriverRuntime) {
			runtime.Run.Wait()
			s.checkDeleteDriverRuntime(feature, runtime)
		}(s, feature, &driverRuntime)

		if oldRuntime != nil {
			// closing the output writer will cause child process to panic
			(*oldRuntime.output).Close()
		}
	}
	return affected, driverName, nil
}

func (s *Service) start() {
	log.Printf("[%s] starting service\n", s.name)
	s.reloadRequiredFeatures(true)
	go s.serviceLoop()
	s.reloadAdditionalFeatures()
}

func (s *Service) reload() {
	log.Printf("[%s] reloading service\n", s.name)
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[%s] reverting config due to fatal failure %v\n", s.name, r)
				s.config.rollBack()
			}
		}()
		s.reloadRequiredFeatures(false)
	}()
	s.reloadAdditionalFeatures()
}

func (s *Service) reloadRequiredFeatures(boot bool) {
	for _, feature := range RequiredFeatures[s.name] {
		affected, driverName, err := s.reloadFeature(feature)
		if err != nil {
			if boot {
				log.Fatalf("[%s] failed to load required feature [%s]", s.name, feature)
			} else {
				log.Panicf("[%] failed to reload required feature [%s]", s.name, feature)
			}
		}
		if affected {
			s.addExpect(
				"state:alive:"+driverName,
				func(*dipper.Message) {},
				10*time.Second,
				func() {
					if boot {
						log.Fatalf("failed to start driver %s.%s", s.name, driverName)
					} else {
						log.Printf("failed to reload driver %s.%s", s.name, driverName)
						s.config.rollBack()
					}
				},
			)
		}
	}
}

func (s *Service) reloadAdditionalFeatures() {
	var additionalFeatures []interface{}
	if cfgItem, ok := s.config.getDriverData("daemon.features.global"); ok {
		additionalFeatures = cfgItem.([]interface{})
	}
	if cfgItem, ok := s.config.getDriverData("daemon.features." + s.name); ok {
		log.Printf("[%s] loaded data: %v", s.name, cfgItem)
		additionalFeatures = append(additionalFeatures, cfgItem.([]interface{})...)
	}
	log.Printf("[%s] features: %v", s.name, additionalFeatures)

	for _, ifeature := range additionalFeatures {
		feature := ifeature.(string)
		if _, ok := RequiredFeatures[feature]; ok {
			log.Printf("[%s] builtin feature %s already processed", s.name, feature)
			break
		}
		affected, driverName, err := s.reloadFeature(feature)
		if err != nil {
			log.Printf("[%s] skip feature %s error %v", s.name, feature, err)
		}
		if affected {
			s.addExpect(
				"state:alive:"+driverName,
				func(*dipper.Message) {},
				10*time.Second,
				func() { log.Printf("[%s] failed to start or reload driver %s", s.name, driverName) },
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
		dipper.FdZero(fds)
		func() {
			s.driverLock.Lock()
			defer s.driverLock.Unlock()
			for _, runtime := range s.driverRuntimes {
				dipper.FdSet(fds, runtime.input)
				if runtime.input > max {
					max = runtime.input
				}
			}
		}()

		err := syscall.Select(max+1, fds, nil, nil, timeout)
		if err != nil {
			log.Printf("[%s] select error %v", s.name, err)
			time.Sleep(time.Second)
		} else {
			func() {
				s.driverLock.Lock()
				defer s.driverLock.Unlock()
				for _, runtime := range s.driverRuntimes {
					if dipper.FdIsSet(fds, runtime.input) {
						msgs := runtime.fetchMessages()
						log.Printf("[%s] incoming messages from %s", s.name, runtime.meta.Name)
						go s.process(msgs, runtime)
					}
				}
			}()
		}
	}
}

func (s *Service) process(msgs []*dipper.Message, runtime *DriverRuntime) {
	// process expect
	for _, msg := range msgs {
		expectKey := fmt.Sprintf("%s:%s:%s", msg.Channel, msg.Subject, runtime.meta.Name)
		if expects, ok := s.deleteExpect(expectKey); ok {
			for _, f := range expects {
				go f(msg)
			}
		}
	}

	for _, msg := range msgs {
		key := fmt.Sprintf("%s:%s", msg.Channel, msg.Subject)
		// responder
		if responders, ok := s.responders[key]; ok {
			for _, f := range responders {
				go f(runtime, msg)
			}
		}

		go func(msg *dipper.Message) {
			// transformer
			if transformers, ok := s.transformers[key]; ok {
				for _, f := range transformers {
					msg = f(runtime, msg)
				}
			}

			// router
			routedMsgs := s.Route(msg)
			for _, routedMsg := range routedMsgs {
				routedMsg.driverRuntime.sendMessage(routedMsg.message)
			}
		}(msg)
	}
}

func (s *Service) addExpect(expectKey string, processor func(*dipper.Message), timeout time.Duration, except func()) {
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

func (s *Service) isExpecting(expectKey string) ([]func(*dipper.Message), bool) {
	defer s.expectLock.Unlock()
	s.expectLock.Lock()
	ret, ok := s.expects[expectKey]
	return ret, ok
}

func (s *Service) deleteExpect(expectKey string) ([]func(*dipper.Message), bool) {
	defer s.expectLock.Unlock()
	s.expectLock.Lock()
	ret, ok := s.expects[expectKey]
	if ok {
		delete(s.expects, expectKey)
	}
	return ret, ok
}

func (s *Service) getDriverRuntime(feature string) *DriverRuntime {
	runtime, ok := dipper.LockGetMap(&s.driverLock, s.driverRuntimes, feature)
	if ok && runtime != nil {
		return runtime.(*DriverRuntime)
	}
	return nil
}

func (s *Service) setDriverRuntime(feature string, runtime *DriverRuntime) *DriverRuntime {
	oldone := dipper.LockSetMap(&s.driverLock, s.driverRuntimes, feature, runtime)
	if oldone != nil {
		return oldone.(*DriverRuntime)
	}
	return nil
}

func (s *Service) checkDeleteDriverRuntime(feature string, check *DriverRuntime) *DriverRuntime {
	oldone := dipper.LockCheckDeleteMap(&s.driverLock, s.driverRuntimes, feature, check)
	if oldone != nil {
		return oldone.(*DriverRuntime)
	}
	return nil
}
