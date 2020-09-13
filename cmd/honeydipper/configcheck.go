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

	"github.com/ghodss/yaml"
	"github.com/honeydipper/honeydipper/internal/api"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/logrusorgru/aurora"
	"github.com/mitchellh/mapstructure"
)

var (
	// ErrorFieldCollision is the error message when two conflicting fields are set
	ErrorFieldCollision = fmt.Errorf("cannot define both")
	// ErrorNotDefined is the error message when an required asset is not defined
	ErrorNotDefined = fmt.Errorf("not defined")
	// ErrorNotAllowed is the message when a field is not allowed due to missing pairing field
	ErrorNotAllowed = fmt.Errorf("not allowed without pairing field")
	// ErrorNotAList is the message when a field is supposed to be a list
	ErrorNotAList = fmt.Errorf("must be a list or something interpolated into a list")
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

	wildcardContexts := map[string]bool{
		"*":       true,
		"_events": true,
	}

	contextErrors := false
	for contextName, contextValue := range cfg.DataSet.Contexts {
		for wfName := range contextValue.(map[string]interface{}) {
			if !wildcardContexts[wfName] {
				errMsg := checkContext(cfg, wfName)
				if len(errMsg) > 0 {
					if !contextErrors {
						fmt.Printf("\nFound errors in contexts:\n")
						fmt.Println("─────────────────────────────────────────────────────────────")
						contextErrors = true
					}
					fmt.Printf("context(%s): %s\n", contextName, aurora.Red(errMsg))
					ret = 1
				}
			}
		}
	}

	// flag to print the header
	ruleErrors := false
	for _, rule := range cfg.DataSet.Rules {
		location, errMsg := checkWorkflow(rule.Do, cfg)
		if len(errMsg) > 0 {
			rule.When.Match = map[string]interface{}{
				"_": aurora.Cyan("truncated ..."),
			}
			if !ruleErrors {
				fmt.Printf("\nFound errors in rules:\n")
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
				fmt.Printf("\nFound errors in workflows:\n")
				fmt.Println("─────────────────────────────────────────────────────────────")
				workflowErrors = true
			}
			fmt.Printf("workflow(%s, `%s`): %s\n", name, aurora.Cyan(location), aurora.Red(errMsg))
			ret = 1
		}
	}

	if checkAuthRules(cfg) > 0 {
		ret = 1
	}

	return ret
}

func checkAuthRules(cfg *config.Config) int {
	repo, ok := cfg.Loaded[cfg.InitRepo]
	if !ok {
		// no init repo during tests
		return 0
	}
	b, e := repo.ReadFile("/tests/api_auth_tests.yaml")
	if e != nil {
		return 0
	}

	type AuthTests struct {
		Models   []interface{}
		Policies []interface{}
		Tests    []struct {
			Name    string
			Subject string
			Object  string
			Action  string
			Result  bool
		}
	}
	var tests AuthTests
	e = yaml.Unmarshal(b, &tests)
	if e != nil {
		fmt.Printf("\nFound errors loading authorization tests definition:\n")
		fmt.Println("─────────────────────────────────────────────────────────────")
		fmt.Printf("%+v\n", aurora.Red(e))
		return 1
	}

	var apiCfg map[string]interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				e = r.(error)
				fmt.Printf("\nFound errors injecting tests into authorization rules:\n")
				fmt.Println("─────────────────────────────────────────────────────────────")
				fmt.Printf("%+v\n", aurora.Red(e))
			}
		}()
		apiCfg = dipper.MustGetMapData(cfg.DataSet.Drivers, "daemon.services.api").(map[string]interface{})
		casbinCfg := dipper.MustGetMapData(apiCfg, "auth.casbin").(map[string]interface{})
		casbinCfg["models"] = append(casbinCfg["models"].([]interface{}), tests.Models...)
		casbinCfg["policies"] = append(casbinCfg["policies"].([]interface{}), tests.Policies...)
	}()
	if e != nil {
		return 1
	}

	l := api.NewStore(nil)
	func() {
		defer func() {
			if r := recover(); r != nil {
				e = r.(error)
				fmt.Printf("\nFound errors loading authorization rules:\n")
				fmt.Println("─────────────────────────────────────────────────────────────")
				fmt.Printf("%+v\n", aurora.Red(e))
			}
		}()
		l.GetAPIHandler("/api", apiCfg)
	}()
	if e != nil {
		return 1
	}

	failedTests := []int{}
	processed := 0
	for i, test := range tests.Tests {
		ok, err := l.Enforce(test.Subject, test.Object, test.Action)
		if err != nil {
			e = err
			processed = i
			break
		}
		if ok != test.Result {
			failedTests = append(failedTests, i)
		}
	}

	if e != nil || len(failedTests) > 0 {
		fmt.Printf("\nFound errors running authorization tests:\n")
		fmt.Println("─────────────────────────────────────────────────────────────")
		for _, num := range failedTests {
			test := tests.Tests[num]
			fmt.Printf(
				"%s: Sub: %s, Obj: %s, Act: %s, Expected: %t, Found: %t\n",
				aurora.Yellow(test.Name),
				test.Subject,
				test.Object,
				test.Action,
				aurora.BrightYellow(test.Result),
				aurora.Red(!test.Result),
			)
		}
		if e != nil {
			test := tests.Tests[processed]
			fmt.Printf(
				"%s: Sub: %s, Obj: %s, Act: %s, Expected: %t, Error: %+v\n",
				aurora.Yellow(test.Name),
				test.Subject,
				test.Object,
				test.Action,
				test.Result,
				aurora.Red(e),
			)
		}
		fmt.Println()
		return 1
	}

	return 0
}

func checkContext(cfg *config.Config, ctxWorkflowName string) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(dipperCLError); ok {
				msg = e.msg
			} else {
				msg = r.(error).Error()
			}
		}
	}()
	checkObjectExists("workflow", ctxWorkflowName, cfg.DataSet.Workflows)
	return msg
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

// make sure there aint multiple actions declared.
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

// check to make sure only one conditional field and one else field.
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

// helper functions below.

func checkIsList(name string, f interface{}) {
	if f == nil {
		return
	}

	if s, ok := f.(string); ok {
		if s != "" && hasLiteral(s) {
			panic(fmt.Errorf("field \"%s\" %w", name, ErrorNotAList))
		}
	} else {
		v := reflect.ValueOf(f)
		if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
			panic(fmt.Errorf("field \"%s\" %w", name, ErrorNotAList))
		}
	}
}

func checkObjectExists(t, name string, m interface{}) {
	if name != "" && !hasInterpolation(name) {
		if !reflect.ValueOf(m).MapIndex(reflect.ValueOf(name)).IsValid() {
			panic(fmt.Errorf("%s \"%s\" %w", t, name, ErrorNotDefined))
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
			panic(fmt.Errorf("%w \"%s\" and \"%s\"", ErrorFieldCollision, f.field, name))
		}
		f.field = name
	}
}

func (f *fieldChecker) allowFieldWhenSet(name string, condition bool) {
	if condition && f.field == "" {
		panic(fmt.Errorf("field \"%s\" %w", name, ErrorNotAllowed))
	}
}
