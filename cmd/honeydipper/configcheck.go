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
	"reflect"
	"strings"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/logrusorgru/aurora"
	"github.com/mitchellh/mapstructure"
)

const (
	// ErrorFieldCollision is the error message when two conflicting fields are set
	ErrorFieldCollision = "cannot define both `%s` and `%s`"
	// ErrorNotDefined is the error message when an required asset is not defined
	ErrorNotDefined = "%s `%s` not defined"
	// ErrorNotAllowed is the message when a field is not allowed due to missing pairing field
	ErrorNotAllowed = "field `%s` not allowed without pairing field"
	// ErrorNotAList is the message when a field is supposed to be a list
	ErrorNotAList = "field `%s` must be a list or something interpolated into a list"
)

type dipperCLError struct {
	location string
	msg      string
}

func runConfigCheck(cfg *config.Config) int {
	ret := 0
	for spec, repo := range cfg.Loaded {
		if len(repo.Errors) > 0 {
			ret = 1
			fmt.Printf("\nRepo [%s] Branch [%s] Path [%s]\n", aurora.Cyan(spec.Repo), aurora.Cyan(spec.Branch), aurora.Cyan(spec.Path))
			fmt.Println("─────────────────────────────────────────────────────────────")
			for _, err := range repo.Errors {
				msg := err.Error.Error()
				// transforming error message
				msg = strings.Replace(msg, "error converting YAML to JSON: yaml: ", "", 1)
				msg = strings.Replace(msg, "error unmarshaling JSON:", "", 1)
				msg = strings.Replace(msg, "unmarshal errors:\n  ", "", 1)

				fmt.Printf("%s: %s\n", err.File[1:], aurora.Red(msg))
			}
		}
	}

	ruleErrors := false
	for _, rule := range cfg.DataSet.Rules {
		location, errMsg := checkWorkflow(rule.Do, cfg)
		if len(errMsg) > 0 {
			rule.When.Match = map[string]interface{}{
				"_": aurora.Cyan("truncated ..."),
			}
			if !ruleErrors {
				fmt.Printf("\nFound error in Rules\n")
				fmt.Println("─────────────────────────────────────────────────────────────")
				ruleErrors = true
			}
			fmt.Printf("rule(%+v, `%s`): %s\n", rule.When, aurora.Cyan(location), aurora.Red(errMsg))
			ret = 1
		}
	}

	workflowErrors := false
	for name, workflow := range cfg.DataSet.Workflows {
		location, errMsg := checkWorkflow(workflow, cfg)
		if len(errMsg) > 0 {
			if !workflowErrors {
				fmt.Printf("\nFound error in Workflows\n")
				fmt.Println("─────────────────────────────────────────────────────────────")
				workflowErrors = true
			}
			fmt.Printf("workflow(%s, `%s`): %s\n", name, aurora.Cyan(location), aurora.Red(errMsg))
			ret = 1
		}
	}

	return ret
}

func checkWorkflow(w config.Workflow, cfg *config.Config) (location string, msg string) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(dipperCLError); ok {
				location = e.location
				msg = e.msg
			} else {
				msg = r.(error).Error()
			}
		}
	}()

	location = "/"

	checkWorkflowActions(w)
	checkWorkflowConditions(w)
	checkObjectExists("workflow", w.Workflow, cfg.DataSet.Workflows)
	checkWorkflowFunction(w, cfg)
	checkWorkflowDriver(w, cfg)

	checkIsList("contexts", w.Contexts)
	checkIsList("iterate", w.Iterate)
	checkIsList("iterate_parallel", w.IterateParallel)

	checkChildWorkflow("else/", w.Else, cfg)

	for i, item := range w.Steps {
		checkChildWorkflow(fmt.Sprintf("step/%d/", i), item, cfg)
	}

	for i, item := range w.Threads {
		checkChildWorkflow(fmt.Sprintf("thread/%d/", i), item, cfg)
	}

	return location, msg
}

func checkChildWorkflow(prefix string, child interface{}, cfg *config.Config) {
	if child == nil {
		return
	}

	w, ok := child.(config.Workflow)
	if !ok {
		err := mapstructure.Decode(child, &w)
		if err != nil {
			panic(dipperCLError{location: prefix, msg: err.Error()})
		}
	}

	l, msg := checkWorkflow(w, cfg)
	if msg != "" {
		panic(dipperCLError{location: prefix + l, msg: msg})
	}
}

func checkWorkflowFunction(w config.Workflow, cfg *config.Config) {
	if w.CallFunction != "" && !hasInterpolation(w.CallFunction) {
		parts := strings.Split(w.CallFunction, ".")
		checkObjectExists("system", parts[0], cfg.DataSet.Systems)
		checkObjectExists(parts[0]+" function", parts[1], cfg.DataSet.Systems[parts[0]].Functions)
	} else if w.Function.Target.System != "" && !hasInterpolation(w.Function.Target.System) {
		f := w.Function.Target
		checkObjectExists("system", f.System, cfg.DataSet.Systems)
		if !hasInterpolation(f.Function) {
			checkObjectExists(f.System+" function", f.Function, cfg.DataSet.Systems[f.System].Functions)
		}
	}
}

func checkWorkflowDriver(w config.Workflow, cfg *config.Config) {
	if w.CallDriver != "" && !hasInterpolation(w.CallDriver) {
		parts := strings.Split(w.CallDriver, ".")
		checkObjectExists("driver", parts[0], cfg.DataSet.Drivers)
	} else if w.Function.Driver != "" && !hasInterpolation(w.Function.Driver) {
		checkObjectExists("driver", w.Function.Driver, cfg.DataSet.Drivers)
	}
}

func checkWorkflowActions(w config.Workflow) {
	f := &fieldChecker{}

	f.setField("call_workflow", hasLiteral(w.Workflow))
	f.setField("call_function", hasLiteral(w.CallFunction))
	f.setField("call_driver", hasLiteral(w.CallDriver))
	f.setField("wait", w.Wait != "")
	f.setField("steps", len(w.Steps) > 0)
	f.setField("threads", len(w.Threads) > 0)
	f.setField("switch", w.Switch != "")
}

func checkWorkflowConditions(w config.Workflow) {
	f := &fieldChecker{}

	f.setField("if_match", w.Match != nil)
	f.setField("unless_match", w.UnlessMatch != nil)
	f.setField("while_match", w.WhileMatch != nil)
	f.setField("until_match", w.UntilMatch != nil)

	f.setField("if", len(w.If) > 0)
	f.setField("unless", len(w.Unless) > 0)
	f.setField("if_any", len(w.IfAny) > 0)
	f.setField("unless_all", len(w.UnlessAll) > 0)
	f.setField("while", len(w.While) > 0)
	f.setField("while_any", len(w.WhileAny) > 0)
	f.setField("until_all", len(w.UntilAll) > 0)

	f.allowFieldWhenSet("else", w.Else != nil)
}

// helper functions below

func checkIsList(name string, f interface{}) {
	if f != nil {
		if s, ok := f.(string); ok {
			if s != "" && hasLiteral(s) {
				panic(fmt.Errorf(ErrorNotAList, name))
			}
		} else {
			v := reflect.ValueOf(f)
			if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
				panic(fmt.Errorf(ErrorNotAList, name))
			}
		}
	}
}

func checkObjectExists(t, name string, m interface{}) {
	if name != "" && !hasInterpolation(name) {
		if !reflect.ValueOf(m).MapIndex(reflect.ValueOf(name)).IsValid() {
			panic(fmt.Errorf(ErrorNotDefined, t, name))
		}
	}
}

func hasLiteral(param string) bool {
	s := strings.TrimSpace(param)
	return s != "" && s[0] != '$' && !(strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}")) && !strings.HasPrefix(s, ":yaml:")
}

func hasInterpolation(param string) bool {
	s := strings.TrimSpace(param)
	return s != "" && (s[0] == '$' || strings.Contains(s, "{{")) || strings.HasPrefix(s, ":yaml:")
}

type fieldChecker struct {
	field string
}

func (f *fieldChecker) setField(name string, condition bool) {
	if condition {
		if f.field != "" {
			panic(fmt.Errorf(ErrorFieldCollision, f.field, name))
		}
		f.field = name
	}
}

func (f *fieldChecker) allowFieldWhenSet(name string, condition bool) {
	if condition && f.field == "" {
		panic(fmt.Errorf(ErrorNotAllowed, name))
	}
}
