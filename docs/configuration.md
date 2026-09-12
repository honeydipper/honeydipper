# Honeydipper Configuration Guide

<!-- toc -->

- [Topology and loading order](#topology-and-loading-order)
- [Data Set](#data-set)
- [Repos](#repos)
- [Drivers](#drivers)
  * [Daemon configuration](#daemon-configuration)
- [Systems](#systems)
- [Agents](#agents)
- [Workflows](#workflows)
- [Rules](#rules)
- [Contexts](#contexts)
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
	Agents    map[string]Agent       `json:"agents,omitempty"`
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
	InitFile    string
	Name        string
	Description string
	KeyFile     string
	KeyPassEnv  string

	TokenSource string
	Username    string
	PassEnv     string

	Options map[string]interface{}
}
```

To load a repo other than the bootstrap repo, just put info in the `repos` section like below.

```yaml
---
repos:
  - repo: <git url to the repo>
    branch: <optional, defaults to main>
    path: <the location of the init.yaml, must start with /, optional, defaults to />
    init_file: <the name of the init file, optional, defaults to init.yaml>
    keyFile: <deploy key used for cloning the repo, optional>
    keyPassEnv: <an environment variable name containing the passphrase for the deploy key, optional>
    token_source: <token source for HTTP authentication, optional>
    username: <username for HTTP authentication, optional>
    pass_env: <an environment variable name containing the password for HTTP authentication, optional>
    options:
      <key>: <value>  # arbitrary key-value pairs passed to load-time templates in the repo
  ...
```

### Load-time templates

Every config file is processed through a Go template engine before being parsed as YAML. The template delimiters are `{%` and `%}` (instead
of the usual `{{` and `}}`, which are reserved for run-time interpolation). The following data is available inside load-time templates:

| Variable | Description |
|---|---|
| `.env` | A map of all environment variables |
| `.local.filename` | The path of the current file relative to the repo root |
| `.local.repo` | The git URL of the current repo |
| `.local.branch` | The branch of the current repo |
| `.version` | The running Honeydipper version string |
| `.init.repo` | The git URL of the bootstrap (init) repo |
| `.init.branch` | The branch of the bootstrap (init) repo |
| `.options` | A map of options passed via the `options` field of the `repos` entry that loaded this repo |

Options are inherited: when repo A loads repo B with some options, those options are available to all files in repo B. If repo B in turn
declares repo C in its `repos` section, repo B's options are merged with any options declared on that entry (the entry's own options take
precedence on conflict) and passed down to repo C.

Example: conditionally loading content based on an option passed by the parent repo.

```yaml
{%- if .options.enable_feature_x %}
systems:
  feature_x:
    data:
      foo: bar
{%- end %}
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

### redis-cache

The `redis-cache` driver provides the `cache` feature used by workflow data
caching (see [Caching](./workflow.md#caching)). It stores cache entries in
Redis and supports both plain keys and Redis hashes (the `#` cache-key syntax
documented in the workflow guide).

#### Configuration

 - `data.per_field_expiration` *(boolean, default `false`)* &mdash; Controls how TTLs
   are applied to Redis hashes written by the `hset` RPC (used for `name#field`
   cache keys).

    - When `true`, each hash field gets its **own** expiration via Redis
      `HEXPIRE`. This requires **Redis >= 7.4** (per-field expiration was
      introduced in Redis 7.4); expired fields are removed automatically by
      Redis.
    - When `false` (the default), per-field `HEXPIRE` is **not** used.
      Instead the whole hash key receives a single shared TTL via `EXPIRE`,
      which is compatible with **Redis versions older than 7.4**.

   Plain (non-`#`) cache keys are always stored as whole keys and are unaffected
   by this option.

#### RPCs

The driver exposes the following RPCs, called internally by the workflow
caching engine through the `cache` feature:

 - `load` / `save` &mdash; read/write a whole plain cache key (the legacy, non-`#`
   path).
 - `hget` &mdash; `HGET key field`; returns the raw value of a single hash field (or
   empty on a miss).
 - `hgetall` &mdash; `HGETALL key`; returns a `map[string]any` of `field -> value`
   for the entire hash (or empty on a miss). Used by the `name#` whole-hash
   read-only cache path.
 - `hset` &mdash; write one or more hash fields with a TTL, applying per-field
   `HEXPIRE` or whole-hash `EXPIRE` according to the `per_field_expiration`
   option (used by the `name#field` cache path).

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

## Agents

Agents define AI model integrations that can be invoked from workflows using `call_agent` and `wait_agent` actions. Each agent maps to an
AI backend driver and configures its behavior including system prompts, tool access, and conversation management.

```go
// Agent defines an AI agent that can be invoked from workflows.
type Agent struct {
	Driver              string   `json:"driver,omitempty"`
	Engine              string   `json:"engine,omitempty"`
	SystemPrompt        string   `json:"system_prompt,omitempty"`
	InferencePrompt     string   `json:"inference_prompt,omitempty"`
	ShouldEmitThoughts  bool     `json:"should_emit_thoughts,omitempty"`
	ShouldStream        bool     `json:"should_stream,omitempty"`
	MaxHistoryLen       int      `json:"max_history_len,omitempty"`
	CompactionPolicy    CompactionPolicy `json:"compaction_policy,omitempty"`
	Tools               []string `json:"tools,omitempty"`
	ModelData           map[string]interface{} `json:"model_data,omitempty"`
}
```

### Agent Fields

| Field | Description |
|---|---|
| `driver` | The AI driver backend to use (e.g., `openai`, `gemini`, `ollama`) |
| `engine` | The model engine/name (e.g., `gpt-4`, `gemini-pro`) |
| `system_prompt` | The system prompt that defines the agent's role and behavior |
| `inference_prompt` | An optional additional prompt prepended at inference time |
| `should_emit_thoughts` | If true, the agent emits intermediate thinking/reasoning content |
| `should_stream` | If true, the response is streamed back as it is generated |
| `max_history_len` | Maximum number of messages to retain in conversation history (default: unlimited) |
| `compaction_policy` | Policy for compacting conversation history when it gets too long (see below) |
| `tools` | List of tools the agent can use. Tools are built from system functions (`sys_`), workflows (`wf_`), sub-agents (`ag_`), and MCP servers (`mcp_`) |
| `model_data` | Additional model-specific parameters (temperature, top_p, etc.) passed to the AI driver |

### Compaction Policy

When a conversation grows beyond token or history limits, the compaction policy controls how older messages are summarized to stay within bounds.

```go
type CompactionPolicy struct {
	Strategy            string `json:"strategy,omitempty"`
	Threshold           int    `json:"threshold,omitempty"`
	ThresholdType       string `json:"threshold_type,omitempty"`
	PreserveRecent      int    `json:"preserve_recent,omitempty"`
	SummarizationAgent  string `json:"summarization_agent,omitempty"`
	SummarizationPrompt string `json:"summarization_prompt,omitempty"`
}
```

| Field | Description |
|---|---|
| `strategy` | Compaction strategy, currently `"summarize"` — uses an LLM to condense older messages |
| `threshold` | When history length or token count exceeds this value, compaction is triggered |
| `threshold_type` | Either `"history_len"` (number of messages) or `"tokens"` (total token count) |
| `preserve_recent` | Number of recent messages to keep verbatim without summarization |
| `summarization_agent` | Name of another agent to use for summarization (defaults to the current agent) |
| `summarization_prompt` | Custom prompt for the summarization step |

### Example Agent Configuration

```yaml
---
agents:
  sre_assistant:
    driver: openai
    engine: gpt-4
    system_prompt: |
      You are an SRE assistant helping with incident response.
      Analyze the situation, suggest remediation steps, and be concise.
    should_stream: true
    max_history_len: 20
    compaction_policy:
      strategy: summarize
      threshold_type: history_len
      threshold: 20
      preserve_recent: 4
    model_data:
      temperature: 0.3
      max_tokens: 2000
```

### Using Agents in Workflows

See the [Workflow Composing Guide](./workflow.md) for details on `call_agent` and `wait_agent` actions. In summary:

* `call_agent: agent_name` — Invokes an agent and **waits** for its response. Use `with:` to pass `text` and other context.
* `wait_agent: session_id` — Waits for a previously invoked agent session response. Used when the agent was called in a detached context.
* `resume: resume_key` — Resumes a paused workflow session (e.g., paused by `wait_agent`).

For example, invoking an agent within a workflow:

```yaml
---
workflows:
  ask_sre:
    call_agent: sre_assistant
    with:
      text: "We have an alert for high CPU on production. What should I check?"
```

### Agent Tool Building

Agents can use tools from four sources. When tools are configured for an agent, Honeydipper automatically builds and registers them:

| Prefix | Source | Example |
|---|---|---|
| `sys_` | System functions | `sys_kubernetes__get_pod_status` |
| `wf_` | Named workflows | `wf_run_kubernetes` |
| `ag_` | Sub-agents (nested agent calls) | `ag_security_checker` |
| `mcp_` | MCP (Model Context Protocol) server tools | `mcp_filesystem__read_file` |

### Agent Session State

Agent conversation state is persisted to the cache (Redis-backed) with a default TTL of 72 hours. Each session tracks:

| State Field | Description |
|---|---|
| `history_len` | Number of messages in the conversation |
| `total_tokens` | Total tokens consumed so far |
| `convo_id` | Unique conversation identifier |

Sessions can be cancelled through the API (`/convos/:convoID/cancel`), and polling for responses uses a 9-second timeout per poll cycle.

### Agent Environment Variables

| Env Var | Purpose |
|---|---|
| `OPENAI_API_KEY` | API key for OpenAI driver |
| `GOOGLE_API_KEY` | API key for Gemini driver |
| `OLLAMA_BASE_URL` | Base URL for Ollama driver |

(Actual environment variables depend on the specific AI driver being used.)

## Workflows

See [Workflow Composing Guide](./workflow.md) for comprehensive workflow documentation.

A workflow definition supports the following additional fields documented here for reference:

### Export Control

| Field | Description |
|---|---|
| `export` | Data to export at end of workflow (always evaluated) |
| `export_on_success` | Data exported only when workflow succeeds |
| `export_on_failure` | Data exported only when workflow fails |
| `export_on_error` | Data exported only when workflow encounters an error |
| `no_export` | List of keys to block from export (`"*"` blocks all) |

The `*` suffix on export keys marks data that should be cached. For example, `cache-data*` is recognized as cacheable data.

### Error Handling on Steps and Threads

When using `steps` or `threads`, use these fields to control failure behavior:

| Field | Values | Default | Description |
|---|---|---|---|
| `on_failure` | `continue`, `exit` | `continue` | On step/thread failure |
| `on_error` | `continue`, `exit` | `exit` | On step/thread error |

For `threads`, `exit` means the workflow returns immediately without waiting for other threads to complete.

### Additional Simple Actions

These simple actions extend the set documented in the [Workflow Composing Guide](./workflow.md):

| Action | Value | Description |
|---|---|---|
| `call_agent` | Agent name | Invoke an AI agent and wait for its response |
| `wait_agent` | Session ID | Wait for a previously invoked agent session's response |
| `resume` | Resume key | Resume a paused workflow session |
| `detach` | Any value | Fire-and-forget; marks the child session as detached |

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

## Contexts

Contexts are named collections of key-value pairs that are injected into workflows at runtime. They are defined in the `contexts` section of a
`DataSet` and can be applied globally, per-system, per-trigger, or per-workflow.

### Built-in Contexts

| Context | Description |
|---|---|
| `_default` | Applied to all workflows unconditionally. Used for default configuration values. |
| `_event` | Applied dynamically based on the triggering event's system, driver, and raw event name (e.g., `_events.mySystem.myTrigger`). Used for hook definitions. |

### Custom Contexts

Custom contexts are user-defined and can contain any data, including hooks, default values, and configuration overrides. A workflow can use
the `context` field (single context name) or `contexts` field (list of context names) to apply them.

How contexts are resolved:

1. **Rule-level**: A rule can specify `context:` or `contexts:` in its `do` section
2. **Workflow-level**: A workflow can specify `context:` or `contexts:` directly
3. **Global hook contexts**: Contexts matching `_events.*` are applied based on event source

### Context Example

```yaml
---
contexts:
  # Applied to all workflows by default
  _default:
    chat_system: slack_bot
    notify:
      - "#ops-team"

  # Applied based on event source
  _events:
    opsgenie:
      alert:
        hooks:
          - on_success:
              - snooze_alert
          - on_failure:
              - escalate_alert

  # Custom context for production operations
  production_ops:
    allowed_channels:
      - "#production"
    require_approval: true
    context: opsgenie  # contexts can reference other contexts

rules:
  - when:
      source:
        system: opsgenie
        trigger: alert
    do:
      context: production_ops
      call_workflow: handle_incident
```

### Context Inheritance with Extends

Systems that use `extends` to inherit from parent systems also inherit context configurations. The child system's context values override
parent values. Context data is deeply merged for maps, replaced for other types (unless type-inconsistent, in which case the original data
is preserved).

### Context Data in Workflows

Context variables are available in the `ctx` namespace for interpolation within workflows:

```yaml
---
workflows:
  my_workflow:
    call_function: slack.say
    with:
      message: "Running in {{ .ctx.notify_pipeline | default 'default' }} mode"
```

For a complete list of contextual data sources and interpolation methods, see the [Workflow Composing Guide](./workflow.md#contextual-data).

## Config check

Honeydipper comes with a configcheck functionality that can help checking configuration validity before any updates
are committed or pushed to the git repos. It can also be used in the CI/CD pipelines to ensure the quality of the configuration files.

You can follow the [installation guide](./INSTALL.md) to install the Honeydipper binary or docker image, then use below commands to check
the local configuration files.

```bash
REPO=<path/to/local/files> honeydipper configcheck
```

If using a docker image:

```bash
docker run -it -v <path/to/config>:/config -e REPO=/config honeydipper/honeydipper configcheck
```

If your local config loads remote git repos and you want to validate them too, use `CHECK_REMOTE` environment variable.

```bash
REPO=<path/to/config> CHECK_REMOTE=1 honeydipper configcheck
```

If using docker image:

```bash
docker run -it -v <path/to/config>:/config -e REPO=/config -e CHECK_REMOTE=1 honeydipper/honeydipper configcheck
```

You can also use `-h` option to see a full list of supported environment variables.

## References

For a list of available drivers, systems, and workflows that you can take advantage of immediately, see the reference here.

 * [Honeydipper config essentials](../essentials.html)
