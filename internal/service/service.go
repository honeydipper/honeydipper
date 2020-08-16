// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package service

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	serrors "github.com/go-errors/errors"
	"github.com/honeydipper/honeydipper/internal/api"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/internal/driver"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// features known to the service for providing some functionalities.
const (
	FeatureEmitter = "emitter"

	// ServiceError indicates error condition when rendering service
	ServiceError dipper.Error = "service error"

	// DriverGracefulTimeout is the timeout in milliseconds for a driver to gracefully shutdown
	DriverGracefulTimeout time.Duration = 50

	// DriverReadyTimeout is the timeout in seconds for the driver to be ready
	DriverReadyTimeout time.Duration = 10

	// DriverRetryBackoff is the interval in seconds before retry loading a driver
	DriverRetryBackoff time.Duration = 30

	// DriverRetryCount is the number of attempts to load a driver
	DriverRetryCount = 3
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
	dipper.RPCCallerBase
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
	EmitMetrics        func()
	APIs               map[string]func(*api.Response)
	CreateAPIResponse  func(dipper.RPCCaller, io.Writer, *dipper.Message) *api.Response
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
	svc.RPCCallerBase.Init(svc, "rpc", "call")

	svc.responders["state:cold"] = []MessageResponder{coldReloadDriverRuntime}
	svc.responders["rpc:call"] = []MessageResponder{handleRPCCall}
	svc.responders["rpc:return"] = []MessageResponder{handleRPCReturn}
	svc.responders["broadcast:reload"] = []MessageResponder{handleReload}
	svc.responders["api:call"] = []MessageResponder{handleAPI}

	svc.CreateAPIResponse = api.ResponseFactory()
	svc.APIs = map[string]func(*api.Response){}

	return svc
}

// GetName returns the name of the service.
func (s *Service) GetName() string {
	return s.name
}

// GetStream returns the writer for communication with the specified feature.
func (s *Service) GetStream(feature string) io.Writer {
	if runtime := s.getDriverRuntime(feature); runtime != nil {
		seconds := 0
		for seconds < 9 && (runtime.State == driver.DriverLoading || runtime.State == driver.DriverReloading) {
			dipper.Logger.Warningf("[%s] waiting for feature %s to load/reload ...", s.name, feature)
			time.Sleep(time.Second)
			seconds++
		}

		if runtime.State != driver.DriverAlive {
			panic(fmt.Errorf("feature failed or loading timeout: %s: %w", feature, ServiceError))
		}

		return runtime.Output
	}
	panic(fmt.Errorf("feature not loaded: %s: %w", feature, ServiceError))
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
			decrypted, _ := s.CallRaw("driver:"+encDriver, "decrypt", decoded)
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
				rerr = fmt.Errorf("%+v: %w", r, ServiceError)
			}
		}
	}()

	oldRuntime := s.getDriverRuntime(feature)
	if oldRuntime == nil {
		dipper.Logger.Warningf("[%s] loading feature %s", s.name, feature)
	} else {
		dipper.Logger.Warningf("[%s] reloading feature %s", s.name, feature)
	}

	var ok bool
	if strings.HasPrefix(feature, "driver:") {
		driverName = feature[7:]
	} else {
		driverName, ok = s.config.GetDriverDataStr(fmt.Sprintf("daemon.featureMap.%s.%s", s.name, feature))
		if !ok {
			driverName, ok = s.config.GetDriverDataStr(fmt.Sprintf("daemon.featureMap.global.%s", feature))
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
			s.hotReload(&driverRuntime, oldRuntime)
		}
	} else {
		// cold reload
		affected = true
		s.coldReload(&driverRuntime, oldRuntime)
	}
	return affected, driverName, nil
}

func (s *Service) hotReload(driverRuntime *driver.Runtime, oldRuntime *driver.Runtime) {
	oldRuntime.Data = driverRuntime.Data
	oldRuntime.DynamicData = driverRuntime.DynamicData
	oldRuntime.State = driver.DriverReloading
	oldRuntime.SendOptions()
}

func (s *Service) coldReload(driverRuntime *driver.Runtime, oldRuntime *driver.Runtime) {
	driverRuntime.Start(s.name)

	s.setDriverRuntime(driverRuntime.Feature, driverRuntime)
	go func(s *Service, runtime *driver.Runtime) {
		defer dipper.SafeExitOnError("[%s] driver runtime %s crash", s.name, runtime.Handler.Meta().Name)
		defer s.checkDeleteDriverRuntime(runtime.Feature, runtime)
		defer close(runtime.Stream)

		//nolint:errcheck
		runtime.Run.Wait()
	}(s, driverRuntime)

	if oldRuntime != nil {
		s.checkDeleteDriverRuntime(driverRuntime.Feature, nil)
		if driverRuntime.Feature == FeatureEmitter {
			// emitter is being replaced
			delete(daemon.Emitters, s.name)
		}
		go func(runtime *driver.Runtime) {
			defer dipper.SafeExitOnError("[%s] runtime %s being replaced output is already closed", s.name, runtime.Handler.Meta().Name)
			// allow 50 millisecond for the data to drain
			time.Sleep(DriverGracefulTimeout * time.Millisecond)
			runtime.Output.Close()
		}(oldRuntime)
	}
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
				time.Sleep(DriverGracefulTimeout * time.Millisecond)
				runtime.Output.Close()
			}(runtime)
		}
	}
}

func (s *Service) loadRequiredFeatures(featureList map[string]bool, boot bool) {
	for feature, required := range featureList {
		if !required {
			continue
		}
		affected, driverName, err := s.loadFeature(feature)
		if err != nil {
			if boot {
				dipper.Logger.Fatalf("[%s] failed to load required feature [%s]", s.name, feature)
			} else {
				dipper.Logger.Panicf("[%s] failed to reload required feature [%s]", s.name, feature)
			}
		}
		if !affected {
			continue
		}

		// expecting the driver to ping back then send options
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
				DriverReadyTimeout*time.Second,
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
						DriverReadyTimeout*time.Second,
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

		switch {
		case ok && chosen < len(orderedRuntimes):
			// selected driver gives message

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

		case !ok && chosen < len(orderedRuntimes):
			// selected driver crashed

			if orderedRuntimes[chosen].Feature == FeatureEmitter {
				// emitter has crashed
				delete(daemon.Emitters, s.name)
			}
			if d := orderedRuntimes[chosen]; d.State == driver.DriverAlive {
				// only reload drivers that used to be in DriveAlive state
				go loadFailedDriverRuntime(orderedRuntimes[chosen], 0)
			}
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
					for i := range expects {
						if &expects[i] == &processor {
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

func coldReloadDriverRuntime(d *driver.Runtime, m *dipper.Message) {
	s := Services[d.Service]
	s.checkDeleteDriverRuntime(d.Feature, d)
	d.Output.Close()
	dipper.Must(s.loadFeature(d.Feature))
}

func loadFailedDriverRuntime(d *driver.Runtime, count int) {
	s := Services[d.Service]
	d.State = driver.DriverFailed
	driverName := d.Handler.Meta().Name
	if emitter, ok := daemon.Emitters[s.name]; ok {
		emitter.CounterIncr("honey.honeydipper.driver.recovery_attempt", []string{
			"service:" + s.name,
			"driver:" + driverName,
		})
	}

	retry := func() {
		dipper.Logger.Warningf("[%s] failed to load/reload driver %s attempt %d", s.name, driverName, count)
		if count < DriverRetryCount {
			time.Sleep(DriverRetryBackoff * time.Second)
			go loadFailedDriverRuntime(d, count+1)
		} else {
			dipper.Logger.Fatalf("[%s] quiting after failed to reload crashed driver %s", s.name, driverName)
		}
	}
	_, _, err := s.loadFeature(d.Feature)
	if err != nil {
		retry()
	} else {
		s.addExpect(
			"state:alive:"+driverName,
			func(*dipper.Message) {
				s.driverRuntimes[d.Feature].State = driver.DriverAlive
				if d.Feature == FeatureEmitter {
					// emitter is loaded
					daemon.Emitters[s.name] = s
				}
			},
			DriverReadyTimeout*time.Second,
			retry,
		)
	}
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
		s.HandleReturn(m)
	} else {
		dipper.SendMessage(s.getDriverRuntime(caller).Output, m)
	}
}

func handleAPI(from *driver.Runtime, m *dipper.Message) {
	s := Services[from.Service]
	dipper.DeserializePayload(m)
	resp := s.CreateAPIResponse(s, s.getDriverRuntime("eventbus").Output, m)
	if resp == nil {
		dipper.Logger.Debugf("[%s] skipping handling API: %+v", s.name, m.Labels)
		return
	}
	method := m.Labels["fn"]
	dipper.Logger.Debugf("[%s] handling API [%s]: %+v", s.name, method, m.Labels)
	if apiFunc, ok := s.APIs[method]; ok {
		go func() {
			defer dipper.SafeExitOnError("[%s] api call panic for [%s]", s.name, method)
			apiFunc(resp)
		}()
	}
}

func handleReload(from *driver.Runtime, m *dipper.Message) {
	daemonID, ok := m.Labels["daemonID"]
	if ok && daemonID != dipper.GetIP() {
		return
	}
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

// CounterIncr increases a counter metric.
func (s *Service) CounterIncr(name string, tags []string) {
	go func() {
		_ = s.CallNoWait(FeatureEmitter, "counter_increment", map[string]interface{}{
			"name": name,
			"tags": tags,
		})
	}()
}

// GaugeSet sets the value for a gauge metric.
func (s *Service) GaugeSet(name string, value string, tags []string) {
	go func() {
		_ = s.CallNoWait(FeatureEmitter, "gauge_set", map[string]interface{}{
			"name":  name,
			"value": value,
			"tags":  tags,
		})
	}()
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
