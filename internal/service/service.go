// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	serrors "github.com/go-errors/errors"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/internal/driver"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// features known to the service for providing some functionalities
const (
	FeatureEmitter = "emitter"
)

// MessageResponder is a function type that respond to messages.
type MessageResponder func(*driver.Runtime, *dipper.Message)

// ExpectHandler is a function type that handles expected message.
type ExpectHandler func(*dipper.Message)

// RoutedMessage stores a dipper message and its destination.
type RoutedMessage struct {
	driverRuntime *driver.Runtime
	message       *dipper.Message
}

// Service is a collection of daemon's feature.
type Service struct {
	name               string
	config             *config.Config
	driverRuntimes     map[string]*driver.Runtime
	expects            map[string][]ExpectHandler
	responders         map[string][]MessageResponder
	transformers       map[string][]func(*driver.Runtime, *dipper.Message) *dipper.Message
	dynamicFeatureData map[string]interface{}
	expectLock         sync.Mutex
	driverLock         sync.Mutex
	selectLock         sync.Mutex
	Route              func(*dipper.Message) []RoutedMessage
	DiscoverFeatures   func(*config.DataSet) map[string]interface{}
	ServiceReload      func(*config.Config)
	RPC                struct {
		Caller dipper.RPCCaller
	}
	EmitMetrics func()
}

// Services holds a catalog of running services in this daemon process.
var Services = map[string]*Service{}

// NewService creates a service with given config and name.
func NewService(cfg *config.Config, name string) *Service {
	svc := &Service{
		name:           name,
		config:         cfg,
		driverRuntimes: map[string]*driver.Runtime{},
		expects:        map[string][]ExpectHandler{},
		responders:     map[string][]MessageResponder{},
	}
	svc.RPC.Caller.Init("rpc", "call")

	svc.responders["state:cold"] = []MessageResponder{coldReloadDriverRuntime}
	svc.responders["rpc:call"] = []MessageResponder{handleRPCCall}
	svc.responders["rpc:return"] = []MessageResponder{handleRPCReturn}
	svc.responders["broadcast:reload"] = []MessageResponder{handleReload}

	return svc
}

func (s *Service) decryptDriverData(key string, val interface{}) (ret interface{}, replace bool) {
	dipper.Logger.Debugf("[%s] decrypting %s", s.name, key)
	if str, ok := val.(string); ok {
		if strings.HasPrefix(str, "ENC[") {
			parts := strings.SplitN(str[4:len(str)-1], ",", 2)
			encDriver := parts[0]
			data := []byte(parts[1])
			decoded, err := base64.StdEncoding.DecodeString(string(data))
			if err != nil {
				dipper.Logger.Panicf("encrypted data shoud be base64 encoded")
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
			dipper.Logger.Warningf("Resuming after error: %v", r)
			dipper.Logger.Warningf(serrors.Wrap(r, 1).ErrorStack())
			dipper.Logger.Warningf("[%s] skip reloading feature: %s", s.name, feature)
			if runtime := s.getDriverRuntime(feature); runtime != nil {
				runtime.State = driver.DriverFailed
			}
			if err, ok := r.(error); ok {
				rerr = err
			} else {
				rerr = errors.New(fmt.Sprint(r))
			}
		}
	}()

	oldRuntime := s.getDriverRuntime(feature)
	if oldRuntime == nil {
		dipper.Logger.Warningf("[%s] loading feature %s", s.name, feature)
	} else {
		dipper.Logger.Warningf("[%s] reloading feature %s", s.name, feature)
	}

	if strings.HasPrefix(feature, "driver:") {
		driverName = feature[7:]
	} else {
		featureMap, ok := s.config.GetDriverData("daemon.featureMap")
		if ok {
			driverName, ok = dipper.GetMapDataStr(featureMap, s.name+"."+feature)
			if !ok {
				driverName, ok = dipper.GetMapDataStr(featureMap, "global."+feature)
			}
		}
		if !ok {
			panic("driver not defined for the feature")
		}
	}
	dipper.Logger.Infof("[%s] mapping feature %s to driver %s", s.name, feature, driverName)

	driverData, _ := s.config.GetDriverData(driverName)
	var dynamicData interface{}
	if strings.HasPrefix(feature, "driver:") {
		dynamicData = s.dynamicFeatureData[feature]
	}

	var driverHandler driver.Handler
	if driverMeta, ok := s.config.GetDriverData(fmt.Sprintf("daemon.drivers.%s", driverName)); ok {
		driverHandler = driver.NewDriver(driverMeta.(map[string]interface{}))
	} else {
		panic("unable to get driver metadata")
	}

	dipper.Recursive(driverData, s.decryptDriverData)
	dipper.Recursive(dynamicData, s.decryptDriverData)

	dipper.Logger.Debugf("[%s] driver %s meta %v", s.name, driverName, driverHandler.Meta())
	driverRuntime := driver.Runtime{
		Feature:     feature,
		Data:        driverData,
		DynamicData: dynamicData,
		State:       driver.DriverLoading,
		Handler:     driverHandler,
	}

	if oldRuntime != nil && oldRuntime.State != driver.DriverFailed && reflect.DeepEqual(*oldRuntime.Handler.Meta(), *driverHandler.Meta()) {
		if reflect.DeepEqual(oldRuntime.Data, driverRuntime.Data) && reflect.DeepEqual(oldRuntime.DynamicData, driverRuntime.DynamicData) {
			dipper.Logger.Infof("[%s] driver not affected: %s", s.name, driverName)
		} else {
			// hot reload
			affected = true
			oldRuntime.Data = driverRuntime.Data
			oldRuntime.DynamicData = driverRuntime.DynamicData
			oldRuntime.State = driver.DriverReloading
			oldRuntime.SendOptions()
		}
	} else {
		// cold reload
		affected = true
		driverRuntime.Start(s.name)

		s.setDriverRuntime(feature, &driverRuntime)
		go func(s *Service, feature string, runtime *driver.Runtime) {
			defer dipper.SafeExitOnError("[%s] driver runtime %s crash", s.name, runtime.Handler.Meta().Name)
			defer s.checkDeleteDriverRuntime(feature, runtime)
			defer func() {
				s.selectLock.Lock()
				defer s.selectLock.Unlock()
				close(runtime.Stream)
			}()

			//nolint:errcheck
			runtime.Run.Wait()
		}(s, feature, &driverRuntime)

		if oldRuntime != nil {
			s.checkDeleteDriverRuntime(feature, nil)
			if feature == FeatureEmitter {
				// emitter is being replaced
				delete(daemon.Emitters, s.name)
			}
			go func(runtime *driver.Runtime) {
				defer dipper.SafeExitOnError("[%s] runtime %s being replaced output is already closed", s.name, runtime.Handler.Meta().Name)
				// allow 50 millisecond for the data to drain
				time.Sleep(50 * time.Millisecond)
				runtime.Output.Close()
			}(oldRuntime)
		}
	}
	return affected, driverName, nil
}

func (s *Service) start() {
	go func() {
		dipper.Logger.Infof("[%s] starting service", s.name)
		featureList := s.getFeatureList()
		s.loadRequiredFeatures(featureList, true)
		go s.serviceLoop()
		time.Sleep(time.Second)
		s.loadAdditionalFeatures(featureList)
		go s.metricsLoop()
	}()
}

// Reload the service when configuration changes are detected.
func (s *Service) Reload() {
	dipper.Logger.Infof("[%s] reloading service", s.name)
	var featureList map[string]bool

	func() {
		defer func() {
			if r := recover(); r != nil {
				dipper.Logger.Errorf("[%s] reverting config due to fatal failure %v", s.name, r)
				s.config.RollBack()
			}
		}()
		if s.ServiceReload != nil {
			s.ServiceReload(s.config)
		}
		featureList = s.getFeatureList()
		s.loadRequiredFeatures(featureList, false)
	}()

	s.loadAdditionalFeatures(featureList)
	s.removeUnusedFeatures(featureList)
}

func (s *Service) getFeatureList() map[string]bool {
	featureList := map[string]bool{}
	if cfgItem, ok := s.config.GetDriverData("daemon.features.global"); ok {
		for _, feature := range cfgItem.([]interface{}) {
			featureName := feature.(map[string]interface{})["name"].(string)
			featureList[featureName], _ = dipper.GetMapDataBool(feature, "required")
		}
	}
	if cfgItem, ok := s.config.GetDriverData("daemon.features." + s.name); ok {
		for _, feature := range cfgItem.([]interface{}) {
			featureName := feature.(map[string]interface{})["name"].(string)
			featureList[featureName], _ = dipper.GetMapDataBool(feature, "required")
		}
	}
	if s.DiscoverFeatures != nil {
		s.dynamicFeatureData = s.DiscoverFeatures(s.config.DataSet)
		for name := range s.dynamicFeatureData {
			featureList[name] = false
		}
	}
	dipper.Logger.Debugf("[%s] final feature list %+v", s.name, featureList)
	return featureList
}

func (s *Service) removeUnusedFeatures(featureList map[string]bool) {
	for feature, runtime := range s.driverRuntimes {
		if _, ok := featureList[feature]; !ok {
			if feature == FeatureEmitter {
				// emitter is removed
				delete(daemon.Emitters, s.name)
			}
			s.checkDeleteDriverRuntime(feature, nil)
			go func(runtime *driver.Runtime) {
				defer dipper.SafeExitOnError("[%s] unused runtime %s output is already closed", s.name, runtime.Handler.Meta().Name)
				// allow 50 millisecond for the data to drain
				time.Sleep(50 * time.Millisecond)
				runtime.Output.Close()
			}(runtime)
		}
	}
}

func (s *Service) loadRequiredFeatures(featureList map[string]bool, boot bool) {
	for feature, required := range featureList {
		if required {
			affected, driverName, err := s.loadFeature(feature)
			if err != nil {
				if boot {
					dipper.Logger.Fatalf("[%s] failed to load required feature [%s]", s.name, feature)
				} else {
					dipper.Logger.Panicf("[%s] failed to reload required feature [%s]", s.name, feature)
				}
			}
			if affected {
				func(feature string, driverName string) {
					s.addExpect(
						"state:alive:"+driverName,
						func(*dipper.Message) {
							s.driverRuntimes[feature].State = driver.DriverAlive
							if feature == FeatureEmitter {
								// emitter is loaded
								daemon.Emitters[s.name] = s
							}
						},
						10*time.Second,
						func() {
							if boot {
								dipper.Logger.Fatalf("failed to start driver %s.%s", s.name, driverName)
							} else {
								dipper.Logger.Warningf("failed to reload driver %s.%s", s.name, driverName)
								s.driverRuntimes[feature].State = driver.DriverFailed
								s.config.RollBack()
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
				dipper.Logger.Warningf("[%s] skip feature %s error %v", s.name, feature, err)
			}
			if affected {
				func(feature string, driverName string) {
					s.addExpect(
						"state:alive:"+driverName,
						func(*dipper.Message) {
							s.driverRuntimes[feature].State = driver.DriverAlive
							if feature == FeatureEmitter {
								// emitter is loaded
								daemon.Emitters[s.name] = s
							}
						},
						10*time.Second,
						func() {
							dipper.Logger.Warningf("[%s] failed to start or reload driver %s", s.name, driverName)
							s.driverRuntimes[feature].State = driver.DriverFailed
						},
					)
				}(feature, driverName)
			}
		}
	}
}

func (s *Service) serviceLoop() {
	daemon.Children.Add(1)
	defer daemon.Children.Done()
	for !daemon.ShuttingDown {
		var cases []reflect.SelectCase
		var orderedRuntimes []*driver.Runtime
		func() {
			s.driverLock.Lock()
			defer s.driverLock.Unlock()
			cases = []reflect.SelectCase{}
			orderedRuntimes = []*driver.Runtime{}
			for _, runtime := range s.driverRuntimes {
				if runtime.State != driver.DriverFailed {
					cases = append(cases, reflect.SelectCase{
						Dir:  reflect.SelectRecv,
						Chan: reflect.ValueOf(runtime.Stream),
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
				orderedRuntimes[chosen].State = driver.DriverFailed
				if orderedRuntimes[chosen].Feature == FeatureEmitter {
					// emitter has crashed
					delete(daemon.Emitters, s.name)
				}
			}
		} else if chosen < len(orderedRuntimes) {
			func() {
				defer dipper.SafeExitOnError("[%s] service loop continue", s.name)
				runtime := orderedRuntimes[chosen]
				msg := value.Interface().(dipper.Message)
				if runtime.Feature != FeatureEmitter {
					if emitter, ok := daemon.Emitters[s.name]; ok {
						emitter.CounterIncr("honey.honeydipper.local.message", []string{
							"service:" + s.name,
							"driver:" + runtime.Handler.Meta().Name,
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
	for fname, runtime := range s.driverRuntimes {
		func() {
			defer dipper.SafeExitOnError("[%s] driver runtime for feature %s already closed", s.name, fname)
			runtime.Output.Close()
		}()
	}
	dipper.Logger.Warningf("[%s] service closed for business", s.name)
}

func (s *Service) process(msg dipper.Message, runtime *driver.Runtime) {
	defer dipper.SafeExitOnError("[%s] continue  message loop", s.name)
	expectKey := fmt.Sprintf("%s:%s:%s", msg.Channel, msg.Subject, runtime.Handler.Meta().Name)
	if expects, ok := s.deleteExpect(expectKey); ok {
		for _, f := range expects {
			go func(f ExpectHandler) {
				defer dipper.SafeExitOnError("[%s] continue  message loop", s.name)
				f(&msg)
			}(f)
		}
	}

	key := fmt.Sprintf("%s:%s", msg.Channel, msg.Subject)
	// responder
	if responders, ok := s.responders[key]; ok {
		for _, f := range responders {
			go func(f MessageResponder) {
				defer dipper.SafeExitOnError("[%s] continue  message loop", s.name)
				f(runtime, &msg)
			}(f)
		}
	}

	go func(msg *dipper.Message) {
		defer dipper.SafeExitOnError("[%s] continue  message loop", s.name)

		// transformer
		if transformers, ok := s.transformers[key]; ok {
			for _, f := range transformers {
				msg = f(runtime, msg)
				if msg == nil {
					break
				}
			}
		}

		if msg != nil && s.Route != nil {
			// router
			routedMsgs := s.Route(msg)

			if len(routedMsgs) > 0 {
				for _, routedMsg := range routedMsgs {
					routedMsg.driverRuntime.SendMessage(routedMsg.message)
				}
			}
		}
	}(&msg)
}

func (s *Service) addResponder(channelSubject string, f MessageResponder) {
	s.responders[channelSubject] = append(s.responders[channelSubject], f)
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
						//nolint:scopelint
						if &p == &processor {
							expects = append(expects[:i], expects[i+1:]...)
							break
						}
					}
				} else {
					delete(s.expects, expectKey)
				}
			}()
			defer dipper.SafeExitOnError("[%s] panic in except handler for %s", s.name, expectKey)
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

func (s *Service) getDriverRuntime(feature string) *driver.Runtime {
	runtime, ok := dipper.LockGetMap(&s.driverLock, s.driverRuntimes, feature)
	if ok && runtime != nil {
		return runtime.(*driver.Runtime)
	}
	return nil
}

func (s *Service) setDriverRuntime(feature string, runtime *driver.Runtime) *driver.Runtime {
	oldone := dipper.LockSetMap(&s.driverLock, s.driverRuntimes, feature, runtime)
	if oldone != nil {
		return oldone.(*driver.Runtime)
	}
	return nil
}

func (s *Service) checkDeleteDriverRuntime(feature string, check *driver.Runtime) {
	dipper.LockCheckDeleteMap(&s.driverLock, s.driverRuntimes, feature, check)
}

// RPCCallRaw is used for making a PRC call with raw bytes from driver to another driver.
func (s *Service) RPCCallRaw(feature string, method string, params []byte) ([]byte, error) {
	return s.RPC.Caller.CallRaw(s.getDriverRuntime(feature).Output, feature, method, params)
}

// RPCCall is used for making a PRC call from driver to another driver.
func (s *Service) RPCCall(feature string, method string, params interface{}) ([]byte, error) {
	return s.RPC.Caller.Call(s.getDriverRuntime(feature).Output, feature, method, params)
}

func coldReloadDriverRuntime(d *driver.Runtime, m *dipper.Message) {
	s := Services[d.Service]
	s.checkDeleteDriverRuntime(d.Feature, d)
	d.Output.Close()
	dipper.PanicError(s.loadFeature(d.Feature))
}

func handleRPCCall(from *driver.Runtime, m *dipper.Message) {
	feature := m.Labels["feature"]
	m.Labels["caller"] = from.Feature
	s := Services[from.Service]
	dipper.SendMessage(s.getDriverRuntime(feature).Output, m)
}

func handleRPCReturn(from *driver.Runtime, m *dipper.Message) {
	caller := m.Labels["caller"]
	s := Services[from.Service]
	if caller == "-" {
		s.RPC.Caller.HandleReturn(m)
	} else {
		dipper.SendMessage(s.getDriverRuntime(caller).Output, m)
	}
}

func handleReload(from *driver.Runtime, m *dipper.Message) {
	daemonID, ok := m.Labels["daemonID"]
	if !ok || daemonID == dipper.GetIP() {
		var min string
		for min = range Services {
			if from.Service > min {
				break
			}
		}
		if from.Service <= min {
			m := dipper.DeserializePayload(m)
			go func() {
				time.Sleep(time.Second)
				if force, ok := dipper.GetMapDataStr(m.Payload, "force"); ok && (force == "yes" || force == "true") {
					daemon.ShutDown()
					os.Exit(0)
				} else {
					dipper.Logger.Infof("[%s] reload config on broadcast reload message", min)
					Services[min].config.Refresh()
				}
			}()
		}
	}
}

// CounterIncr increases a counter metric.
func (s *Service) CounterIncr(name string, tags []string) {
	if emitter, ok := s.driverRuntimes[FeatureEmitter]; ok && emitter.State == driver.DriverAlive {
		go func() {
			defer dipper.SafeExitOnError("[%s] emitter crashed")
			s.RPC.Caller.CallNoWait(emitter.Output, FeatureEmitter, "counter_increment", map[string]interface{}{
				"name": name,
				"tags": tags,
			})
		}()
	}
}

// GaugeSet sets the value for a gauge metric.
func (s *Service) GaugeSet(name string, value string, tags []string) {
	if emitter, ok := s.driverRuntimes[FeatureEmitter]; ok && emitter.State == driver.DriverAlive {
		go func() {
			defer dipper.SafeExitOnError("[%s] emitter crashed")
			s.RPC.Caller.CallNoWait(emitter.Output, FeatureEmitter, "gauge_set", map[string]interface{}{
				"name":  name,
				"value": value,
				"tags":  tags,
			})
		}()
	}
}

func (s *Service) metricsLoop() {
	for !daemon.ShuttingDown {
		func() {
			defer dipper.SafeExitOnError("[%s] metrics loop crashing")
			if emitter, ok := s.driverRuntimes[FeatureEmitter]; ok && emitter.State == driver.DriverAlive {
				counts := map[int]int{
					driver.DriverLoading:   0,
					driver.DriverAlive:     0,
					driver.DriverFailed:    0,
					driver.DriverReloading: 0,
				}
				for _, runtime := range s.driverRuntimes {
					counts[runtime.State]++
				}
				s.GaugeSet("honey.honeydipper.drivers", strconv.Itoa(counts[driver.DriverLoading]), []string{
					"service:" + s.name,
					"state:loading",
				})
				s.GaugeSet("honey.honeydipper.drivers", strconv.Itoa(counts[driver.DriverAlive]), []string{
					"service:" + s.name,
					"state:alive",
				})
				s.GaugeSet("honey.honeydipper.drivers", strconv.Itoa(counts[driver.DriverFailed]), []string{
					"service:" + s.name,
					"state:failed",
				})
			}
			if s.EmitMetrics != nil {
				s.EmitMetrics()
			}
		}()
		time.Sleep(time.Minute)
	}
}
