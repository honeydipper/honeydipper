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
- [Config check](#config-check)
- [References](#references)

<!-- tocstop -->

## Topology and loading order

As mentioned in the [Architecture/Design](../README.md), Honeydipper requires very little local configuration to bootstrap; it only requires a
few environment variables to point it towards the git repo from which the bootstrap configurations are loaded. The bootstrap repo can load
other repos using the `repos` section in any of the loaded yaml files. Inside every repo, Honeydipper will first load the `init.yaml`, and
then load all the yaml files under `includes` section. Any of the files can also use a `includes` section to load even more files, and so
on.

Inside every repo, when loading files, an including file will be loaded after all the files that it includes are loaded. So the including
file can override anything in the included files. Similarly, repos are loaded after their dependency repos, so they can override anything in
the depended repo.

One of the key selling point of Honeydipper is the ability to reuse and share. The drivers, systems, workflows and rules can all be packaged
into repos then shared among projects, teams and organizations. Over time, we are expecting to see a number of reusable public config repos
contributed and maintained by communities. The seed of the repos is the
[honeydipper-config-essentials](https://github.com/honeydipper/honeydipper-config-essentials) repo, and the reference document can be found
[here](https://honeydipper-sphinx.readthedocs.io/en/latest/essentials.html).

## Data Set

`DataSet` is the building block of Honeydipper config. Every configuration file contains a `DataSet`. Once all files are loaded, all the
`DataSet` will be merged into a final `DataSet`. A `DataSet` is made up with one or more sections listed below.

```go
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
```

While it is possible to fit everything into a single file, it is recommended to organize your configurations into smaller chunks in a way
that each chunk contains only relevant settings. For example, a file can define just a system and all its functions and triggers. Or, a file
can define all the information about a driver. Another example would be to define a workflow in a file separately.

## Repos

Repos are defined like below.

```go
// RepoInfo points to a git repo where config data can be read from.
type RepoInfo struct {
	Repo        string
	Branch      string
	Path        string
	Name        string
	Description string
	KeyFile     string
	KeyPassEnv  string
}
```

To load a repo other than the bootstrap repo, just put info in the `repos` section like below.

```yaml
---
repos:
  - repo: <git url to the repo>
    branch: <optional, defaults to master>
    path: <the location of the init.yaml, must starts with /, optional, defaults to />
    keyFile: <deploy key used for cloning the repo, optional>
    keyPassEnv: <an environment variable name containing the passphrase for the deploy key, optional>
  ...
```

## Drivers

The `drivers` section provides driver specific config data, such as webhook listening port, Redis connections etc. It is a map from the
names of the drivers to their data. The data element and structure of the driver data is only meaningful to the driver itself. Honeydipper
just passes the data as-is, a `map[string]interface{}` in `go`.

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

A system can extend another system to inherit data, triggers and functions, and then can override any of the inherited data with its own
definition.  We can create some abstract systems that contains part of the data that can be shared by multiple child systems. A `Function`
can either be defined using `driver` and `rawAction` or inherit definition from another `Function` by specifying a `target`. Similarly, a
`Trigger` can be defined using `driver` and `rawEvent` or inherit definition from another `Trigger` using `source`.

For example, inheriting the `kubernetes` system to create an instance of `kubernetes` cluster.

```yaml
---
systems:
  my-k8s-cluster:
    extends:
      - kubernetes
    data:
      source:
        type: gcloud-gke
        project: myproject
        location: us-west1-a
        cluster: mycluster
        service_account: ENC[gcloud-kms,...masked...]
```

You can then use `my-k8s-cluster.recycleDeployment` function in workflows or rules to recycle deployments in the cluster. Or, you can pass
`my-k8s-cluster` to `run_kubernetes` workflow as `system` context variable to run jobs in that cluster.

Another example would be to extend the `slack_bot` system, to create another instance of slack integration.

```yaml
---
systems:
  slack_bot: # first slack bot integration
    data:
      token: ...
      slash_token: ...
      interact_token: ...

  my_team_slack_bot: # second slack bot integration
    extends:
      - slack_bot
    data:
      token: ...
      slash_token: ...
      interact_token: ...

rules:
  - when:
      source:
        system: my_team_slack_bot
        trigger: slashcommand
    do:
      call_workflow: my_team_slashcommands
```

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

## Config check

Honeydipper 0.1.8 and above comes with a configcheck functionality that can help checking configuration validity before any updates
are committed or pushed to the git repos. It can also be used in the CI/CD pipelines to ensure the quality of the configuration files.

You can follow the [installation guide](./INSTALL.md) to install the Honeydipper binary or docker image, then use below commands to check
the local configuration files.

```bash
REPO=</path/to/local/files> honeydipper configcheck
```

If using a docker image

```bash
docker run -it -v </path/to/config>:/config -e REPO=/config honeydipper/honeydipper:x.x.x configcheck
```

If your local config loads remote git repos and you want to validate them too, use `CHECK_REMOTE` environment variable.

```bash
REPO=</path/to/config> CHECK_REMOTE=1 honeydipper configcheck
```

If using docker image

```bash
docker run -it -v </path/to/config>:/config -e REPO=/config -e CHECK_REMOTE=1 honeydipper/honeydipper:x.x.x configcheck
```

You can also use `-h` option to see a full list of supported environment variables.

## References

For a list of available drivers, systems, and workflows that you can take advantage of immediately, see the reference here.

 * [Honeydipper config essentials](../essentials.html)

