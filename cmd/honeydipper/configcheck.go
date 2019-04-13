// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package honeydipper is an event-driven, rule based orchestration platform tailor towards
// DevOps and system engineering workflows.
package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/logrusorgru/aurora"
	"github.com/mitchellh/mapstructure"
)

const (
	// InterpolationPrefixPath the path interpolation prefix
	InterpolationPrefixPath = ":path:"
	// InterpolationPrefixYaml the yaml interpolation prefix
	InterpolationPrefixYaml = ":yaml:"
)

func runConfigCheck(cfg *config.Config) bool {
	hasError := false
	for spec, repo := range cfg.Loaded {
		if len(repo.Errors) > 0 {
			hasError = true
			fmt.Printf("\nRepo [%s] Branch [%s] Path [%s]\n", aurora.Cyan(spec.Repo), aurora.Cyan(spec.Branch), aurora.Cyan(spec.Path))
			fmt.Println("─────────────────────────────────────────────────────────────")
			for _, err := range repo.Errors {
				msg := err.Error.Error()
				// transforming error message
				msg = strings.Replace(msg, "error converting YAML to JSON: yaml: ", "", 1)

				fmt.Printf("%s: %s\n", err.File[1:], aurora.Red(msg))
			}
		}
	}

	ruleErrors := false
	for _, rule := range cfg.DataSet.Rules {
		location, errMsg := checkWorkflow(rule.Do)
		if len(errMsg) > 0 {
			rule.When.Conditions = map[string]interface{}{
				"_": aurora.Cyan("truncated ..."),
			}
			if !ruleErrors {
				fmt.Printf("\nFound error in Rules\n")
				fmt.Println("─────────────────────────────────────────────────────────────")
				ruleErrors = true
			}
			fmt.Printf("rule(%+v, %s): %s\n", rule.When, aurora.Cyan(location), aurora.Red(errMsg))
			hasError = true
		}
	}

	workflowErrors := false
	for name, workflow := range cfg.DataSet.Workflows {
		location, errMsg := checkWorkflow(workflow)
		if len(errMsg) > 0 {
			if !workflowErrors {
				fmt.Printf("\nFound error in Workflows\n")
				fmt.Println("─────────────────────────────────────────────────────────────")
				workflowErrors = true
			}
			fmt.Printf("workflow(%s, %s): %s\n", name, aurora.Cyan(location), aurora.Red(errMsg))
			hasError = true
		}
	}

	return hasError
}

// to be refactored to simpler function or functions
//nolint:gocyclo
func checkWorkflow(w config.Workflow) (string, string) {
	var location string

	// check content based on type of workflow
	switch w.Type {
	case "":
		fallthrough
	case "name":
		if _, ok := w.Content.(string); !ok {
			return location, "name of workflow missing for named workflow"
		}
	case "suspend":
		if _, ok := w.Content.(string); !ok {
			return location, "suspend ID missing (content) for suspend workflow"
		}
	case "pipe":
		fallthrough
	case "if":
		fallthrough
	case "parallel":
		switch v := w.Content.(type) {
		case string:
			v = strings.TrimSpace(v)
			if v[:6] == InterpolationPrefixPath || v[:6] == InterpolationPrefixYaml {
				// skip interpolation
			} else {
				return location, "workflow content should not be a string for pipe, parallel, if"
			}
		case []interface{}:
			if len(v) == 0 {
				return location, "workflow content should have more than one child for pipe, parallel, if"
			}
			if w.Type == "if" {
				if len(v) > 2 {
					return location, "`if` type workflow should at most 2 branches in content"
				}
			}
		default:
			return location, "workflow content should be a slice of children for pipe, parallel, if"
		}
		// check children
		for i, child := range w.Content.([]interface{}) {
			location = w.Type + "/" + strconv.Itoa(i)
			switch c := child.(type) {
			case string:
				c = strings.TrimSpace(c)
				if c[:6] == InterpolationPrefixPath || c[:6] == InterpolationPrefixYaml {
					// skip interpolation
				} else {
					return location, "workflow should not be a string"
				}
			case map[string]interface{}:
				var cw config.Workflow
				err := mapstructure.Decode(c, &cw)
				if err != nil {
					return location, err.Error()
				}
				childLocation, errMsg := checkWorkflow(cw)
				if len(errMsg) > 0 {
					return location + "/" + childLocation, errMsg
				}
			default:
				return location, "workflow should be a structure with some required/optional fields"
			}
		}
	case "switch":
		switch v := w.Content.(type) {
		case string:
			v = strings.TrimSpace(v)
			if v[:6] == InterpolationPrefixPath || v[:6] == InterpolationPrefixYaml || v[:2] == "{{" {
				// skip interpolation
			} else {
				return location, "workflow should not be a string for switch"
			}
		case map[string]interface{}:
			// check children
			for i, child := range v {
				location = w.Type + "/" + i
				switch c := child.(type) {
				case string:
					c = strings.TrimSpace(c)
					if c[:6] == InterpolationPrefixPath || c[:6] == InterpolationPrefixYaml {
						// skip interpolation
					} else {
						return location, "workflow should not be a string"
					}
				case map[string]interface{}:
					var cw config.Workflow
					err := mapstructure.Decode(c, &cw)
					if err != nil {
						return location, err.Error()
					}
					childLocation, errMsg := checkWorkflow(cw)
					if len(errMsg) > 0 {
						return location + "/" + childLocation, errMsg
					}
				default:
					return location, "workflow should be a structure with some required/optional fields"
				}
			}
		default:
			return location, "workflow content should be a map of child workflows for switch"
		}
	}

	// check condition
	if w.Type == "if" || w.Type == "switch" {
		if len(w.Condition) == 0 {
			return location, "condition missing for if or switch workflow"
		}
	} else {
		if len(w.Condition) > 0 {
			return location, "condition not allowed for workflows other than if or switch"
		}
	}

	// check wfdata
	if childLocation, dataErr := checkWfdata(w.Data); dataErr != "" {
		return location + "/" + childLocation, dataErr
	}

	return "", ""
}

func checkWfdata(data interface{}) (string, string) {
	var location = "data"
	switch v := data.(type) {
	case string:
		return location, ""
	case []interface{}:
		for index, value := range v {
			location = strconv.Itoa(index)
			if childLocation, errMsg := checkWfdata(value); errMsg != "" {
				return location + "/" + childLocation, errMsg
			}
		}
	case map[string]interface{}:
		for name, value := range v {
			location = "data/" + name
			if name == "steps" {
				return location, "use `*steps` to avoid merging of the steps from outer workflows"
			} else if name == "work" {
				if wfstr, ok := value.(string); ok {
					if len(wfstr) == 0 {
						return location, "empty `work`"
					} else if wfstr[0] != ':' {
						return location, "`work` should be a workflow object or something interpolated into a workflow object"
					}
				} else if dict, ok := value.(map[string]interface{}); ok {
					var cw config.Workflow
					err := mapstructure.Decode(dict, &cw)
					if err != nil {
						return location, "`work` should be a workflow object: " + err.Error()
					}
					childLocation, errMsg := checkWorkflow(cw)
					if len(errMsg) > 0 {
						return location + "/" + childLocation, errMsg
					}
				} else {
					return location, "`work` should be a workflow object or something interpolated into a workflow object"
				}
			}
			childLocation, errMsg := checkWfdata(value)
			if errMsg != "" {
				return location + "/" + childLocation, errMsg
			}
		}
	}
	return "", ""
}
