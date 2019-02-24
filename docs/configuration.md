# Honeydipper Configuration Guide

<!-- toc -->

- [Topology and loading order](#topology-and-loading-order)
- [Data Set](#data-set)
- [Repos](#repos)
- [Drivers](#drivers)
  * [Daemon configuration](#daemon-configuration)
- [Systems](#systems)
- [Workflows](#workflows)
- [Rules](#rules)
- [References](#references)

<!-- tocstop -->

## Topology and loading order

As mentioned in the [Architecture/Design](../README.md), Honeydipper requires almost no local configuration to bootstrap, only requries a few environment variable to point it towards the git repo where the bootstrap configurations are loaded from. The bootstrap repo can load other repos using `repos` section in any of the loaded yaml files. Inside every repo, Honeydipper will first load the `init.yaml`, and then load all the yaml files under `includes` section. Any of the files can also use a `includes` section to load even more files, and so on.

Inside every repo, when loading files, an including file will be loaded after all the files that it includes are loaded. So the including file can override anything in the included files. Similarly, repos are loaded after their dependency repos, so they can override anything in the depended repo.

One of the key selling point of Honeydipper is the ability to reuse and share. The drivers, systems, workflows and rules can all be packaged into repos then shared among projects, teams and organizations. Overtime, we are expecting to see a number of reusable public config repos contributed and maintained by communities. The seed of the repos is the [honeydipper-config-essentials](https://github.com/honeydipper/honeydipper-config-essentials) repo, and the reference document can be found at [here](https://honeydipper.github.io/honeydipper-config-essentials/).

## Data Set

Every file contains a `DataSet`. In the end, all the files in all the repos will be merged into a final `DataSet`. A `DataSet` should contain some of the below mentioned sections. See [a sample config file](../configs/sample-init.yaml) for examples.

```go
// DataSet is a subset of configuration that can be assembled to the complete final configuration.
type DataSet struct {
	Systems   map[string]System      `json:"systems,omitempty"`
	Rules     []Rule                 `json:"rules,omitempty"`
	Drivers   map[string]interface{} `json:"drivers,omitempty"`
	Includes  []string               `json:"includes,omitempty"`
	Repos     []RepoInfo             `json:"repos,omitempty"`
	Workflows map[string]Workflow    `json:"workflows,omitempty"`
}
```

While it is possible to fit everything into a single file, it is recommended to organzie your configurations into smaller chunks in a way that each chunk contains only relevant settings. For example, a file can define just a system and all its functions and triggers. Or, a file can define all the infomation about a driver. Another example would be to define a workflow in a file separately.

## Repos

Repos are defined like below.

```go
// RepoInfo points to a git repo where config data can be read from.
type RepoInfo struct {
	Repo   string
	Branch string `json:"branch,omitempty"`
	Path   string `json:"path,omitempty"`
}
```

To load a repo other than the bootstrap repo, just put info in the `repos` section like below.

```yaml
---
repos:
  - repo: <git url to the repo>
    branch: <optional, defaults to master>
    path: <the location of the init.yaml, must starts with /, optional, defaults to />
  ...
```

## Drivers

`drivers` section provides driver specific config data, such as webhook listening port, redis connections etc. It is a map from the names of the drivers to their data. The data element and structure of the driver data is only meaningful to the driver itself.  Honeydipper just pass the data as-is, a `map[string]interface{}` in `go`.

### Daemon configuration

Note that, `daemon` configuration is loaded and passed as a driver in this section.

```yaml
---
drivers:
  daemon:
    loglevel: <one of INFO, DEBUG, WARNING, ERROR>
    featureMap:  # map of services to their defined features
      global:    # all services will recognize these features
        emitter: datadog-emitter
        eventbus: redisqueue
      operator:
        ...
      receiver:
        ...
      engine:
        ...
    features:   # the features to be loaded, mapped features won't be loaded unless they are listed here
      global:
        - name: eventbus
          required: true  # will be loaded before other driver, and will rollback if this fails during config changes
        - name: emitter
        - name: driver:gcloud-kms  # no feature name, just use the driver: prefix
          required: true
      operator:
        - name: driver:gcloud-gke
        ...
```

## Systems

As defined, systems are a group of triggers and actions and some data that can be re-used.

```go
// System is an abstract construct to group data, trigger and function definitions.
type System struct {
	Data      map[string](interface{}) `json:"data,omitempty"`
	Triggers  map[string]Trigger       `json:"triggers,omitempty"`
	Functions map[string]Function      `json:"functions,omitempty"`
	Extends   []string                 `json:"extends,omitempty"`
}

// Trigger is the datastructure hold the information to match and process an event.
type Trigger struct {
	Driver     string      `json:"driver,omitempty"`
	RawEvent   string      `json:"rawevent,omitempty"`
	Conditions interface{} `json:"conditions,omitempty"`
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
```

A system can extend another system to inherit data, triggers and functions, and then can override any of the inherited data with its own definition.  We can create some abstract systems that contains part of the data that can be shared by multiple child systems. A `Function` can either be defined using `driver` and `rawAction` or inherit definition from another `Function` by specifying a `target`. Similarly, a `Trigger` can be defined using `driver` and `rawEvent` or inherit definition from another `Trigger` using `source`.

See the [sample file](../configs/sample-init.yaml) for examples.

## Workflows

See [Workflow Composing Guide](./workflow.md) for details on workflows.

## Rules

Here is the definition:

```go
// Rule is a data structure defining what action to take when certain event happen.
type Rule struct {
	When Trigger
	Do   Workflow
}
```

Refer to the Systems section for the definition of `Trigger`, and see [Workflow Composing Guide](./workflow.md) for workflows.

## References

For a list of available drivers, systems, and workflows that you can take advantage of immediately, see the reference here.

 * [Honeydipper config essentials](https://honeydipper.github.io/honeydipper-config-essentials/)

