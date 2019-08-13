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
	Driver     string
	RawEvent   string
	Match      map[string]interface{} `json:"if_match" mapstructure:"if_match"`
	Parameters map[string]interface{}
	// A trigger should have only one of source event a raw event.
	Source Event
	Export map[string]interface{}
}

// Function is the datastructure hold the information to run actions.
type Function struct {
	Driver     string
	RawAction  string
	Parameters map[string](interface{})
	// An action should have only one of target action or a raw action.
	Target          Action
	Export          map[string]interface{}
	ExportOnSuccess map[string]interface{} `json:"export_on_success" mapstructure:"export_on_success"`
	ExportOnFailure map[string]interface{} `json:"export_on_failure" mapstructure:"export_on_failure"`
}

// System is an abstract construct to group data, trigger and function definitions.
type System struct {
	Data      map[string](interface{})
	Triggers  map[string]Trigger
	Functions map[string]Function
	Extends   []string
}

// Workflow defines one or more actions needed to complete certain task and how they are orchestrated.
type Workflow struct {
	Name        string
	Description string
	Context     string
	Contexts    []string
	Local       map[string]interface{} `json:"with" mapstructure:"with"`

	Match       interface{} `json:"if_match" mapstructure:"if_match"`
	UnlessMatch interface{} `json:"unless_match" mapstructure:"unelss_match"`
	If          []string
	IfAny       []string `json:"if_any" mapstructure:"if_any"`
	Unless      []string
	UnlessAny   []string `json:"unless_any" mapstructure:"unless_any"`
	While       []string
	WhileAny    []string `json:"while_any" mapstructure:"while_any"`
	Until       []string
	UntilAny    []string `json:"until_any" mapstructure:"until_any"`

	Else interface{} `json:"else,omitempty"`

	Iterate         interface{}
	IterateParallel interface{} `json:"iterate_parallel" mapstructure:"iterate_parallel"`
	IterateAs       string      `json:"iterate_as" mapstructure:"iterate_as"`

	Retry   string
	Backoff string

	OnError      string `json:"on_error" mapstructure:"on_error"`
	OnFailure    string `json:"on_failure" mapstructure:"on_failure"`
	Workflow     string `json:"call_workflow" mapstructure:"call_workflow"`
	Function     Function
	CallFunction string `json:"call_function" mapstructure:"call_function"`
	CallDriver   string `json:"call_driver" mapstructure:"call_driver"`
	Steps        []Workflow
	Threads      []Workflow
	Wait         string

	Switch  string
	Cases   map[string]interface{}
	Default interface{}

	Export          map[string]interface{}
	ExportOnSuccess map[string]interface{} `json:"export_on_success" mapstructure:"export_on_success"`
	ExportOnFailure map[string]interface{} `json:"export_on_failure" mapstructure:"export_on_failure"`
	NoExport        []string               `json:"no_export" mapstructure:"no_export"`
}

// Rule is a data structure defining what action to take when certain event happen.
type Rule struct {
	When Trigger
	Do   Workflow
}

// RepoInfo points to a git repo where config data can be read from.
type RepoInfo struct {
	Repo   string
	Branch string
	Path   string
}

// DataSet is a subset of configuration that can be assembled to the complete final configuration.
type DataSet struct {
	Systems   map[string]System
	Rules     []Rule
	Drivers   map[string]interface{}
	Includes  []string
	Repos     []RepoInfo
	Workflows map[string]Workflow
	Contexts  map[string]interface{}
}
