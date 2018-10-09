package main

import (
	"fmt"
	"github.com/imdario/mergo"
	"gopkg.in/src-d/go-git.v4"
	gitCfg "gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"path"
)

func (c *ConfigRepo) assemble(assembled *ConfigSet, assembledList map[RepoInfo]*ConfigRepo) (*ConfigSet, map[RepoInfo]*ConfigRepo) {
	for _, repo := range c.config.Repos {
		if _, ok := assembledList[repo]; !ok {
			if repoRuntime, ok := c.parent.loaded[repo]; ok {
				assembled, assembledList = repoRuntime.assemble(assembled, assembledList)
			}
		}
	}

	mergo.Merge(assembled, c.config, mergo.WithOverride)
	assembledList[*c.repo] = c
	return assembled, assembledList
}

func (c *ConfigRepo) isFileLoaded(filename string) bool {
	return c.files[filename] == true
}

func (c *ConfigRepo) loadFile(filename string) {
	defer safeExitOnError("config file [%v] skipped\n", filename)

	if !c.isFileLoaded(filename) {
		yamlFile, err := ioutil.ReadFile(path.Join(c.root, filename[1:]))
		if err != nil {
			panic(err)
		}
		var content ConfigSet
		err = yaml.UnmarshalStrict(yamlFile, &content)
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

		mergo.Merge(&(c.config), content, mergo.WithOverride)
		c.files[filename] = true
		log.Printf("config file [%v] loaded\n", filename)
	}
}

// NewConfigRepo : given a Config object, and RepoInfo, create a new ConfigRepo return a pointer
func NewConfigRepo(c *Config, repo RepoInfo) *ConfigRepo {
	return &(ConfigRepo{c, &repo, ConfigSet{}, map[string]bool{}, ""})
}

func (c *ConfigRepo) loadRepo() {
	defer safeExitOnError("repo [%v] skipped\n", c.repo.Repo)

	var repoObj *git.Repository
	var err error
	if c.root == "" {
		log.Printf("cloning repo [%v]", c.repo.Repo)
		if c.root, err = ioutil.TempDir(c.parent.wd, "git"); err != nil {
			log.Printf("%v", err)
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

	log.Printf("fetching repo [%v]", c.repo.Repo)
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

	log.Printf("using branch [%v] in repo [%v]", branch, c.repo.Repo)
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

	log.Printf("start loading repo [%v]", c.repo.Repo)
	root := "/"
	if c.repo.Path != "" {
		root = c.repo.Path
	}
	c.loadFile(path.Clean(path.Join(root, "init.yaml")))
	log.Printf("repo [%v] loaded", c.repo.Repo)
}

func (c *ConfigRepo) refreshRepo() (ret bool) {
	defer func() { safeExitOnError("repo [%v] skipped\n", c.repo.Repo); ret = false }()
	var repoObj *git.Repository
	var err error
	log.Printf("refreshing repo [%v]", c.repo.Repo)
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
			log.Printf("no changes, skip repo [%s]", c.repo.Repo)
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
	log.Printf("repo [%v] reloaded", c.repo.Repo)
	return true
}
