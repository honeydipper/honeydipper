package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/mitchellh/mapstructure"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// NewService : create a Service with given config and name
func NewService(cfg *Config, name string) *Service {
	service := &Service{
		name:           name,
		config:         cfg,
		driverRuntimes: map[string]*DriverRuntime{},
		expects:        map[string][]ExpectHandler{},
		responders:     map[string][]MessageResponder{},
	}
	service.RPC.Caller.Init("rpc", "call")

	service.responders["state:cold"] = []MessageResponder{coldReloadDriverRuntime}
	service.responders["rpc:call"] = []MessageResponder{handleRPCCall}
	service.responders["rpc:return"] = []MessageResponder{handleRPCReturn}
	service.responders["broadcast:reload"] = []MessageResponder{handleReload}

	return service
}

func (s *Service) decryptDriverData(key string, val interface{}) (ret interface{}, replace bool) {
	if str, ok := val.(string); ok {
		if strings.HasPrefix(str, "ENC[") {
			parts := strings.SplitN(str[4:len(str)-1], ",", 2)
			encDriver := parts[0]
			data := []byte(parts[1])
			decoded, err := base64.StdEncoding.DecodeString(string(data))
			if err != nil {
				log.Panicf("encrypted data shoud be base64 encoded")
			}
			decrypted, _ := s.RPCCallRaw("driver:"+encDriver, "decrypt", decoded)
			return string(decrypted), true
		}
	}
	return nil, false
}

func (s *Service) loadFeature(feature string) (affected bool, driverName string, rerr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Infof("Resuming after error: %v", r)
			log.Infof("[%s] skip reloading feature: %s", s.name, feature)
			s.setDriverRuntime(feature, &DriverRuntime{state: DriverFailed})
			if err, ok := r.(error); ok {
				rerr = err
			} else {
				rerr = errors.New(fmt.Sprint(r))
			}
		}
	}()
	log.Warningf("[%s] reloading feature %s", s.name, feature)

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
	log.Infof("[%s] mapping feature %s to driver %s", s.name, feature, driverName)

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

	dipper.Recursive(driverData, s.decryptDriverData)
	dipper.Recursive(dynamicData, s.decryptDriverData)

	log.Debugf("[%s] driver %s meta %v", s.name, driverName, driverMeta)
	driverRuntime := DriverRuntime{
		feature:     feature,
		meta:        &driverMeta,
		data:        driverData,
		dynamicData: dynamicData,
		state:       DriverLoading,
	}

	if oldRuntime != nil && oldRuntime.state != DriverFailed && reflect.DeepEqual(*oldRuntime.meta, *driverRuntime.meta) {
		if reflect.DeepEqual(oldRuntime.data, driverRuntime.data) && reflect.DeepEqual(oldRuntime.dynamicData, driverRuntime.dynamicData) {
			log.Infof("[%s] driver not affected: %s", s.name, driverName)
		} else {
			// hot reload
			affected = true
			oldRuntime.data = driverRuntime.data
			oldRuntime.dynamicData = driverRuntime.dynamicData
			oldRuntime.state = DriverReloading
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
	go func() {
		log.Infof("[%s] starting service", s.name)
		featureList := s.getFeatureList()
		s.loadRequiredFeatures(featureList, true)
		go s.serviceLoop()
		time.Sleep(time.Second)
		s.loadAdditionalFeatures(featureList)
		go s.metricsLoop()
	}()
}

func (s *Service) reload() {
	log.Infof("[%s] reloading service", s.name)
	if s.ServiceReload != nil {
		s.ServiceReload(s.config)
	}
	featureList := s.getFeatureList()
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("[%s] reverting config due to fatal failure %v", s.name, r)
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
			featureName := feature.(map[string]interface{})["name"].(string)
			featureList[featureName], _ = dipper.GetMapDataBool(feature, "required")
		}
	}
	if cfgItem, ok := s.config.getDriverData("daemon.features." + s.name); ok {
		for _, feature := range cfgItem.([]interface{}) {
			featureName := feature.(map[string]interface{})["name"].(string)
			featureList[featureName], _ = dipper.GetMapDataBool(feature, "required")
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
	log.Debugf("[%s] final feature list %+v", s.name, featureList)
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
					log.Panicf("[%s] failed to reload required feature [%s]", s.name, feature)
				}
			}
			if affected {
				func(feature string, driverName string) {
					s.addExpect(
						"state:alive:"+driverName,
						func(*dipper.Message) {
							s.driverRuntimes[feature].state = DriverAlive
						},
						10*time.Second,
						func() {
							if boot {
								log.Fatalf("failed to start driver %s.%s", s.name, driverName)
							} else {
								log.Warningf("failed to reload driver %s.%s", s.name, driverName)
								s.driverRuntimes[feature].state = DriverFailed
								s.config.rollBack()
							}
						},
					)
				}(feature, driverName)
			}
		}
	}
}

func (s *Service) loadAdditionalFeatures(featureList map[string]bool) {
	for feature, required := range featureList {
		if !required {
			affected, driverName, err := s.loadFeature(feature)
			if err != nil {
				log.Infof("[%s] skip feature %s error %v", s.name, feature, err)
			}
			if affected {
				func(feature string, driverName string) {
					s.addExpect(
						"state:alive:"+driverName,
						func(*dipper.Message) {
							s.driverRuntimes[feature].state = DriverAlive
						},
						10*time.Second,
						func() {
							log.Warningf("[%s] failed to start or reload driver %s", s.name, driverName)
							s.driverRuntimes[feature].state = DriverFailed
						},
					)
				}(feature, driverName)
			}
		}
	}
}

func (s *Service) serviceLoop() {
	daemonChildren.Add(1)
	defer daemonChildren.Done()
	for !shuttingDown {
		var cases []reflect.SelectCase
		var orderedRuntimes []*DriverRuntime
		func() {
			s.driverLock.Lock()
			defer s.driverLock.Unlock()
			cases = []reflect.SelectCase{}
			orderedRuntimes = []*DriverRuntime{}
			for _, runtime := range s.driverRuntimes {
				if runtime.state != DriverFailed {
					cases = append(cases, reflect.SelectCase{
						Dir:  reflect.SelectRecv,
						Chan: reflect.ValueOf(runtime.stream),
					})
					orderedRuntimes = append(orderedRuntimes, runtime)
				}
			}
		}()
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(time.After(time.Second)),
		})

		var chosen int
		var value reflect.Value
		var ok bool
		func() {
			s.selectLock.Lock()
			defer s.selectLock.Unlock()
			chosen, value, ok = reflect.Select(cases)
		}()

		if !ok {
			if chosen < len(orderedRuntimes) {
				orderedRuntimes[chosen].state = DriverFailed
			}
		} else if chosen < len(orderedRuntimes) {
			func() {
				runtime := orderedRuntimes[chosen]
				msg := value.Interface().(dipper.Message)
				if runtime.feature != "emitter" {
					if emitter, ok := s.driverRuntimes["emitter"]; ok && emitter.state == DriverAlive {
						s.counterIncr("honey.honeydipper.local.message", []string{
							"service:" + s.name,
							"driver:" + runtime.meta.Name,
							"direction:inbound",
							"channel:" + msg.Channel,
							"subject:" + msg.Subject,
						})
					}
				}

				s.driverLock.Lock()
				defer s.driverLock.Unlock()
				go s.process(msg, runtime)
			}()
		}
	}
	for _, runtime := range s.driverRuntimes {
		runtime.output.Close()
	}
	log.Warningf("[%s] service closed for business", s.name)
}

func (s *Service) process(msg dipper.Message, runtime *DriverRuntime) {
	defer dipper.SafeExitOnError("[%s] continue  message loop", s.name)
	expectKey := fmt.Sprintf("%s:%s:%s", msg.Channel, msg.Subject, runtime.meta.Name)
	if expects, ok := s.deleteExpect(expectKey); ok {
		for _, f := range expects {
			go f(&msg)
		}
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

func (s *Service) addExpect(expectKey string, processor ExpectHandler, timeout time.Duration, except func()) {
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

func (s *Service) isExpecting(expectKey string) ([]ExpectHandler, bool) {
	defer s.expectLock.Unlock()
	s.expectLock.Lock()
	ret, ok := s.expects[expectKey]
	return ret, ok
}

func (s *Service) deleteExpect(expectKey string) ([]ExpectHandler, bool) {
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
		oldruntime := oldone.(*DriverRuntime)
		return oldruntime
	}
	return nil
}

// RPCCallRaw : making a PRC call with raw bytes from driver to another driver
func (s *Service) RPCCallRaw(feature string, method string, params []byte) ([]byte, error) {
	return s.RPC.Caller.CallRaw(s.getDriverRuntime(feature).output, feature, method, params)
}

// RPCCall : making a PRC call from driver to another driver
func (s *Service) RPCCall(feature string, method string, params interface{}) ([]byte, error) {
	return s.RPC.Caller.Call(s.getDriverRuntime(feature).output, feature, method, params)
}

func coldReloadDriverRuntime(d *DriverRuntime, m *dipper.Message) {
	s := Services[d.service]
	s.checkDeleteDriverRuntime(d.feature, d)
	d.output.Close()
	s.loadFeature(d.feature)
}

func handleRPCCall(from *DriverRuntime, m *dipper.Message) {
	feature := m.Labels["feature"]
	m.Labels["caller"] = from.feature
	s := Services[from.service]
	dipper.SendMessage(s.getDriverRuntime(feature).output, m)
}

func handleRPCReturn(from *DriverRuntime, m *dipper.Message) {
	caller := m.Labels["caller"]
	s := Services[from.service]
	if caller == "-" {
		s.RPC.Caller.HandleReturn(m)
	} else {
		dipper.SendMessage(s.getDriverRuntime(caller).output, m)
	}
}

func handleReload(from *DriverRuntime, m *dipper.Message) {
	daemonID, ok := m.Labels["daemonID"]
	if !ok || daemonID == dipper.GetIP() {
		var min string
		for min = range Services {
			if from.service > min {
				break
			}
		}
		if from.service <= min {
			time.Sleep(time.Second)
			log.Infof("[%s] reload config on broadcast reload message", min)
			Services[min].config.refresh()
		}
	}
}

func (s *Service) counterIncr(name string, tags []string) {
	if emitter, ok := s.driverRuntimes["emitter"]; ok && emitter.state == DriverAlive {
		go s.RPC.Caller.CallNoWait(emitter.output, "emitter", "counter_increment", map[string]interface{}{
			"name": name,
			"tags": tags,
		})
	}
}

func (s *Service) gaugeSet(name string, value string, tags []string) {
	if emitter, ok := s.driverRuntimes["emitter"]; ok && emitter.state == DriverAlive {
		go s.RPC.Caller.CallNoWait(emitter.output, "emitter", "gauge_set", map[string]interface{}{
			"name":  name,
			"value": value,
			"tags":  tags,
		})
	}
}

func (s *Service) metricsLoop() {
	for !shuttingDown {
		if emitter, ok := s.driverRuntimes["emitter"]; ok && emitter.state == DriverAlive {
			counts := map[int]int{
				DriverLoading:   0,
				DriverAlive:     0,
				DriverFailed:    0,
				DriverReloading: 0,
			}
			for _, runtime := range s.driverRuntimes {
				counts[runtime.state]++
			}
			s.gaugeSet("honey.honeydipper.drivers", strconv.Itoa(counts[DriverLoading]), []string{
				"service:" + s.name,
				"state:loading",
			})
			s.gaugeSet("honey.honeydipper.drivers", strconv.Itoa(counts[DriverAlive]), []string{
				"service:" + s.name,
				"state:alive",
			})
			s.gaugeSet("honey.honeydipper.drivers", strconv.Itoa(counts[DriverFailed]), []string{
				"service:" + s.name,
				"state:failed",
			})
		}
		if s.EmitMetrics != nil {
			s.EmitMetrics()
		}
		time.Sleep(time.Minute)
	}
}
