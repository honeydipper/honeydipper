package main

import (
	"github.com/honeyscience/honeydipper/dipper"
	"io"
	"os/exec"
	"sync"
)

// Event : the runtime data representation of an event
type Event struct {
	System  string
	Trigger string
}

// Action : the runtime data representation of an action
type Action struct {
	System   string
	Function string
}

// Filter : internal operation to mutate the payload data between events and actions
type Filter interface{}

// Trigger : is the datastructure hold the information to match and process an event
type Trigger struct {
	Driver     string                   `json:"driver,omitempty"`
	RawEvent   string                   `json:"rawevent,omitempty"`
	Conditions interface{}              `json:"conditions,omitempty"`
	Fields     map[string](interface{}) `json:"fields,omitempty"`
	Source     Event                    `json:"source,omitempty"`
	Filters    []Filter                 `json:"filters,omitempty"`
}

// Function : is the datastructure hold the information to run actions
type Function struct {
	Driver     string                   `json:"driver,omitempty"`
	RawAction  string                   `json:"rawaction,omitempty"`
	Parameters map[string](interface{}) `json:"parameters,omitempty"`
	Results    map[string](interface{}) `json:"results,omitempty"`
	Target     Action                   `json:"target,omitempty"`
	Filters    []Filter                 `json:"filters,omitempty"`
}

// System : is an abstract construct to group data, trigger and function definitions
type System struct {
	Data      map[string](interface{}) `json:"data,omitempty"`
	Triggers  map[string]Trigger       `json:"triggers,omitempty"`
	Functions map[string]Function      `json:"functions,omitempty"`
}

// Condition : used to for conditioning in workflow
type Condition struct {
	Op     string `json:"op,omitempty"`
	Values []string
}

// Workflow : defines the steps, and relationship of the actions
type Workflow struct {
	Type       string      `json:"type,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
	Content    interface{}
}

// Rule : is a data structure defining what action to take upon an event
type Rule struct {
	When Trigger
	Do   Workflow
}

// RepoInfo : points a git repo where config data can be read from
type RepoInfo struct {
	Repo   string
	Branch string `json:"branch,omitempty"`
	Path   string `json:"path,omitempty"`
}

// ConfigSet : is a complete set of configuration at a specific moment
type ConfigSet struct {
	Systems   map[string]System      `json:"systems,omitempty"`
	Rules     []Rule                 `json:"rules,omitempty"`
	Drivers   map[string]interface{} `json:"drivers,omitempty"`
	Includes  []string               `json:"includes,omitempty"`
	Repos     []RepoInfo             `json:"repos,omitempty"`
	Workflows map[string]Workflow    `json:"workflows,omitempty"`
}

// ConfigRepo : used to track what has been loaded in a repo
type ConfigRepo struct {
	parent *Config
	repo   *RepoInfo
	config ConfigSet
	files  map[string]bool
	root   string
}

// Config : is the complete configration of the daemon including history and the running services
type Config struct {
	initRepo          RepoInfo
	services          []string
	config            *ConfigSet
	loaded            map[RepoInfo]*ConfigRepo
	wd                string
	lastRunningConfig struct {
		config *ConfigSet
		loaded map[RepoInfo]*ConfigRepo
	}
}

// Service : service is a collection of daemon's feature
type Service struct {
	name               string
	config             *Config
	driverRuntimes     map[string]*DriverRuntime
	expects            map[string][]func(*dipper.Message)
	responders         map[string][]func(*DriverRuntime, *dipper.Message)
	transformers       map[string][]func(*DriverRuntime, *dipper.Message) *dipper.Message
	dynamicFeatureData map[string]interface{}
	expectLock         sync.Mutex
	driverLock         sync.Mutex
	selectLock         sync.Mutex
	Route              func(*dipper.Message) []RoutedMessage
	DiscoverFeatures   func(*ConfigSet) map[string]interface{}
	ServiceReload      func(*Config)
}

// Driver : the parent class for all driver types
type Driver struct {
	Type       string
	Executable string
	Arguments  []string
	PreStart   func(string, *DriverRuntime)
}

// DriverMeta : holds the information about the driver itself
type DriverMeta struct {
	Name     string
	Feature  string
	Services []string
	Data     interface{}
}

// DriverRuntime : the runtime information of the running driver
type DriverRuntime struct {
	meta        *DriverMeta
	data        interface{}
	dynamicData interface{}
	feature     string
	stream      chan dipper.Message
	input       io.ReadCloser
	output      io.WriteCloser
	driver      *Driver
	service     string
	Run         *exec.Cmd
}

// RoutedMessage : a service process a message and use the routed message to send to drivers
type RoutedMessage struct {
	driverRuntime *DriverRuntime
	message       *dipper.Message
}
