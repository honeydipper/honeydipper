// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package config

import (
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
)

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
	Local       interface{} `json:"with" mapstructure:"with"`
	Parameters  interface{} `json:"parameters" mapstructure:"parameters"`

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
	IteratePool     string      `json:"iterate_pool" mapstructure:"iterate_pool"`
	IterateAs       string      `json:"iterate_as" mapstructure:"iterate_as"`

	Retry   string
	Backoff string

	OnError      string `json:"on_error" mapstructure:"on_error"`
	OnFailure    string `json:"on_failure" mapstructure:"on_failure"`
	Workflow     string `json:"call_workflow" mapstructure:"call_workflow"`
	Function     Function
	CallFunction string `json:"call_function" mapstructure:"call_function"`
	CallDriver   string `json:"call_driver" mapstructure:"call_driver"`
	CallAgent    string `json:"call_agent" mapstructure:"call_agent"`
	WaitAgent    string `json:"wait_agent" mapstructure:"wait_agent"`
	Steps        []Workflow
	Threads      []Workflow
	Wait         string
	Detach       bool
	Resume       string

	SendEvent interface{} `json:"send_event" mapstructure:"send_event"`

	Switch  string
	Cases   map[string]interface{}
	Default interface{}

	CacheKey string `json:"cache_key" mapstructure:"cache_key"`
	CacheTTL string `json:"cache_ttl" mapstructure:"cache_ttl"`

	Export          map[string]interface{}
	ExportOnSuccess map[string]interface{} `json:"export_on_success" mapstructure:"export_on_success"`
	ExportOnFailure map[string]interface{} `json:"export_on_failure" mapstructure:"export_on_failure"`
	ExportOnError   map[string]interface{} `json:"export_on_error" mapstructure:"export_on_error"`
	NoExport        []string               `json:"no_export" mapstructure:"no_export"`
}

// Rule is a data structure defining what action to take when certain event happen.
type Rule struct {
	When Trigger
	Do   Workflow
}

// RepoKey holds the identity fields of a RepoInfo and can be used as a map key.
type RepoKey struct {
	Repo     string
	Branch   string
	Path     string
	InitFile string
}

// RepoInfo points to a git repo where config data can be read from.
type RepoInfo struct {
	Repo        string
	Branch      string
	Path        string
	InitFile    string `json:"init_file" mapstructure:"init_file"`
	Name        string
	Description string
	KeyFile     string `json:"key_file" mapstructure:"key_file"`
	KeyPassEnv  string `json:"key_pass_env" mapstructure:"key_pass_env"`

	TokenSource string `json:"token_source" mapstructure:"token_source"`
	Username    string
	PassEnv     string `json:"pass_env" mapstructure:"pass_env"`

	Options map[string]interface{}
}

// Key returns the RepoKey (identity fields only) for use as a map key.
func (r RepoInfo) Key() RepoKey {
	return RepoKey{Repo: r.Repo, Branch: r.Branch, Path: r.Path, InitFile: r.InitFile}
}

// AgentToolDef defines if the workflow is a workflow or a system.
type AgentToolDef struct {
	Type     string
	Name     string
	Only     []string
	Excludes []string
}

// Agent is a abstract data structure including AI driver, prompt and tools definitions for an agent session.
type Agent struct {
	Name               string
	Driver             string
	Engine             string
	SystemPrompt       string                     `json:"system_prompt" mapstructure:"system_prompt"`
	InferencePrompt    string                     `json:"inference_prompt" mapstructure:"inference_prompt"`
	ShouldEmitThoughts bool                       `json:"should_emit_thoughts" mapstructure:"should_emit_thoughts"`
	ShouldStream       bool                       `json:"should_stream" mapstructure:"should_stream"`
	MaxHistoryLen      int                        `json:"max_history_len" mapstructure:"max_history_len"`
	CompactionPolicy   *agentpkg.CompactionPolicy `json:"compaction_policy" mapstructure:"compaction_policy"`
	Tools              []AgentToolDef
	ModelData          map[string]interface{} `json:"model_data" mapstructure:"model_data"`
	AgentSettings      interface{}            `json:"agent_settings" mapstructure:"additional_data"`
	PreContext         []string               `json:"pre_context" mapstructure:"pre_context"`
	SkillsPaths        []string               `json:"skills" mapstructure:"skills"`
	FileTool           string                 `json:"file_tool" mapstructure:"file_tool"`
	Description        string
	TokenCounter       string `json:"token_counter" mapstructure:"token_counter"`
	TurnLockTimeout    string `json:"turn_lock_timeout" mapstructure:"turn_lock_timeout"`
	DriverCallTimeout  string `json:"driver_call_timeout" mapstructure:"driver_call_timeout"`
	Meta               interface{}
}

// DataSet is a subset of configuration that can be assembled to the complete final configuration.
type DataSet struct {
	Systems   map[string]System
	Rules     []Rule
	Agents    map[string]Agent
	Drivers   map[string]interface{}
	Includes  []string
	Repos     []RepoInfo
	Workflows map[string]Workflow
	Contexts  map[string]interface{}
}
