// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package main

import (
	"fmt"
	"testing"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestRunConfigCheck(t *testing.T) {
	runConfigTestCases := []interface{}{
		[]interface{}{
			&config.Config{
				Staged:   &config.DataSet{},
				Services: []string{ConfigCheckService},
			},
			0,
			"runConfigCheck should return zero for empty config repo",
		},
		[]interface{}{
			&config.Config{
				Staged:   &config.DataSet{},
				Services: []string{ConfigCheckService},
				Loaded: map[config.RepoInfo]*config.Repo{
					{Repo: "good one"}: {Errors: nil},
					{Repo: "bad one"}:  {Errors: []config.Error{{Error: fmt.Errorf("error converting YAML to JSON: yaml: %s", "test.yaml"), File: "test"}}},
				},
			},
			1,
			"runConfigCheck should return non-zero if there is error loading yaml",
		},
		[]interface{}{
			&config.Config{
				Staged: &config.DataSet{
					Contexts: map[string]interface{}{"_default": map[string]interface{}{"wf-not-exists": map[string]interface{}{"data": "value"}}},
				},
				Services: []string{ConfigCheckService},
			},
			1,
			"runConfigCheck should return non-zero if a context is missing matching workflow",
		},
		[]interface{}{
			&config.Config{
				Staged: &config.DataSet{
					Rules: []config.Rule{
						{When: config.Trigger{Driver: "non-exist"}, Do: config.Workflow{Workflow: "non-exist"}},
					},
				},
				Services: []string{ConfigCheckService},
			},
			1,
			"runConfigCheck should return non-zero if a rule calls a missing workflow",
		},
		[]interface{}{
			&config.Config{
				Staged: &config.DataSet{
					Workflows: map[string]config.Workflow{"test-workflow": {Workflow: "dne"}},
				},
				Services: []string{ConfigCheckService},
			},
			1,
			"runConfigCheck should return non-zero if a workflow calls a missing workflow",
		},
	}

	for _, tcase := range runConfigTestCases {
		tc := tcase.([]interface{})
		result := runConfigCheck(tc[0].(*config.Config))
		assert.Equal(t, tc[1], result, tc[2])
	}
}

func TestCheckObjectExistsWorkFlowDoesNotExist(t *testing.T) {
	defer recoverAssertion(`workflow "test-fail" not defined`, t)
	workflows := map[string]config.Workflow{
		"test-wf": {
			Name: "test",
		},
	}
	checkObjectExists("workflow", "test-fail", workflows)
}

func TestCheckObjectExists(t *testing.T) {
	defer recoverAssertion("", t)
	workflows := map[string]config.Workflow{
		"test-wf": {
			Name: "test",
		},
	}
	checkObjectExists("workflow", "test-wf", workflows)
}

func TestCheckWorkflowDriverCallDriver(t *testing.T) {
	defer recoverAssertion(`driver "test-driver" not defined`, t)
	cfg := &config.Config{
		Staged: &config.DataSet{
			Drivers: map[string]interface{}{
				"driver1": "",
			},
		},
	}
	workflow := config.Workflow{Name: "test", CallDriver: "test-driver.test-call"}
	checkWorkflowDriver(workflow, cfg)
}

func TestCheckWorkflowDriverFunctionDriver(t *testing.T) {
	defer recoverAssertion(`driver "test-driver" not defined`, t)
	cfg := &config.Config{
		Staged: &config.DataSet{
			Drivers: map[string]interface{}{
				"driver1": "",
			},
		},
	}
	workflow := config.Workflow{Name: "test", Function: config.Function{Driver: "test-driver"}}
	checkWorkflowDriver(workflow, cfg)
}

// TestCheckWorkflowFunction.
var wfFunctionTestCases = []struct {
	wf  config.Workflow
	cfg *config.Config
	out string
}{
	{
		config.Workflow{Name: "test", CallFunction: "test_system.test_function"},
		&config.Config{Staged: &config.DataSet{Systems: map[string]config.System{
			"test_system": {Functions: map[string]config.Function{"test_function": {Driver: "web"}}},
		}}},
		"",
	},
	{
		config.Workflow{Name: "test", CallFunction: "test_system.test_function"},
		&config.Config{Staged: &config.DataSet{Systems: map[string]config.System{
			"system_does_not_exist": {Functions: map[string]config.Function{"test_function": {Driver: "web"}}},
		}}},
		`system "test_system" not defined`,
	},
	{
		config.Workflow{Name: "test", CallFunction: "test_system.test_function"},
		&config.Config{Staged: &config.DataSet{Systems: map[string]config.System{
			"test_system": {Functions: map[string]config.Function{"test_function_does_not_exist": {Driver: "web"}}},
		}}},
		`test_system function "test_function" not defined`,
	},
	{
		config.Workflow{Name: "test", Function: config.Function{Target: config.Action{System: "test_system", Function: "test_function"}}},
		&config.Config{Staged: &config.DataSet{Systems: map[string]config.System{
			"test_system": {Functions: map[string]config.Function{"test_function": {Driver: "web"}}},
		}}},
		"",
	},
	{
		config.Workflow{Name: "test", Function: config.Function{Target: config.Action{System: "test_system", Function: "test_function"}}},
		&config.Config{Staged: &config.DataSet{Systems: map[string]config.System{
			"not_exist": {Functions: map[string]config.Function{"test_function": {Driver: "web"}}},
		}}},
		`system "test_system" not defined`,
	},
	{
		config.Workflow{Name: "test", Function: config.Function{Target: config.Action{System: "test_system", Function: "test_function"}}},
		&config.Config{Staged: &config.DataSet{Systems: map[string]config.System{
			"test_system": {Functions: map[string]config.Function{"not_exist": {Driver: "web"}}},
		}}},
		`test_system function "test_function" not defined`,
	},
}

func TestCheckWorkflowFunctions(t *testing.T) {
	for _, tc := range wfFunctionTestCases {
		testCheckWorkflowFunctionHelper(t, tc.wf, tc.cfg, tc.out)
	}
}

func testCheckWorkflowFunctionHelper(t *testing.T, wf config.Workflow, cfg *config.Config, out string) {
	defer recoverAssertion(out, t)
	checkWorkflowFunction(wf, cfg)
}

// TestCheckWorkflowActions.
var wfActionTestCases = []struct {
	in  config.Workflow
	out string
}{
	{
		config.Workflow{Name: "test", Workflow: "test_workflow"},
		"",
	},
	{
		config.Workflow{Name: "test", Workflow: "test_workflow", CallFunction: "blah"},
		`cannot define both "call_workflow" and "call_function"`,
	},
	{
		config.Workflow{Name: "test", Workflow: "test_workflow", Steps: []config.Workflow{{}}},
		`cannot define both "call_workflow" and "steps"`,
	},
	{
		config.Workflow{Name: "test", Workflow: "test_workflow", CallDriver: "blah", Switch: "switch"},
		`cannot define both "call_workflow" and "call_driver"`,
	},
	{
		config.Workflow{Name: "test", Workflow: "test_workflow", Switch: "switch"},
		`cannot define both "call_workflow" and "switch"`,
	},
}

func TestCheckWorkflowActions(t *testing.T) {
	for _, tc := range wfActionTestCases {
		testCheckWorkflowActionsHelper(t, tc.in, tc.out)
	}
}

func testCheckWorkflowActionsHelper(t *testing.T, wf config.Workflow, out string) {
	defer recoverAssertion(out, t)
	checkWorkflowActions(wf)
}

// TestWorkFlowConditions.
var wfConditionsTestCases = []struct {
	in  config.Workflow
	out string
}{
	{config.Workflow{Name: "test", Match: "match"}, ""},
	{config.Workflow{Name: "test", Match: "match", UnlessMatch: "UnlessMatch"}, `cannot define both "if_match" and "unless_match"`},
	{config.Workflow{Name: "test", Else: "else"}, `field "else" not allowed without pairing field`},
	{config.Workflow{Name: "test", If: []string{"1", "2"}, Else: "else"}, ""},
	{config.Workflow{Name: "test", UntilAll: []string{"1", "2"}, While: []string{"1", "2"}}, `cannot define both "while" and "until_all"`},
}

func TestCheckWorkflowConditions(t *testing.T) {
	for _, tc := range wfConditionsTestCases {
		testCheckWorkflowConditionsHelper(t, tc.in, tc.out)
	}
}

func testCheckWorkflowConditionsHelper(t *testing.T, wf config.Workflow, out string) {
	defer recoverAssertion(out, t)
	checkWorkflowConditions(wf)
}

func TestCheckIsListString(t *testing.T) {
	defer recoverAssertion(`field "test" must be a list or something interpolated into a list`, t)
	checkIsList("test", "notList")
}

func TestCheckIsListMap(t *testing.T) {
	defer recoverAssertion(`field "test" must be a list or something interpolated into a list`, t)
	checkIsList("test", make(map[string]int))
}

func TestCheckIsListNil(t *testing.T) {
	defer recoverAssertion("", t)
	checkIsList("test", nil)
}

var hasLiteralTestCases = []struct {
	in  string
	out bool
}{
	{"blah", true},
	{"{{blah", true},
	{"", false},
	{"{{}}", false},
	{"${{}}}}", false},
	{":yaml:", false},
}

func TestHasLiteral(t *testing.T) {
	for _, tc := range hasLiteralTestCases {
		out := hasLiteral(tc.in)
		if out != tc.out {
			t.Errorf("Expected: %v, Got: %v instead", tc.out, out)
		}
	}
}

var hasInterpolationTestCases = []struct {
	in  string
	out bool
}{
	{"blah", false},
	{"{{blah", true},
	{"", false},
	{"{{}}", true},
	{"${{}}}}", true},
	{":yaml:", true},
}

func TestHasInterpolation(t *testing.T) {
	for _, tc := range hasInterpolationTestCases {
		out := hasInterpolation(tc.in)
		if out != tc.out {
			t.Errorf("Expected: %v, Got: %v instead", tc.out, out)
		}
	}
}

// helper func.
func recoverAssertion(out string, t *testing.T) {
	expected := out
	var msg string
	if r := recover(); r != nil {
		if e, ok := r.(dipperCLError); ok {
			msg = e.msg
		} else {
			msg = r.(error).Error()
		}
	}

	if msg != expected {
		t.Errorf("Expected: %s, Got: %s instead", out, msg)
	}
}
