// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package config defines data structure and logic for loading and
// refreshing configurations for Honeydipper
package config

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/imdario/mergo"
)

const (
	// StageLoading means that the config is being loaded.
	StageLoading = iota
	// StageBooting means that the consumers are loading only essential config.
	StageBooting
	// StageDiscovering means discovering dynamic config data for drivers.
	StageDiscovering
	// StageServing means all config should be processed and serving.
	StageServing
)

var (
	// StageNames maps to the actual stage integers.
	StageNames = []string{
		"Loading",
		"Booting",
		"Discovering",
		"Serving",
	}

	// ErrConfigRollback happens when daemon decides to rollback during reload.
	ErrConfigRollback = errors.New("config rollback")
)

//nolint:gochecknoinits
func init() {
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
	gob.Register(System{})
	gob.Register(DataSet{})
}

// Config is a wrapper around the final complete configration of the daemon.
// including history and the runtime information.
type Config struct {
	InitRepo          RepoInfo
	Services          []string
	DataSet           *DataSet
	Loaded            map[RepoInfo]*Repo
	WorkingDir        string
	LastRunningConfig struct {
		DataSet *DataSet
		Loaded  map[RepoInfo]*Repo
	}
	OnChange      func()
	IsConfigCheck bool
	CheckRemote   bool
	IsDocGen      bool
	DocSrc        string
	DocDst        string
	Locker        *sync.Mutex
	RawData       *DataSet
	Stage         int
	Staged        *DataSet
	StageWG       []*sync.WaitGroup
}

// ResetStage resets the stage of the config.
func (c *Config) ResetStage() {
	locker := &sync.Mutex{}
	locker.Lock()
	defer locker.Unlock()
	if c.Locker != nil {
		c.Locker.Lock()
		defer c.Locker.Unlock()
	}
	c.Locker = locker
	c.Stage = StageLoading

	if c.StageWG != nil {
		for _, wg := range c.StageWG {
			dipper.WaitGroupDoneAll(wg)
		}
	}
	c.StageWG = make([]*sync.WaitGroup, 3)
	c.StageWG[StageLoading] = &sync.WaitGroup{}
	c.StageWG[StageLoading].Add(len(c.Services))
	c.StageWG[StageBooting] = &sync.WaitGroup{}
	c.StageWG[StageBooting].Add(len(c.Services))
	c.StageWG[StageDiscovering] = &sync.WaitGroup{}
	c.StageWG[StageDiscovering].Add(len(c.Services))
}

// Bootstrap loads the configuration during daemon bootstrap.
// WorkingDir is where git will clone remote repo into.
func (c *Config) Bootstrap(wd string) {
	if !c.IsConfigCheck && !c.IsDocGen {
		c.ResetStage()
	}
	c.WorkingDir = wd
	c.loadRepo(c.InitRepo)
	c.assemble()
}

// Watch runs a loop to periodically check remote git repo, reload if changes are detected.
// It requires that the OnChange variable is defined prior to invoking the function.
func (c *Config) Watch() {
	for {
		interval := time.Minute
		if intervalStr, ok := c.GetStagedDriverDataStr("daemon.configCheckInterval"); ok {
			value, err := time.ParseDuration(intervalStr)
			if err != nil {
				dipper.Logger.Warningf("invalid drivers.daemon.configCheckInterval %v", err)
			} else {
				interval = value
			}
		}
		time.Sleep(interval)

		if watch, ok := dipper.GetMapDataBool(c.Staged.Drivers, "daemon.watchConfig"); !ok || watch {
			c.Refresh()
		}
	}
}

// Refresh checks remote git remote, reload if changes are detected.
// It requires that the OnChange variable is defined prior to invoking the function.
func (c *Config) Refresh() {
	changeDetected := false
	for _, repoRuntime := range c.Loaded {
		changeDetected = (repoRuntime.refreshRepo() || changeDetected)
	}
	if changeDetected {
		c.ResetStage()
		c.LastRunningConfig.DataSet = c.RawData
		c.LastRunningConfig.Loaded = map[RepoInfo]*Repo{}
		for k, v := range c.Loaded {
			c.LastRunningConfig.Loaded[k] = v
		}
		dipper.Logger.Debug("reassembling configset")
		defer func() {
			if r := recover(); r != nil {
				dipper.Logger.Warningf("Error loading new config: %v", r)
				dipper.Logger.Warning(errors.Wrap(r, 1).ErrorStack())
				dipper.Logger.Warningf("Rolling back to previous config ...")
				c.RollBack()
			}
		}()
		c.assemble()

		if c.OnChange != nil {
			c.OnChange()
		}
	}
}

// RollBack the configuration to the saved last running configuration.
// It requires that the OnChange variable is defined prior to invoking the function.
func (c *Config) RollBack() {
	if c.LastRunningConfig.DataSet != nil && c.LastRunningConfig.DataSet != c.DataSet {
		c.Staged = c.LastRunningConfig.DataSet
		c.Loaded = map[RepoInfo]*Repo{}
		for k, v := range c.LastRunningConfig.Loaded {
			c.Loaded[k] = v
		}
		dipper.Logger.Warning("config rolled back to last running version")
		c.ResetStage()
		if c.OnChange != nil {
			c.OnChange()
		}
	}
}

func (c *Config) assemble() {
	c.Staged, c.Loaded = c.Loaded[c.InitRepo].assemble(&(DataSet{}), map[RepoInfo]*Repo{})
}

// AdvanceStage processes the config and advances the config into the new stage.
func (c *Config) AdvanceStage(service string, stage int, fns ...dipper.ItemProcessor) {
	c.Locker.Lock()
	defer c.Locker.Unlock()
	locker := c.Locker
	stageWG := c.StageWG[c.Stage]

	switch {
	case c.Stage >= stage:
		return
	case c.Stage < stage-1:
		panic(ErrConfigRollback)
	default:
		if !dipper.WaitGroupDone(stageWG) {
			panic(ErrConfigRollback)
		}
		locker.Unlock()
		dipper.WaitGroupWait(stageWG)
		locker.Lock()

		if locker != c.Locker {
			panic(ErrConfigRollback)
		}

		dipper.Logger.Infof("[%s] service reached stage %s", service, StageNames[stage])
		for _, fn := range fns {
			c.RecursiveStaged(fn)
		}

		switch stage {
		case StageBooting:
			c.RawData = &DataSet{}
			dipper.Must(DeepCopy(c.Staged, c.RawData))
		case StageDiscovering:
			c.extendAllSystems()
		case StageServing:
			c.RecursiveStaged(dipper.RegexParser)

			c.DataSet = c.Staged
		}
		c.Stage = stage
	}
}

// recursive iterates all items and their children to parse the values with give processor.
func (c *Config) recursive(f dipper.ItemProcessor, stage bool) {
	var processor dipper.ItemProcessor
	target := c.DataSet
	if stage {
		target = c.Staged
	}

	processor = func(name string, val interface{}) (interface{}, bool) {
		switch v := val.(type) {
		case string:
			nv, ok := f(name, val)
			if stage {
				return nv, ok
			}
			// only staged data can be changed.
			return nil, false
		case Rule:
			dipper.Recursive(&v.Do, processor)
			dipper.Recursive(&v.When, processor)
		case Workflow:
			dipper.Recursive(v.Match, processor)
			dipper.Recursive(v.UnlessMatch, processor)
			dipper.Recursive(v.Steps, processor)
			dipper.Recursive(v.Threads, processor)
			dipper.Recursive(v.Else, processor)
			dipper.Recursive(v.Cases, processor)
		case Trigger:
			dipper.Recursive(v.Match, processor)
			dipper.Recursive(v.Parameters, processor)
		case Function:
			dipper.Recursive(v.Parameters, processor)
		case System:
			dipper.Recursive(v.Data, processor)
			dipper.Recursive(v.Triggers, processor)
			dipper.Recursive(v.Functions, processor)
		}

		return nil, false
	}

	dipper.Logger.Debugf("[config] recursively processing workflows ...")
	dipper.Recursive(target.Workflows, processor)
	dipper.Logger.Debugf("[config] recursively processing rules ...")
	dipper.Recursive(target.Rules, processor)
	dipper.Logger.Debugf("[config] recursively processing systems ...")
	dipper.Recursive(target.Systems, processor)
	dipper.Logger.Debugf("[config] recursively processing contexts ...")
	dipper.Recursive(target.Contexts, f)
	dipper.Logger.Debugf("[config] recursively processing drivers ...")
	for k, v := range target.Drivers {
		if k != "daemon" {
			dipper.Recursive(v, f)
		}
	}
}

// RecursiveStaged recursively process staged data.
func (c *Config) RecursiveStaged(f dipper.ItemProcessor) {
	c.recursive(f, true)
}

// Recursive recursively process readonly config data.
func (c *Config) Recursive(f dipper.ItemProcessor) {
	c.recursive(f, false)
}

func (c *Config) isRepoLoaded(repo RepoInfo) bool {
	_, ok := c.Loaded[repo]

	return ok
}

func (c *Config) loadRepo(repo RepoInfo) {
	if !c.isRepoLoaded(repo) {
		repoRuntime := newRepo(c, repo)
		repoRuntime.loadRepo()
		if c.Loaded == nil {
			c.Loaded = map[RepoInfo]*Repo{}
		}
		c.Loaded[repo] = repoRuntime
	}
}

// GetStagedDriverData gets an item from a staged driver's data block.
//   conn,ok := c.GetStagedDriverData("redis.connection")
// The function returns an interface{} that could be anything.
func (c *Config) GetStagedDriverData(path string) (ret interface{}, ok bool) {
	if c.Staged == nil || c.Staged.Drivers == nil {
		return nil, false
	}

	return dipper.GetMapData(c.Staged.Drivers, path)
}

// GetStagedDriverDataStr gets an item from a staged driver's data block.
//   logLevel,ok := c.GetStagedDriverData("daemon.loglevel")
// The function assume the return value is a string will do a type assertion.
// upon returning.
func (c *Config) GetStagedDriverDataStr(path string) (ret string, ok bool) {
	if c.Staged == nil || c.Staged.Drivers == nil {
		return "", false
	}

	return dipper.GetMapDataStr(c.Staged.Drivers, path)
}

// GetDriverData gets an item from a driver's data block.
//   conn,ok := c.GetDriverData("redis.connection")
// The function returns an interface{} that could be anything.
func (c *Config) GetDriverData(path string) (ret interface{}, ok bool) {
	if c.DataSet == nil || c.DataSet.Drivers == nil {
		return nil, false
	}

	return dipper.GetMapData(c.DataSet.Drivers, path)
}

// GetDriverDataStr gets an item from a driver's data block.
//   logLevel,ok := c.GetDriverData("daemon.loglevel")
// The function assume the return value is a string will do a type assertion.
// upon returning.
func (c *Config) GetDriverDataStr(path string) (ret string, ok bool) {
	if c.DataSet == nil || c.DataSet.Drivers == nil {
		return "", false
	}

	return dipper.GetMapDataStr(c.DataSet.Drivers, path)
}

func (c *Config) extendSystem(processed map[string]bool, system string) {
	var merged System
	current := c.Staged.Systems[system]
	for _, extend := range current.Extends {
		parts := strings.Split(extend, "=")
		var base, subKey string
		base = strings.TrimSpace(parts[0])
		//nolint:gomnd
		if len(parts) >= 2 {
			subKey = base
			base = strings.TrimSpace(parts[1])
		}

		if _, ok := processed[base]; !ok {
			c.extendSystem(processed, base)
		}

		baseSys := c.Staged.Systems[base]
		baseCopy := &System{}
		dipper.Must(DeepCopy(&baseSys, baseCopy))

		if subKey == "" {
			dipper.Must(mergeSystem(&merged, *baseCopy))
		} else {
			addSubsystem(&merged, *baseCopy, subKey)
		}
	}
	dipper.Must(mergeSystem(&merged, current))
	c.Staged.Systems[system] = merged
	processed[system] = true
}

func (c *Config) extendAllSystems() {
	processed := map[string]bool{}
	for name := range c.Staged.Systems {
		if _, ok := processed[name]; !ok {
			c.extendSystem(processed, name)
		}
	}
}

// DeepCopy performs a deep copy of the object.
func DeepCopy(s interface{}, d interface{}) error {
	if s == nil {
		return nil
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	e := enc.Encode(s)
	if e != nil {
		return fmt.Errorf("failed to encode for deep copy: %w", e)
	}

	dec := gob.NewDecoder(&buf)
	e = dec.Decode(d)
	if e != nil {
		return fmt.Errorf("failed to decode for deep copy: %w", e)
	}

	return nil
}

func mergeDataSet(d *DataSet, s DataSet) error {
	for name := range s.Systems {
		system := s.Systems[name]
		srcCopy := &System{}
		dipper.Must(DeepCopy(&system, srcCopy))
		exist, ok := d.Systems[name]
		if ok {
			dipper.Must(mergeSystem(&exist, *srcCopy))
		} else {
			exist = *srcCopy
		}
		if d.Systems == nil {
			d.Systems = map[string]System{}
		}
		d.Systems[name] = exist
	}
	s.Systems = map[string]System{}
	s.Contexts = dipper.MustDeepCopyMap(s.Contexts)
	s.Drivers = dipper.MustDeepCopyMap(s.Drivers)

	return mergo.Merge(d, s, mergo.WithOverride, mergo.WithAppendSlice)
}

func addSubsystem(d *System, s System, key string) {
	if len(s.Triggers) > 0 && d.Triggers == nil {
		d.Triggers = map[string]Trigger{}
	}
	for name, trigger := range s.Triggers {
		d.Triggers[key+"."+name] = trigger
	}

	if len(s.Functions) > 0 && d.Functions == nil {
		d.Functions = map[string]Function{}
	}
	for name, function := range s.Functions {
		d.Functions[key+"."+name] = function
	}

	if d.Data == nil {
		d.Data = map[string]interface{}{}
	}
	d.Data[key] = s.Data
}

func mergeSystem(d *System, s System) error {
	if len(s.Triggers) > 0 && d.Triggers == nil {
		d.Triggers = map[string]Trigger{}
	}
	for name, trigger := range s.Triggers {
		if exist, ok := d.Triggers[name]; ok {
			err := mergo.Merge(&exist, trigger, mergo.WithOverride, mergo.WithAppendSlice)
			if err != nil {
				return fmt.Errorf("%w", err)
			}
			if exist.Description == "" {
				exist.Description = trigger.Description
			}
			if exist.Meta == nil {
				exist.Meta = trigger.Meta
			}
			d.Triggers[name] = exist
		} else {
			d.Triggers[name] = trigger
		}
	}

	if len(s.Functions) > 0 && d.Functions == nil {
		d.Functions = map[string]Function{}
	}
	for name, function := range s.Functions {
		exist, ok := d.Functions[name]
		if ok {
			err := mergo.Merge(&exist, function, mergo.WithOverride, mergo.WithAppendSlice)
			if err != nil {
				return fmt.Errorf("%w", err)
			}
			if exist.Description == "" {
				exist.Description = function.Description
			}
			if exist.Meta == nil {
				exist.Meta = function.Meta
			}
			d.Functions[name] = exist
		} else {
			d.Functions[name] = function
		}
	}

	err := mergo.Merge(&d.Data, s.Data, mergo.WithOverride, mergo.WithAppendSlice)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	d.Extends = append(d.Extends, s.Extends...)

	if s.Description != "" {
		d.Description = s.Description
	}

	if s.Meta != nil {
		d.Meta = s.Meta
	}

	return nil
}
