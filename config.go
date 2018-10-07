package main

func (c *Config) bootstrap(wd string) {
	c.wd = wd
	c.loadRepo(c.initRepo)
	c.assemble()
}

func (c *Config) watch() {
}

func (c *Config) assemble() {
	assembled := &(ConfigSet{})
	assembled, _ = c.loaded[c.initRepo].assemble(assembled, map[RepoInfo]bool{})
	c.config = assembled
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
