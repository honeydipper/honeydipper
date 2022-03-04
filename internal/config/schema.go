// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

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
	Source      Event
	Export      map[string]interface{}
	Description string
	Meta        interface{}
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
	Description     string
	Meta            interface{}
}

// System is an abstract construct to group data, trigger and function definitions.
type System struct {
	Data        map[string](interface{})
	Triggers    map[string]Trigger
	Functions   map[string]Function
	Extends     []string
	Description string
	Meta        interface{}
}

// Workflow defines one or more actions needed to complete certain task and how they are orchestrated.
type Workflow struct {
	Name        string
	Description string
	Meta        interface{}
	Context     string
	Contexts    interface{}
	Local       map[string]interface{} `json:"with" mapstructure:"with"`

	Match       interface{} `json:"if_match" mapstructure:"if_match"`
	UnlessMatch interface{} `json:"unless_match" mapstructure:"unelss_match"`
	WhileMatch  interface{} `json:"while_match" mapstructure:"while_match"`
	UntilMatch  interface{} `json:"until_match" mapstructure:"until_match"`
	If          []string
	IfAny       []string `json:"if_any" mapstructure:"if_any"`
	Unless      []string
	UnlessAll   []string `json:"unless_any" mapstructure:"unless_all"`
	While       []string
	WhileAny    []string `json:"while_any" mapstructure:"while_any"`
	Until       []string
	UntilAll    []string `json:"until_any" mapstructure:"until_all"`

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
	Repo        string
	Branch      string
	Path        string
	Name        string
	Description string
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
