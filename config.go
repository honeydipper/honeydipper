package main

import (
	"fmt"
	"time"
)

func (c *Config) bootstrap(wd string) {
	c.wd = wd
	c.loadRepo(c.initRepo)
	c.assemble()
}

func (c *Config) watch() {
	for {
		defaultInterval, _ := time.ParseDuration("1m")
		var interval time.Duration
		if intervalStr, ok := c.config.Drivers["daemon"]["configPollInterval"].(string); ok {
			interval, _ = time.ParseDuration(intervalStr)
		}
		if interval == 0 {
			time.Sleep(defaultInterval)
		} else {
			time.Sleep(interval)
		}

		changeDetected := false
		for _, repoRuntime := range c.loaded {
			changeDetected = repoRuntime.refreshRepo() || changeDetected
		}
		if changeDetected {
			c.assemble()
			fmt.Print(c.config)
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
