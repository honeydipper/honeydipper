package main

import (
	"github.com/honeyscience/honeydipper/dipper"
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
				log.Warningf("invalid drivers.daemon.configPollInterval %v", err)
			}
			interval = value
		}
		time.Sleep(interval)

		changeDetected := false
		for _, repoRuntime := range c.loaded {
			changeDetected = (repoRuntime.refreshRepo() || changeDetected)
		}
		if changeDetected {
			c.lastRunningConfig.config = c.config
			c.lastRunningConfig.loaded = map[RepoInfo]*ConfigRepo{}
			for k, v := range c.loaded {
				c.lastRunningConfig.loaded[k] = v
			}
			log.Infof("reassembling configset")
			c.assemble()

			for _, service := range Services {
				go service.reload()
			}
		}
	}
}

func (c *Config) rollBack() {
	if c.lastRunningConfig.config != nil && c.lastRunningConfig.config != c.config {
		c.config = c.lastRunningConfig.config
		c.loaded = map[RepoInfo]*ConfigRepo{}
		for k, v := range c.lastRunningConfig.loaded {
			c.loaded[k] = v
		}
		log.Infof("config rolled back to last running version")
		for _, service := range Services {
			service.reload()
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
	if c.config == nil || c.config.Drivers == nil {
		return nil, false
	}
	return dipper.GetMapData(c.config.Drivers, path)
}

func (c *Config) getDriverDataStr(path string) (ret string, ok bool) {
	if c.config == nil || c.config.Drivers == nil {
		return "", false
	}
	return dipper.GetMapDataStr(c.config.Drivers, path)
}
