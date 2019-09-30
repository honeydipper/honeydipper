// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package config defines data structure and logic for loading and
// refreshing configurations for Honeydipper

package main

import (
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// DocItem describe a item or a group of items in the document output
type DocItem struct {
	ForEach  string `json:"for_each"`
	Template string
	Name     string
	Source   string
	Children []string
}

// DocGenConfig is the schema for config how to generate the documents
type DocGenConfig struct {
	Repos    []config.RepoInfo
	Items    []DocItem
	Sections []DocGenConfig
}

func runDocGen(cfg *config.Config) {
	var dgCfg DocGenConfig

	yamlStr, err := ioutil.ReadFile(path.Join(cfg.DocSrc, "docgen.yaml"))
	if err != nil {
		panic(err)
	}

	err = yaml.UnmarshalStrict(yamlStr, &dgCfg, yaml.DisallowUnknownFields)
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
			downloadChildren(item, cfg)
		case item.Source != "":
			downloadItem(item, cfg)
		}
	}
}

func downloadChildren(item DocItem, cfg *config.Config) {
	for _, child := range item.Children {
		parts := strings.Split(child, "=>")
		sourceSuffix := strings.TrimSpace(parts[0])
		nameSuffix := sourceSuffix
		if len(parts) > 1 {
			nameSuffix = strings.TrimSpace(parts[1])
		}
		downloadItem(DocItem{
			Name:   path.Join(item.Name, nameSuffix),
			Source: item.Source + "/" + sourceSuffix,
		}, cfg)
	}
}

func downloadItem(item DocItem, cfg *config.Config) {
	dipper.Logger.Infof("Downloading source %s", item.Source)
	resp, err := http.Get(item.Source)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		dipper.Logger.Warningf("Received status code %d when fetching file %s", resp.StatusCode, item.Source)
	} else {
		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}

		file := path.Join(cfg.DocDst, item.Name)
		ensureDirExists(file)
		err = ioutil.WriteFile(file, content, 0644)
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

func createItem(item DocItem, envData map[string]interface{}, cfg *config.Config) {
	name := dipper.InterpolateStr(item.Name, envData)
	dipper.Logger.Infof("Generating file %s from template %s", name, item.Template)
	tmpl, err := ioutil.ReadFile(path.Join(cfg.DocSrc, item.Template))
	if err != nil {
		panic(err)
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

	doc := dipper.InterpolateStr(string(tmpl), envData)

	file := path.Join(cfg.DocDst, name)
	ensureDirExists(file)
	err = ioutil.WriteFile(file, []byte(doc), 0644)
	if err != nil {
		panic(err)
	}
}
