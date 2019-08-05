// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/go-errors/errors"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"gopkg.in/src-d/go-git.v4"
	gitCfg "gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// Error represents a configuration error
type Error struct {
	Error error
	File  string
}

// Repo contains runtime repo info used to track what has been loaded in a repo.
type Repo struct {
	parent  *Config
	repo    *RepoInfo
	DataSet DataSet
	files   map[string]bool
	root    string
	Errors  []Error
}

func (c *Repo) assemble(assembled *DataSet, assembledList map[RepoInfo]*Repo) (*DataSet, map[RepoInfo]*Repo) {
	assembledList[*c.repo] = c
	for _, repo := range c.DataSet.Repos {
		if _, ok := assembledList[repo]; !ok {
			if repoRuntime, ok := c.parent.Loaded[repo]; ok {
				assembled, assembledList = repoRuntime.assemble(assembled, assembledList)
			}
		}
	}

	dipper.PanicError(mergeDataSet(assembled, c.DataSet))
	return assembled, assembledList
}

func (c *Repo) isFileLoaded(filename string) bool {
	return c.files[filename]
}

func (c *Repo) loadFile(filename string) {
	defer func() {
		if r := recover(); r != nil {
			dipper.Logger.Warningf("Resuming after error: %v", r)
			dipper.Logger.Warning(errors.Wrap(r, 1).ErrorStack())
			dipper.Logger.Warningf("config file [%v] skipped", filename)
			c.Errors = append(c.Errors, Error{Error: r.(error), File: filename})
		}
	}()

	if !c.isFileLoaded(filename) {
		yamlFile, err := ioutil.ReadFile(path.Join(c.root, filename[1:]))
		if err != nil {
			panic(err)
		}
		var content DataSet
		err = yaml.UnmarshalStrict(yamlFile, &content, yaml.DisallowUnknownFields)
		if err != nil {
			panic(err)
		}

		if content.Repos != nil {
			if c.parent.IsConfigCheck && c.parent.CheckRemote || !c.parent.IsConfigCheck {
				for _, referredRepo := range content.Repos {
					if !c.parent.isRepoLoaded(referredRepo) {
						c.parent.loadRepo(referredRepo)
					}
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

		c.normalizeFilePaths(filename, &content)
		dipper.PanicError(mergeDataSet(&(c.DataSet), content))
		c.files[filename] = true
		dipper.Logger.Infof("config file [%v] loaded", filename)
	}
}

func newRepo(c *Config, repo RepoInfo) *Repo {
	return &(Repo{c, &repo, DataSet{}, map[string]bool{}, "", []Error{}})
}

func (c *Repo) loadRepo() {
	defer func() {
		if r := recover(); r != nil {
			dipper.Logger.Warningf("Resuming after error: %v", r)
			dipper.Logger.Warning(errors.Wrap(r, 1).ErrorStack())
			dipper.Logger.Warningf("repo [%v] skipped", c.repo.Repo)
			c.Errors = append(c.Errors, Error{Error: r.(error), File: "_"})
		}
	}()

	var err error
	if c.parent.IsConfigCheck && *c.repo == c.parent.InitRepo {
		c.root = c.repo.Repo
		dipper.Logger.Infof("using working copy of repo [%v]", c.root)
		// uncomment below to ensure the working copy is a repo
		// if _, err = git.PlainOpen(c.root); err != nil {
		//   panic(err)
		// }
	} else {
		dipper.Logger.Infof("cloning repo [%v]", c.repo.Repo)
		var repoObj *git.Repository
		opts := &git.CloneOptions{URL: c.repo.Repo}
		if c.root, err = ioutil.TempDir(c.parent.WorkingDir, "git"); err != nil {
			dipper.Logger.Errorf("%v", err)
			dipper.Logger.Fatalf("Unable to create subdirectory in %v", c.parent.WorkingDir)
		}

		if strings.HasPrefix(c.repo.Repo, "git@") {
			if auth := GetGitSSHAuth(); auth != nil {
				opts.Auth = auth
			}
		}

		repoObj, err = git.PlainClone(c.root, false, opts)
		if err != nil {
			panic(err)
		}

		dipper.Logger.Infof("fetching repo [%v]", c.repo.Repo)
		branch := "master"
		if c.repo.Branch != "" {
			branch = c.repo.Branch
		}
		err = repoObj.Fetch(&git.FetchOptions{
			RefSpecs: []gitCfg.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
			Auth:     opts.Auth,
		})
		if err != nil {
			panic(err)
		}

		dipper.Logger.Infof("using branch [%v] in repo [%v]", branch, c.repo.Repo)
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
	}

	dipper.Logger.Infof("start loading repo [%v]", c.repo.Repo)
	root := "/"
	if c.repo.Path != "" {
		root = c.repo.Path
	}
	c.loadFile(path.Clean(path.Join(root, "init.yaml")))
	dipper.Logger.Infof("repo [%v] loaded", c.repo.Repo)
}

func (c *Repo) refreshRepo() bool {
	c.Errors = []Error{}

	defer func() {
		if r := recover(); r != nil {
			dipper.Logger.Warningf("Resuming after error: %v", r)
			dipper.Logger.Warning(errors.Wrap(r, 1).ErrorStack())
			dipper.Logger.Warningf("repo [%v] skipped", c.repo.Repo)
			c.Errors = append(c.Errors, Error{Error: r.(error), File: "_"})
		}
	}()

	var repoObj *git.Repository
	var err error
	dipper.Logger.Infof("refreshing repo [%v]", c.repo.Repo)
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

		opts := &git.PullOptions{
			RemoteName:    "origin",
			ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
		}
		if strings.HasPrefix(c.repo.Repo, "git@") {
			if auth := GetGitSSHAuth(); auth != nil {
				opts.Auth = auth
			}
		}

		err = tree.Pull(opts)
		if err == git.NoErrAlreadyUpToDate {
			dipper.Logger.Infof("no changes skip repo [%s]", c.repo.Repo)
			return false
		} else if err != nil {
			panic(err)
		}
	}

	c.DataSet = DataSet{}
	c.files = map[string]bool{}
	root := "/"
	if c.repo.Path != "" {
		root = c.repo.Path
	}
	c.loadFile(path.Clean(path.Join(root, "init.yaml")))
	dipper.Logger.Warningf("repo [%v] reloaded", c.repo.Repo)
	return true
}
