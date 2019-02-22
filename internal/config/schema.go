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
	Conditions map[string]interface{} `json:"conditions,omitempty"`
	// A trigger should have only one of source event a raw event.
	Source Event `json:"source,omitempty"`
}

// Function is the datastructure hold the information to run actions.
type Function struct {
	Driver     string                   `json:"driver,omitempty"`
	RawAction  string                   `json:"rawaction,omitempty"`
	Parameters map[string](interface{}) `json:"parameters,omitempty"`
	// An action should have only one of target action or a raw action.
	Target Action `json:"target,omitempty"`
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
	Type      string `json:"type,omitempty"`
	Condition string `json:"condition,omitempty"`
	Content   interface{}
	Data      map[string]interface{}
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
}

// DriverMeta holds the meta information about the driver itself.
type DriverMeta struct {
	Name     string
	Feature  string
	Services []string
	Data     interface{}
}

// Driver is The parent class for all driver handlers.
type Driver struct {
	Type       string
	Executable string
	Arguments  []string
}
