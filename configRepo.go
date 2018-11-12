package main

import (
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/imdario/mergo"
	"gopkg.in/src-d/go-git.v4"
	gitCfg "gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"io/ioutil"
	"path"
)

func (c *ConfigRepo) assemble(assembled *ConfigSet, assembledList map[RepoInfo]*ConfigRepo) (*ConfigSet, map[RepoInfo]*ConfigRepo) {
	assembledList[*c.repo] = c
	for _, repo := range c.config.Repos {
		if _, ok := assembledList[repo]; !ok {
			if repoRuntime, ok := c.parent.loaded[repo]; ok {
				assembled, assembledList = repoRuntime.assemble(assembled, assembledList)
			}
		}
	}

	mergo.Merge(assembled, c.config, mergo.WithOverride, mergo.WithAppendSlice)
	return assembled, assembledList
}

func (c *ConfigRepo) isFileLoaded(filename string) bool {
	return c.files[filename] == true
}

func (c *ConfigRepo) loadFile(filename string) {
	defer dipper.SafeExitOnError("config file [%v] skipped", filename)

	if !c.isFileLoaded(filename) {
		yamlFile, err := ioutil.ReadFile(path.Join(c.root, filename[1:]))
		if err != nil {
			panic(err)
		}
		var content ConfigSet
		err = yaml.Unmarshal(yamlFile, &content)
		if err != nil {
			panic(err)
		}

		if content.Repos != nil {
			for _, referredRepo := range content.Repos {
				if !c.parent.isRepoLoaded(referredRepo) {
					c.parent.loadRepo(referredRepo)
				}
			}
		}

		if content.Includes != nil {
			cwd := path.Dir(filename)
			for _, include := range content.Includes {
				absname := path.Clean(path.Join(cwd, include))
				if !c.isFileLoaded(absname) {
					c.loadFile(absname)
				}
			}
		}

		mergo.Merge(&(c.config), content, mergo.WithOverride, mergo.WithAppendSlice)
		c.files[filename] = true
		log.Infof("config file [%v] loaded", filename)
	}
}

// NewConfigRepo : given a Config object, and RepoInfo, create a new ConfigRepo return a pointer
func NewConfigRepo(c *Config, repo RepoInfo) *ConfigRepo {
	return &(ConfigRepo{c, &repo, ConfigSet{}, map[string]bool{}, ""})
}

func (c *ConfigRepo) loadRepo() {
	defer dipper.SafeExitOnError("repo [%v] skipped", c.repo.Repo)

	var repoObj *git.Repository
	var err error
	if c.root == "" {
		log.Infof("cloning repo [%v]", c.repo.Repo)
		if c.root, err = ioutil.TempDir(c.parent.wd, "git"); err != nil {
			log.Errorf("%v", err)
			log.Fatalf("Unable to create subdirectory in %v", c.parent.wd)
		}

		repoObj, err = git.PlainClone(c.root, false, &git.CloneOptions{
			URL: c.repo.Repo,
		})

		if err != nil {
			panic(err)
		}
	} else if repoObj, err = git.PlainOpen(c.root); err != nil {
		panic(err)
	}

	log.Infof("fetching repo [%v]", c.repo.Repo)
	branch := "master"
	if c.repo.Branch != "" {
		branch = c.repo.Branch
	}
	err = repoObj.Fetch(&git.FetchOptions{
		RefSpecs: []gitCfg.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
	})
	if err != nil {
		panic(err)
	}

	log.Infof("using branch [%v] in repo [%v]", branch, c.repo.Repo)
	if tree, err := repoObj.Worktree(); err != nil {
		panic(err)
	} else {
		err = tree.Checkout(&git.CheckoutOptions{
			Branch: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
		})
		if err != nil {
			panic(err)
		}
	}

	log.Infof("start loading repo [%v]", c.repo.Repo)
	root := "/"
	if c.repo.Path != "" {
		root = c.repo.Path
	}
	c.loadFile(path.Clean(path.Join(root, "init.yaml")))
	log.Infof("repo [%v] loaded", c.repo.Repo)
}

func (c *ConfigRepo) refreshRepo() (ret bool) {
	defer dipper.SafeExitOnError("repo [%v] skipped", c.repo.Repo)
	var repoObj *git.Repository
	var err error
	log.Infof("refreshing repo [%v]", c.repo.Repo)
	if repoObj, err = git.PlainOpen(c.root); err != nil {
		panic(err)
	}

	if tree, err := repoObj.Worktree(); err != nil {
		panic(err)
	} else {
		branch := "master"
		if c.repo.Branch != "" {
			branch = c.repo.Branch
		}
		err = tree.Pull(&git.PullOptions{
			RemoteName:    "origin",
			ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
		})
		if err == git.NoErrAlreadyUpToDate {
			log.Infof("no changes skip repo [%s]", c.repo.Repo)
			return false
		} else if err != nil {
			panic(err)
		}
	}

	c.config = ConfigSet{}
	c.files = map[string]bool{}
	root := "/"
	if c.repo.Path != "" {
		root = c.repo.Path
	}
	c.loadFile(path.Clean(path.Join(root, "init.yaml")))
	log.Warningf("repo [%v] reloaded", c.repo.Repo)
	ret = true
	return ret
}
