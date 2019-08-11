// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package config

// Event is the runtime data representation of an event.
type Event struct {
	System  string
	Trigger string
}

// Action is the runtime data representation of an action.
type Action struct {
	System   string
	Function string
}

// Trigger is the datastructure hold the information to match and process an event.
type Trigger struct {
	Driver     string                 `json:"driver,omitempty"`
	RawEvent   string                 `json:"rawevent,omitempty"`
	Match      map[string]interface{} `json:"if_match,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	// A trigger should have only one of source event a raw event.
	Source Event                  `json:"source,omitempty"`
	Export map[string]interface{} `json:"export,omitempty"`
}

// Function is the datastructure hold the information to run actions.
type Function struct {
	Driver     string                   `json:"driver,omitempty"`
	RawAction  string                   `json:"rawaction,omitempty"`
	Parameters map[string](interface{}) `json:"parameters,omitempty"`
	// An action should have only one of target action or a raw action.
	Target          Action                 `json:"target,omitempty"`
	Export          map[string]interface{} `json:"export,omitempty"`
	ExportOnSuccess map[string]interface{} `json:"export_on_success,omitempty"`
	ExportOnFailure map[string]interface{} `json:"export_on_failure,omitempty"`
}

// System is an abstract construct to group data, trigger and function definitions.
type System struct {
	Data      map[string](interface{}) `json:"data,omitempty"`
	Triggers  map[string]Trigger       `json:"triggers,omitempty"`
	Functions map[string]Function      `json:"functions,omitempty"`
	Extends   []string                 `json:"extends,omitempty"`
}

// Workflow defines one or more actions needed to complete certain task and how they are orchestrated.
type Workflow struct {
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Context     string                 `json:"context,omitempty"`
	Contexts    []string               `json:"contexts,omitempty"`
	Local       map[string]interface{} `json:"with,omitempty"`

	Match       map[string]interface{} `json:"if_match,omitempty"`
	UnlessMatch map[string]interface{} `json:"unless_match,omitempty"`
	If          []string               `json:"if,omitempty"`
	IfAny       []string               `json:"if_any,omitempty"`
	Unless      []string               `json:"unless,omitempty"`
	UnlessAny   []string               `json:"unless_all,omitempty"`
	While       []string               `json:"while,omitempty"`
	WhileAny    []string               `json:"while_any,omitempty"`
	Until       []string               `json:"until,omitempty"`
	UntilAny    []string               `json:"until_any,omitempty"`

	Else interface{} `json:"else,omitempty"`

	Iterate         interface{} `json:"iterate,omitempty"`
	IterateParallel interface{} `json:"iterate_parallel,omitempty"`
	IterateAs       string      `json:"iterate_as,omitempty"`

	Retry   string `json:"retry,omitempty"`
	Backoff string `json:"backoff,omitempty"`

	OnError    string     `json:"on_error,omitempty"`
	OnFailure  string     `json:"on_failure,omitempty"`
	Workflow   string     `json:"call_workflow,omitempty"`
	Function   Function   `json:"function,omitempty"`
	CallFunc   string     `json:"call_function,omitempty"`
	CallDriver string     `json:"call_driver,omitempty"`
	Steps      []Workflow `json:"steps,omitempty"`
	Threads    []Workflow `json:"threads,omitempty"`
	Wait       string     `json:"wait,omitempty"`

	Switch  string                 `json:"switch,omitempty"`
	Cases   map[string]interface{} `json:"cases,omitempty"`
	Default interface{}            `json:"default,omitempty"`

	Export          map[string]interface{} `json:"export,omitempty"`
	ExportOnSuccess map[string]interface{} `json:"export_on_success,omitempty"`
	ExportOnFailure map[string]interface{} `json:"export_on_failure,omitempty"`
	NoExport        []string               `json:"no_export,omitempty"`
}

// Rule is a data structure defining what action to take when certain event happen.
type Rule struct {
	When Trigger
	Do   Workflow
}

// RepoInfo points to a git repo where config data can be read from.
type RepoInfo struct {
	Repo   string
	Branch string `json:"branch,omitempty"`
	Path   string `json:"path,omitempty"`
}

// DataSet is a subset of configuration that can be assembled to the complete final configuration.
type DataSet struct {
	Systems   map[string]System      `json:"systems,omitempty"`
	Rules     []Rule                 `json:"rules,omitempty"`
	Drivers   map[string]interface{} `json:"drivers,omitempty"`
	Includes  []string               `json:"includes,omitempty"`
	Repos     []RepoInfo             `json:"repos,omitempty"`
	Workflows map[string]Workflow    `json:"workflows,omitempty"`
	Contexts  map[string]interface{} `json:"contexts,omitempty"`
}
