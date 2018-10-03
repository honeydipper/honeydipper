package main

import (
	"time"
)

// System : is an abstract construct to hold data, event and action definitions
type System struct {
	data    (interface{})
	events  [](interface{})
	actions [](interface{})
}

// Rule : is a data structure defining what action to take upon an event
type Rule interface{}

// DriverData : holds the data necessary for a driver to operate
type DriverData interface{}

// RepoInfo : points a git repo where config data can be read from
type RepoInfo struct {
	repo   string
	branch string
	path   string
}

// ConfigRev : is a complete set of configuration at a specific moment
type ConfigRev struct {
	systems []System
	rules   []Rule
	drivers []DriverData
}

// Config : is the complete configration of the daemon including history and the running services
type Config struct {
	initRepo RepoInfo
	services []string
	revs     map[time.Time]ConfigRev
}

func (c *Config) bootstrap() {
}

func (c *Config) watch() {
}
