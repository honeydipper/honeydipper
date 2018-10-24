package main

import (
	"errors"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/mitchellh/mapstructure"
	"log"
	"reflect"
	"strings"
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

	service.responders["state:cold"] = []func(*DriverRuntime, *dipper.Message){coldReloadDriverRuntime}

	return service
}

func coldReloadDriverRuntime(d *DriverRuntime, m *dipper.Message) {
	s, _ := Services[d.service]
	s.checkDeleteDriverRuntime(d.feature, d)
	d.output.Close()
	s.loadFeature(d.feature)
}

func (s *Service) loadFeature(feature string) (affected bool, driverName string, rerr error) {
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

	if strings.HasPrefix(feature, "driver:") {
		driverName = feature[7:]
	} else {
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
	}
	log.Printf("[%s] mapping feature %s to driver %s", s.name, feature, driverName)

	driverData, _ := s.config.getDriverData(driverName)
	var dynamicData interface{}
	if strings.HasPrefix(feature, "driver:") {
		dynamicData, _ = s.dynamicFeatureData[feature]
	}
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
		feature:     feature,
		meta:        &driverMeta,
		data:        driverData,
		dynamicData: dynamicData,
	}

	if oldRuntime != nil && reflect.DeepEqual(*oldRuntime.meta, *driverRuntime.meta) {
		if reflect.DeepEqual(oldRuntime.data, driverRuntime.data) && reflect.DeepEqual(oldRuntime.dynamicData, driverRuntime.dynamicData) {
			log.Printf("[%s] driver not affected: %s", s.name, driverName)
		} else {
			// hot reload
			affected = true
			oldRuntime.data = driverRuntime.data
			oldRuntime.dynamicData = driverRuntime.dynamicData
			oldRuntime.sendOptions()
		}
	} else {
		// cold reload
		affected = true
		switch Type, _ := dipper.GetMapDataStr(driverMeta.Data, "Type"); Type {
		case "go":
			godriver := NewGoDriver(driverMeta.Data.(map[string]interface{})).Driver
			driverRuntime.driver = &godriver
			driverRuntime.start(s.name)
		default:
			panic(fmt.Sprintf("unknown driver type %s", Type))
		}

		s.setDriverRuntime(feature, &driverRuntime)
		go func(s *Service, feature string, runtime *DriverRuntime) {
			runtime.Run.Wait()
			func() {
				s.selectLock.Lock()
				defer s.selectLock.Unlock()
				close(runtime.stream)
			}()
			s.checkDeleteDriverRuntime(feature, runtime)
		}(s, feature, &driverRuntime)

		if oldRuntime != nil {
			// closing the output writer will cause child process to panic
			oldRuntime.output.Close()
		}
	}
	return affected, driverName, nil
}

func (s *Service) start() {
	log.Printf("[%s] starting service\n", s.name)
	featureList := s.getFeatureList()
	s.loadRequiredFeatures(featureList, true)
	go s.serviceLoop()
	s.loadAdditionalFeatures(featureList)
}

func (s *Service) reload() {
	log.Printf("[%s] reloading service\n", s.name)
	if s.ServiceReload != nil {
		s.ServiceReload(s.config)
	}
	featureList := s.getFeatureList()
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[%s] reverting config due to fatal failure %v\n", s.name, r)
				s.config.rollBack()
			}
		}()
		s.loadRequiredFeatures(featureList, false)
	}()
	s.loadAdditionalFeatures(featureList)
	s.removeUnusedFeatures(featureList)
}

func (s *Service) getFeatureList() map[string]bool {
	featureList := map[string]bool{}
	if cfgItem, ok := s.config.getDriverData("daemon.features.global"); ok {
		for _, feature := range cfgItem.([]interface{}) {
			featureList[feature.(string)] = false
		}
	}
	if cfgItem, ok := s.config.getDriverData("daemon.features." + s.name); ok {
		log.Printf("[%s] loaded data: %v", s.name, cfgItem)
		for _, feature := range cfgItem.([]interface{}) {
			featureList[feature.(string)] = false
		}
	}
	for _, feature := range RequiredFeatures[s.name] {
		featureList[feature] = true
	}
	if s.DiscoverFeatures != nil {
		s.dynamicFeatureData = s.DiscoverFeatures(s.config.config)
		for name := range s.dynamicFeatureData {
			featureList[name] = false
		}
	}
	return featureList
}

func (s *Service) removeUnusedFeatures(featureList map[string]bool) {
	for feature, runtime := range s.driverRuntimes {
		if _, ok := featureList[feature]; !ok {
			s.checkDeleteDriverRuntime(feature, nil)
			runtime.output.Close()
		}
	}
}

func (s *Service) loadRequiredFeatures(featureList map[string]bool, boot bool) {
	for feature, required := range featureList {
		if required {
			affected, driverName, err := s.loadFeature(feature)
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
}

func (s *Service) loadAdditionalFeatures(featureList map[string]bool) {
	for feature, required := range featureList {
		if !required {
			affected, driverName, err := s.loadFeature(feature)
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
}

func (s *Service) serviceLoop() {
	for {
		var cases []reflect.SelectCase
		var orderedRuntimes []*DriverRuntime
		func() {
			s.driverLock.Lock()
			defer s.driverLock.Unlock()
			cases = make([]reflect.SelectCase, len(s.driverRuntimes)+1)
			orderedRuntimes = make([]*DriverRuntime, len(s.driverRuntimes))
			i := 0
			for _, runtime := range s.driverRuntimes {
				cases[i] = reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(runtime.stream),
				}
				orderedRuntimes[i] = runtime
				i++
			}
		}()
		cases[len(cases)-1] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(time.After(time.Second)),
		}

		var chosen int
		var value reflect.Value
		var ok bool
		func() {
			s.selectLock.Lock()
			defer s.selectLock.Unlock()
			chosen, value, ok = reflect.Select(cases)
		}()

		if !ok {
			chosenRuntime := orderedRuntimes[chosen]
			s.checkDeleteDriverRuntime(chosenRuntime.feature, chosenRuntime)
		} else if chosen < len(orderedRuntimes) {
			func() {
				s.driverLock.Lock()
				defer s.driverLock.Unlock()
				go s.process(value.Interface().(dipper.Message), orderedRuntimes[chosen])
			}()
		}
	}
}

func (s *Service) process(msg dipper.Message, runtime *DriverRuntime) {
	expectKey := fmt.Sprintf("%s:%s:%s", msg.Channel, msg.Subject, runtime.meta.Name)
	if expects, ok := s.deleteExpect(expectKey); ok {
		for _, f := range expects {
			go f(&msg)
		}
	}

	if strings.HasPrefix(msg.Channel, "rpc") {
		s.handleRPC(runtime, &msg)
		return
	}

	key := fmt.Sprintf("%s:%s", msg.Channel, msg.Subject)
	// responder
	if responders, ok := s.responders[key]; ok {
		for _, f := range responders {
			go f(runtime, &msg)
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
	}(&msg)
}

func (s *Service) handleRPC(from *DriverRuntime, m *dipper.Message) bool {
	if m.Channel == "rpc" {
		parts := strings.SplitN(m.Subject, ".", 2)
		feature := parts[0]
		method := parts[1]
		callee := s.getDriverRuntime(feature)
		if callee == nil {
			log.Panicf("[%s] unable to find the rpc callee %s", s.name, feature)
		}
		dipper.SendRawMessage(callee.output, m.Channel, method+"."+from.feature, m.Payload.([]byte))
		return true
	} else if m.Channel == "rpcReply" {
		parts := strings.SplitN(m.Subject, ".", 2)
		feature := parts[0]
		rpcID := parts[1]
		caller := s.getDriverRuntime(feature)
		if caller == nil {
			log.Panicf("[%s] unable to find the rpc caller %s", s.name, feature)
		}
		dipper.SendRawMessage(caller.output, m.Channel, rpcID, m.Payload.([]byte))
		return true
	}
	return false
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
