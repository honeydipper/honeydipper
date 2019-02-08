// Package config defines data structure and logic for loading and
// refreshing configurations for Honeydipper
package config

import (
	"bytes"
	"encoding/gob"
	"time"

	"github.com/go-errors/errors"
	"github.com/honeyscience/honeydipper/pkg/dipper"
	"github.com/imdario/mergo"
)

// Config is a wrapper around the final complete configration of the daemon
// including history and the runtime information
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
	OnChange func()
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
// The function assume the return value is a string will do a type assertion
// upon returning.
func (c *Config) GetDriverDataStr(path string) (ret string, ok bool) {
	if c.DataSet == nil || c.DataSet.Drivers == nil {
		return "", false
	}
	return dipper.GetMapDataStr(c.DataSet.Drivers, path)
}

func (c *Config) extendSystem(processed map[string]bool, system string) {
	var merged System
	var current = c.DataSet.Systems[system]
	for _, parent := range current.Extends {
		if _, ok := processed[parent]; !ok {
			c.extendSystem(processed, parent)
		}

		parentSys := c.DataSet.Systems[parent]
		parentCopy, err := SystemCopy(&parentSys)
		if err != nil {
			panic(err)
		}

		err = mergeSystem(&merged, *parentCopy)
		if err != nil {
			panic(err)
		}
	}
	err := mergeSystem(&merged, current)
	if err != nil {
		panic(err)
	}
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

// SystemCopy performs a deep copy of the given system
func SystemCopy(s *System) (*System, error) {
	var buf bytes.Buffer
	if s == nil {
		return nil, nil
	}
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)
	err := enc.Encode(*s)
	if err != nil {
		return nil, err
	}
	var scopy System
	err = dec.Decode(&scopy)
	if err != nil {
		return nil, err
	}
	return &scopy, nil
}

func mergeDataSet(d *DataSet, s DataSet) error {
	for name, system := range s.Systems {
		exist, ok := d.Systems[name]
		if ok {
			err := mergeSystem(&exist, system)
			if err != nil {
				return err
			}
		} else {
			exist = system
		}
		if d.Systems == nil {
			d.Systems = map[string]System{}
		}
		d.Systems[name] = exist
	}

	s.Systems = map[string]System{}
	err := mergo.Merge(d, s, mergo.WithOverride, mergo.WithAppendSlice)
	return err
}

func mergeSystem(d *System, s System) error {
	for name, trigger := range s.Triggers {
		exist, ok := d.Triggers[name]
		if ok {
			err := mergo.Merge(&exist, trigger, mergo.WithOverride, mergo.WithAppendSlice)
			if err != nil {
				return err
			}
		} else {
			exist = trigger
		}
		if d.Triggers == nil {
			d.Triggers = map[string]Trigger{}
		}
		d.Triggers[name] = exist
	}

	for name, function := range s.Functions {
		exist, ok := d.Functions[name]
		if ok {
			err := mergo.Merge(&exist, function, mergo.WithOverride, mergo.WithAppendSlice)
			if err != nil {
				return err
			}
		} else {
			exist = function
		}
		if d.Functions == nil {
			d.Functions = map[string]Function{}
		}
		d.Functions[name] = exist
	}

	err := mergo.Merge(&d.Data, s.Data, mergo.WithOverride, mergo.WithAppendSlice)
	if err != nil {
		return err
	}

	d.Extends = append(d.Extends, s.Extends...)
	return nil
}
