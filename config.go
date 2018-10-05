package main

import (
	"time"
)

func (c *Config) bootstrap(wd string) {
	current := ConfigRuntime{wd: wd}
	current.loadRepo(c.initRepo)
	c.revs = make(map[time.Time]ConfigRuntime)
	c.revs[time.Now()] = current
}

func (c *Config) watch() {
}
