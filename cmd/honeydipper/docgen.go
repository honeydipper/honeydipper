// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package config defines data structure and logic for loading and
// refreshing configurations for Honeydipper

package main

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// DocItem describe a item or a group of items in the document output.
type DocItem struct {
	ForEach  string `json:"for_each"`
	Template string
	Name     string
	Source   string
	Children []string
}

// DocGenConfig is the schema for config how to generate the documents.
type DocGenConfig struct {
	Repos    []config.RepoInfo
	Items    []DocItem
	Sections []DocGenConfig
}

// IncludePattern is used for find all include statements.
var IncludePattern = regexp.MustCompile(`\{\{\s*include\s+"([\w\.\/-]+)"\s+\}\}`)
var tmplCache = map[string]string{}

func runDocGen(cfg *config.Config) {
	var dgCfg DocGenConfig

	yamlStr, err := ioutil.ReadFile(path.Join(cfg.DocSrc, "docgen.yaml"))
	if err != nil {
		panic(err)
	}

	err = yaml.Unmarshal(yamlStr, &dgCfg)
	if err != nil {
		panic(err)
	}

	envData := map[string]interface{}{
		"repos": dgCfg.Repos,
	}

	for _, item := range dgCfg.Items {
		switch {
		case item.ForEach != "":
			v := reflect.ValueOf(envData[item.ForEach])
			for i := 0; i < v.Len(); i++ {
				if child := v.Index(i); child.IsValid() {
					envData["current"] = child.Interface()
					createItem(item, envData, cfg)
				}
			}
		case item.Template != "":
			createItem(item, envData, cfg)
		case len(item.Children) > 0:
			fetchChildren(item, cfg)
		case item.Source != "":
			fetchItem(item, cfg)
		}
	}
}

func fetchChildren(item DocItem, cfg *config.Config) {
	for _, child := range item.Children {
		parts := strings.Split(child, "=>")
		sourceSuffix := strings.TrimSpace(parts[0])
		nameSuffix := sourceSuffix
		if len(parts) > 1 {
			nameSuffix = strings.TrimSpace(parts[1])
		}
		fetchItem(DocItem{
			Name:   path.Join(item.Name, nameSuffix),
			Source: item.Source + "/" + sourceSuffix,
		}, cfg)
	}
}

func fetchItem(item DocItem, cfg *config.Config) {
	dipper.Logger.Infof("Fetching source %s", item.Source)
	switch {
	case strings.HasPrefix(item.Source, "http://"):
		fallthrough
	case strings.HasPrefix(item.Source, "https://"):
		resp, err := http.Get(item.Source)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= http.StatusMultipleChoices {
			dipper.Logger.Warningf("Received status code %d when fetching file %s", resp.StatusCode, item.Source)
		} else {
			content, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				panic(err)
			}

			file := path.Join(cfg.DocDst, item.Name)
			ensureDirExists(file)
			//nolint:gosec
			err = ioutil.WriteFile(file, content, 0644)
			if err != nil {
				panic(err)
			}
		}
	default:
		in, err := os.Open(item.Source)
		if err != nil {
			panic(err)
		}
		defer in.Close()
		file := path.Join(cfg.DocDst, item.Name)
		ensureDirExists(file)
		out, err := os.Create(file)
		if err != nil {
			panic(err)
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		if err != nil {
			panic(err)
		}
	}
}

func ensureDirExists(file string) {
	dir := filepath.Dir(file)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		panic(err)
	}
}

func readFile(root string, file string) string {
	cwd := path.Dir(file)
	tmpl, err := ioutil.ReadFile(path.Join(root, file))
	if err != nil {
		panic(err)
	}

	includes := IncludePattern.FindAllSubmatchIndex(tmpl, -1)

	sections := make([]string, len(includes)*2+1)
	pos := 0
	for i, match := range includes {
		sections[i*2] = string(tmpl[pos:match[0]])
		filename := path.Clean(path.Join(cwd, string(tmpl[match[2]:match[3]])))
		sections[i*2+1] = readFile(root, filename)
		pos = match[1]
	}
	sections[2*len(includes)] = string(tmpl[pos:])

	return strings.Join(sections, "")
}

func createItem(item DocItem, envData map[string]interface{}, cfg *config.Config) {
	name := dipper.InterpolateStr(item.Name, envData)
	dipper.Logger.Infof("Generating file %s from template %s", name, item.Template)
	tmpl, ok := tmplCache[item.Template]
	if !ok {
		tmpl = readFile(cfg.DocSrc, item.Template)
		tmplCache[item.Template] = tmpl
	}

	if current, ok := envData["current"]; ok {
		if v, ok := current.(config.RepoInfo); ok {
			currentRepo := &config.Config{
				InitRepo:      v,
				IsConfigCheck: false,
			}
			currentRepo.Bootstrap("/tmp")
			envData["current_repo"] = currentRepo.DataSet
		}
	}

	doc := dipper.InterpolateStr(tmpl, envData)

	file := path.Join(cfg.DocDst, name)
	ensureDirExists(file)
	//nolint:gosec
	err := ioutil.WriteFile(file, []byte(doc), 0644)
	if err != nil {
		panic(err)
	}
}
