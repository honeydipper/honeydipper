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
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/go-errors/errors"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"gopkg.in/src-d/go-git.v4"
	gitCfg "gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// Error represents a configuration error.
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

	dipper.Must(mergeDataSet(assembled, c.DataSet))

	return assembled, assembledList
}

func (c *Repo) isFileLoaded(filename string) bool {
	return c.files[filename]
}

// ReadFile reads a file from the repo.
func (c *Repo) ReadFile(filename string) ([]byte, error) {
	b, e := ioutil.ReadFile(path.Join(c.root, filename[1:]))
	if e != nil {
		return b, fmt.Errorf("read file: %w", e)
	}

	return b, nil
}

func (c *Repo) loadFile(filename string) {
	defer c.recovering(filename, "")

	if c.isFileLoaded(filename) {
		return
	}

	var content DataSet
	yamlFile := dipper.Must(ioutil.ReadFile(path.Join(c.root, filename[1:]))).([]byte)
	dipper.Must(yaml.Unmarshal(yamlFile, &content))

	if content.Repos != nil {
		if !c.parent.IsDocGen && (c.parent.CheckRemote || !c.parent.IsConfigCheck) {
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
	dipper.Must(mergeDataSet(&(c.DataSet), content))
	c.files[filename] = true
	dipper.Logger.Infof("config file [%v] loaded", filename)
}

func newRepo(c *Config, repo RepoInfo) *Repo {
	return &(Repo{c, &repo, DataSet{}, map[string]bool{}, "", []Error{}})
}

func (c *Repo) cloneFetchRepo() {
	dipper.Logger.Infof("cloning repo [%v]", c.repo.Repo)
	var err error
	if c.root, err = ioutil.TempDir(c.parent.WorkingDir, "git"); err != nil {
		dipper.Logger.Errorf("%v", err)
		dipper.Logger.Fatalf("Unable to create subdirectory in %v", c.parent.WorkingDir)
	}

	opts := &git.CloneOptions{URL: c.repo.Repo}
	if strings.HasPrefix(c.repo.Repo, "git@") {
		if auth := GetGitSSHAuth(); auth != nil {
			opts.Auth = auth
		}
	}
	repoObj := dipper.Must(git.PlainClone(c.root, false, opts)).(*git.Repository)

	dipper.Logger.Infof("fetching repo [%v]", c.repo.Repo)
	branch := "master"
	if c.repo.Branch != "" {
		branch = c.repo.Branch
	}
	dipper.Must(repoObj.Fetch(&git.FetchOptions{
		RefSpecs: []gitCfg.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
		Auth:     opts.Auth,
	}))

	dipper.Logger.Infof("using branch [%v] in repo [%v]", branch, c.repo.Repo)
	tree := dipper.Must(repoObj.Worktree()).(*git.Worktree)
	dipper.Must(tree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
	}))
}

func (c *Repo) loadRepo() {
	defer c.recovering("", c.repo.Repo)

	if c.parent.IsConfigCheck && *c.repo == c.parent.InitRepo {
		c.root = dipper.Must(filepath.Abs(c.repo.Repo)).(string)
		dipper.Logger.Infof("using working copy of repo [%v]", c.root)
		// uncomment below to ensure the working copy is a repo
		// if _, err = git.PlainOpen(c.root); err != nil {
		//   panic(err)
		// }
	} else {
		c.cloneFetchRepo()
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

	defer c.recovering("", c.repo.Repo)

	var repoObj *git.Repository
	var err error
	dipper.Logger.Infof("refreshing repo [%v]", c.repo.Repo)
	if repoObj, err = git.PlainOpen(c.root); err != nil {
		panic(err)
	}

	tree := dipper.Must(repoObj.Worktree()).(*git.Worktree)

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
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		dipper.Logger.Infof("no changes skip repo [%s]", c.repo.Repo)

		return false
	} else if err != nil {
		panic(err)
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

func (c *Repo) recovering(filename string, repo string) {
	if r := recover(); r != nil {
		dipper.Logger.Warningf("Resuming after error: %v", r)
		dipper.Logger.Warning(errors.Wrap(r, 1).ErrorStack())
		if filename != "" {
			dipper.Logger.Warningf("config file [%v] skipped", filename)
			c.Errors = append(c.Errors, Error{Error: r.(error), File: filename})
		}
		if repo != "" {
			dipper.Logger.Warningf("repo [%v] skipped", repo)
			c.Errors = append(c.Errors, Error{Error: r.(error), File: "_"})
		}
	}
}
