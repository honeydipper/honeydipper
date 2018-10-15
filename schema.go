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
	Driver     string                   `yaml:"driver,omitempty"`
	RawEvent   string                   `yaml:"rawevent,omitempty"`
	Conditions (interface{})            `yaml:"conditions,omitempty"`
	Fields     map[string](interface{}) `yaml:"fields,omitempty"`
	Source     Event                    `yaml:"source,omitempty"`
	Filters    []Filter                 `yaml:"filters,omitempty"`
}

// Function : is the datastructure hold the information to run actions
type Function struct {
	Driver     string                   `yaml:"driver,omitempty"`
	RawAction  string                   `yaml:"rawaction,omitempty"`
	Parameters map[string](interface{}) `yaml:"parameters,omitempty"`
	Results    map[string](interface{}) `yaml:"results,omitempty"`
	Target     Action                   `yaml:"target,omitempty"`
	Filters    []Filter                 `yaml:"filters,omitempty"`
}

// System : is an abstract construct to group data, trigger and function definitions
type System struct {
	Data      map[string](interface{}) `yaml:"data,omitempty"`
	Triggers  map[string]Trigger       `yaml:"triggers,omitempty"`
	Functions map[string]Function      `yaml:"functions,omitempty"`
}

// Condition : used to for conditioning in workflow
type Condition struct {
	Op     string `yaml:"op,omitempty"`
	Values []string
}

// Workflow : defines the steps, and relationship of the actions
type Workflow struct {
	Block      string
	Conditions []Condition `yaml:"conditions,omitempty"`
	Content    [](interface{})
}

// Rule : is a data structure defining what action to take upon an event
type Rule struct {
	When Trigger
	Do   Workflow
}

// RepoInfo : points a git repo where config data can be read from
type RepoInfo struct {
	Repo   string
	Branch string `yaml:"branch,omitempty"`
	Path   string `yaml:"path,omitempty"`
}

// ConfigSet : is a complete set of configuration at a specific moment
type ConfigSet struct {
	Systems  map[string]System      `yaml:"systems,omitempty"`
	Rules    map[string]Rule        `yaml:"rules,omitempty"`
	Drivers  map[string]interface{} `yaml:"drivers,omitempty"`
	Includes []string               `yaml:"includes,omitempty"`
	Repos    []RepoInfo             `yaml:"repos,omitempty"`
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
	name           string
	config         *Config
	driverRuntimes map[string]*DriverRuntime
	expects        map[string][]func(*dipper.Message)
	responders     map[string][]func(*DriverRuntime, *dipper.Message)
	transformers   map[string][]func(*DriverRuntime, *dipper.Message) *dipper.Message
	Route          func(*dipper.Message) []RoutedMessage
	expectLock     sync.Mutex
	driverLock     sync.Mutex
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
	meta    *DriverMeta
	data    *interface{}
	feature string
	input   int
	output  *io.WriteCloser
	driver  *Driver
	service string
	Run     *exec.Cmd
}

// RoutedMessage : a service process a message and use the routed message to send to drivers
type RoutedMessage struct {
	driverRuntime *DriverRuntime
	message       *dipper.Message
}
