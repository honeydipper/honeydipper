package main

import (
	"github.com/honeyscience/honeydipper/dipper"
	"log"
	"time"
)

func (c *Config) bootstrap(wd string) {
	c.wd = wd
	c.loadRepo(c.initRepo)
	c.assemble()
}

func (c *Config) watch() {
	for {
		interval := time.Minute
		if intervalStr, ok := c.getDriverDataStr("daemon.configPollInterval"); ok {
			value, err := time.ParseDuration(intervalStr)
			if err != nil {
				log.Printf("invalid drivers.daemon.configPollInterval %v", err)
			}
			interval = value
		}
		time.Sleep(interval)

		changeDetected := false
		for _, repoRuntime := range c.loaded {
			changeDetected = repoRuntime.refreshRepo() || changeDetected
		}
		if changeDetected {
			log.Printf("reassembling configset")
			c.assemble()
		}
	}
}

func (c *Config) assemble() {
	c.config, c.loaded = c.loaded[c.initRepo].assemble(&(ConfigSet{}), map[RepoInfo]*ConfigRepo{})
}

func (c *Config) isRepoLoaded(repo RepoInfo) bool {
	_, ok := c.loaded[repo]
	return ok
}

func (c *Config) loadRepo(repo RepoInfo) {
	if !c.isRepoLoaded(repo) {
		repoRuntime := NewConfigRepo(c, repo)
		repoRuntime.loadRepo()
		if c.loaded == nil {
			c.loaded = map[RepoInfo]*ConfigRepo{}
		}
		c.loaded[repo] = repoRuntime
	}
}

func (c *Config) getDriverData(path string) (ret interface{}, ok bool) {
	return dipper.GetMapData(c.config.Drivers, path)
}

func (c *Config) getDriverDataStr(path string) (ret string, ok bool) {
	return dipper.GetMapDataStr(c.config.Drivers, path)
}
