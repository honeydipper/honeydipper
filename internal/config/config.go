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
	"time"

	"github.com/go-errors/errors"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/imdario/mergo"
)

//nolint:gochecknoinits
func init() {
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
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
}

// Bootstrap loads the configuration during daemon bootstrap.
// WorkingDir is where git will clone remote repo into.
func (c *Config) Bootstrap(wd string) {
	c.WorkingDir = wd
	c.loadRepo(c.InitRepo)
	c.assemble()
}

// Watch runs a loop to periodically check remote git repo, reload if changes are detected.
// It requires that the OnChange variable is defined prior to invoking the function.
func (c *Config) Watch() {
	for {
		interval := time.Minute
		if intervalStr, ok := c.GetDriverDataStr("daemon.configCheckInterval"); ok {
			value, err := time.ParseDuration(intervalStr)
			if err != nil {
				dipper.Logger.Warningf("invalid drivers.daemon.configCheckInterval %v", err)
			} else {
				interval = value
			}
		}
		time.Sleep(interval)

		if watch, ok := dipper.GetMapDataBool(c.DataSet.Drivers, "daemon.watchConfig"); !ok || watch {
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
		c.LastRunningConfig.DataSet = c.DataSet
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
		c.DataSet = c.LastRunningConfig.DataSet
		c.Loaded = map[RepoInfo]*Repo{}
		for k, v := range c.LastRunningConfig.Loaded {
			c.Loaded[k] = v
		}
		dipper.Logger.Warning("config rolled back to last running version")
		if c.OnChange != nil {
			c.OnChange()
		}
	}
}

func (c *Config) assemble() {
	c.DataSet, c.Loaded = c.Loaded[c.InitRepo].assemble(&(DataSet{}), map[RepoInfo]*Repo{})
	c.extendAllSystems()
	c.parseWorkflowRegex()
}

func (c *Config) parseWorkflowRegex() {
	var processor func(key string, val interface{}) (interface{}, bool)

	processor = func(name string, val interface{}) (interface{}, bool) {
		switch v := val.(type) {
		case string:
			return dipper.RegexParser(name, val)
		case Rule:
			dipper.Recursive(&v.Do, processor)
		case Workflow:
			dipper.Recursive(v.Match, processor)
			dipper.Recursive(v.UnlessMatch, processor)
			dipper.Recursive(v.Steps, processor)
			dipper.Recursive(v.Threads, processor)
			dipper.Recursive(v.Else, processor)
			dipper.Recursive(v.Cases, processor)
		}

		return nil, false
	}

	dipper.Recursive(c.DataSet.Workflows, processor)
	dipper.Recursive(c.DataSet.Rules, processor)
	dipper.Recursive(c.DataSet.Contexts, dipper.RegexParser)
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
	current := c.DataSet.Systems[system]
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

		baseSys := c.DataSet.Systems[base]
		baseCopy := dipper.Must(SystemCopy(&baseSys)).(*System)

		if subKey == "" {
			dipper.Must(mergeSystem(&merged, *baseCopy))
		} else {
			addSubsystem(&merged, *baseCopy, subKey)
		}
	}
	dipper.Must(mergeSystem(&merged, current))
	c.DataSet.Systems[system] = merged
	processed[system] = true
}

func (c *Config) extendAllSystems() {
	processed := map[string]bool{}
	for name := range c.DataSet.Systems {
		if _, ok := processed[name]; !ok {
			c.extendSystem(processed, name)
		}
	}
}

// SystemCopy performs a deep copy of the given system.
func SystemCopy(s *System) (*System, error) {
	var buf bytes.Buffer
	if s == nil {
		return nil, nil
	}
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)
	err := enc.Encode(*s)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	var scopy System
	err = dec.Decode(&scopy)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return &scopy, nil
}

func mergeDataSet(d *DataSet, s DataSet) error {
	for name, system := range s.Systems {
		exist, ok := d.Systems[name]
		if ok {
			dipper.Must(mergeSystem(&exist, system))
		} else {
			exist = system
		}
		if d.Systems == nil {
			d.Systems = map[string]System{}
		}
		d.Systems[name] = exist
	}

	s.Systems = map[string]System{}
	s.Contexts = dipper.MustDeepCopyMap(s.Contexts)
	s.Drivers = dipper.MustDeepCopyMap(s.Drivers)
	err := mergo.Merge(d, s, mergo.WithOverride, mergo.WithAppendSlice)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
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
