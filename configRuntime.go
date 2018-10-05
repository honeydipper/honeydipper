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

func (c *ConfigRuntime) isRepoLoaded(repo RepoInfo) bool {
	_, ok := c.loaded[repo]
	return ok
}

func (c *ConfigRuntime) loadRepo(repo RepoInfo) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Resuming after error: %v\n", r)
			log.Printf("repo [%v] skipped\n", repo.Repo)
		}
	}()

	repoRuntime, ok := c.loaded[repo]
	if !ok {
		repoRuntime = ConfigRepoRuntime{c, ConfigSet{}, make(map[string]bool), ""}
	}

	var repoObj *git.Repository
	var err error
	if repoRuntime.root == "" {
		if c.wd == "" {
			c.wd = "."
		}
		if repoRuntime.root, err = ioutil.TempDir(c.wd, "git"); err != nil {
			log.Printf("%v", err)
			log.Fatalf("Unable to create subdirectory in %v", c.wd)
		}

		repoObj, err = git.PlainClone(repoRuntime.root, false, &git.CloneOptions{
			URL: repo.Repo,
		})

		if err != nil {
			panic(err)
		}
	} else if repoObj, err = git.PlainOpen(repoRuntime.root); err != nil {
		panic(err)
	}

	branch := "master"
	if repo.Branch != "" {
		branch = repo.Branch
	}
	err = repoObj.Fetch(&git.FetchOptions{
		RefSpecs: []gitCfg.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
	})
	if err != nil {
		panic(err)
	}

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

	log.Printf("start loading repo [%v]", repo.Repo)
	root := "/"
	if repo.Path != "" {
		root = repo.Path
	}
	repoRuntime.loadFile(repo, path.Clean(path.Join(root, "setup.yaml")))
	mergo.Merge(&(c.config), repoRuntime.config, mergo.WithOverride)
	if c.loaded == nil {
		c.loaded = make(map[RepoInfo]ConfigRepoRuntime)
	}
	c.loaded[repo] = repoRuntime
	log.Printf("repo [%v] loaded", repo.Repo)
}

func (c *ConfigRepoRuntime) isFileLoaded(filename string) bool {
	return c.files[filename] == true
}

// filename should be absolute path within the repo start with slash
func (c *ConfigRepoRuntime) loadFile(repo RepoInfo, filename string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Resuming after error: %v\n", r)
			log.Printf("config file [%v] skipped\n", filename)
		}
	}()

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
					c.loadFile(repo, absname)
				}
			}
		}

		mergo.Merge(&(c.config), content, mergo.WithOverride)
		c.files[filename] = true
		log.Printf("config file [%v] loaded\n", filename)
	}
}
