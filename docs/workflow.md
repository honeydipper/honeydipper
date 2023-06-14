# Workflow Composing Guide

<!-- toc -->

- [Composing Workflows](#composing-workflows)
  * [Simple Actions](#simple-actions)
  * [Complex Actions](#complex-actions)
  * [Iterations](#iterations)
  * [Conditions](#conditions)
  * [Looping](#looping)
  * [Hooks](#hooks)
- [Contextual Data](#contextual-data)
  * [Sources](#sources)
  * [Interpolation](#interpolation)
  * [Merging Modifier](#merging-modifier)
- [Essential Workflows](#essential-workflows)
  * [`notify`](#notify)
  * [`workflow_announcement`](#workflow_announcement)
  * [`workflow_status`](#workflow_status)
  * [`send_heartbeat`](#send_heartbeat)
  * [`snooze_alert`](#snooze_alert)
- [Running a Kubernetes Job](#running-a-kubernetes-job)
  * [Basic of `run_kubernetes`](#basic-of-run_kubernetes)
  * [Environment Variables and Volumes](#environment-variables-and-volumes)
  * [Predefined Step](#predefined-step)
  * [Expanding `run_kubernetes`](#expanding-run_kubernetes)
  * [Using `run_kubernetes` in GKE](#using-run_kubernetes-in-gke)
- [Slash Commands](#slash-commands)
  * [Predefined Commands](#predefined-commands)
  * [Adding New Commands](#adding-new-commands)
  * [Mapping Parameters](#mapping-parameters)
  * [Messages and notifications](#messages-and-notifications)
  * [Secure the commands](#secure-the-commands)

<!-- tocstop -->

*DipperCL* is the control language that `Honeydipper` uses to configure data, assets and logic for its operation. It is basically a YAML with a `Honeydipper` specific schema.

## Composing Workflows
`workflow` defines what to do and how to perform when an event is triggered. `workflow` can be defined in rules directly in the `do` section, or it can be defined independently with a name so it can be re-used/shared among multiple rules and workflows. A `workflow` can be as simple as invoking a single driver `rawAction`. It can also contains complicate logics, procedures dealing with various scenarios. All `workflows` are built with the same building blocks, follow the same process, and they can be stacked/combined with each other to achieve more complicated goals.

An example of a workflow defined in a rule calling an `rawAction`:

```yaml
---
rules:
  - when:
      driver: webhook
      conditions:
        url: /test1
    do:
      call_driver: redispubsub.broadcast
      with:
        subject: internal
        channel: foo
        key: bar
```

An example of named workflow that can be invoked from other workflows.

```yaml
---
workflows:
  foo:
    call_function: example.execute
    with:
      key1: val2
      key2: val2

rules:
  - when:
      source:
        system: example
        trigger: happened
    do:
      call_workflow: foo
```

### Simple Actions
There are 4 types of simple actions that a workflow can perform.

 - `call_workflow`: calling out to another named workflow, taking a string specifying the name of the workflow
 - `call_function`: calling a predefined system function, taking a string in the form of `system.function`
 - `call_driver`: calling a `rawAction` offered by a driver, taking a string in the form of `driver.rawAction`
 - `wait`: wait for the specified amount of time or receive a wake-up request with a matching token. The time should be formatted according to the requirement for function [ParseDuration](https://golang.org/pkg/time/#ParseDuration). A unit suffix is required.

They can not be combined.

A function can also have no action at all. `{}` is a perfectly legit no-op workflow.

### Complex Actions
Complex actions are groups of multiple `workflows` organized together to do some complex work.

 - `steps`: an array of child workflows that are executed in sequence
 - `threads`: an array of child workflows that are executed in parallel
 - `switch/cases/default`: taking a piece of contextual data specified in `switch`, chose and execute from a map of child workflows defined in `cases` or execute the child workflow defined in `default` if no branch matches

These can not be combined with each other or with any of the simple actions.

When using `steps` or `threads`, you can control the behaviour of the workflow upon `failure` or `error` status through fields `on_failure` or `on_error`.  The allowed values are `continue` and `exit`. By default, `on_failure` is set to `continue` while `on_error` is set to `exit`. When using `threads`, `exit` means that when one thread returns error, the workflow returns without waiting for other threads to return.

### Iterations
Any of the actions can be combined with an `iterate` or `iterate_parallel` field to be executed multiple times with different values from a list. The current element of the list will be stored in a local contextual data item named `current`. Optionally, you can also customize the name of contextual data item using `iterate_as`. The elements of the lists to be iterated don't have to be simple strings, it can be a map or other complex data structures.

For example:

<!-- {% raw %} -->
```yaml
---
workflows:
  foo:
    iterate:
      - name: Peter
        role: hero
      - name: Paul
        role: villain
    call_workflow: announce
    with:
      message: '{{ .ctx.current.name }} is playing the role of `{{ .ctx.current.role }}`.'
```
<!-- {% endraw %} -->

### Conditions
We can also specify the conditions that the workflow checks before taking any action.

 - `if_match/unless_match`: specify the skeleton data to match the contextual data
 - `if/unless/if_any/unless/unless_all`: specify the list of strings that interpolate to truy/falsy values

Some examples for using skeleton data matching:
```yaml
---
workflows:
  do_foo:
    if_match:
      foo: bar
    call_workflow: do_something

  do_bar:
    unless_match:
      team: :regex:engineering-.*
    call_workflow: complaint
    with:
      message: Only engineers are allowed here.

  do_something:
    if_match:
      user:
        - privileged_user1
        - privileged_user2
    call_workflow: assert
    with:
      message: you are either privileged_user1 or priviledged_user2

  do_some_other-stuff:
    if_match:
      user:
        age: 13
    call_workflow: assert
    with:
      message: .ctx.user matchs a data strucure with age field equal to 13
```

Please note how we use regular expression, list of options to match the contextual data, and how to match a field deep into the data structure.

Below are some examples of using list of conditions:
<!-- {% raw %} -->
```yaml
---
workflows:
  run_if_all_meets:
    if:
      - $ctx.exits # ctx.exits must not be empty and not one of such strings `false`, `nil`, `{}`, `[]`, `0`. 
      - $ctx.also  # ctx.also must also be truy
    call_workflow: assert
    with:
      message: `exits` and `also` are both truy

  run_if_either_meets:
    if_any:
      - '{{ empty .ctx.exists | not }}'
      - '{{ empty .ctx.also | not }}'
    call_workflow: assert
    with:
      message: at least one of `exits` or `also` is not empty
```
<!-- {% endraw %} -->

### Looping
We can also repeat the actions in the workflow through looping fields

 - `while`: specify a list of strings that interpolate into truy/falsy values
 - `until`: specify a list of strings that interpolate into truy/falsy values
    
For example:

<!-- {% raw %} -->
```yaml
---
workflows:
  retry_func: # a simple forever retry
    on_error: continue
    on_failure: exit
    with:
      success: false
    until:
      - $ctx.success
    steps:
      - call_function: $ctx.func
      - export:
          success: '{{ eq .labels.status "success" }}'
    no_export:
      - success

  retry_func_count_with_exp_backoff:
    on_error: continue
    on_failure: exit
    with:
      success: false
      backoff: 0
      count-: 2
    until:
      - $ctx.success
      - $ctx.count
    steps:
      - if:
          - $ctx.backoff
        wait: '{{ .ctx.backoff }}s'
      - call_function: $ctx.func
      - export:
          count: '{{ sub (int .ctx.count) 1 }}'
          success: '{{ eq .labels.status "success" }}'
          backoff: '{{ .ctx.backoff | default 10 | int | mul 2 }}'
    no_export:
      - success
      - count
      - backoff
```
<!-- {% endraw %} -->

### Hooks
Hooks are child workflows executed at a specified moments in the parent workflow's lifecycle. It is a great way to separate auxiliary work, such as sending heartbeat, sending slack messages, making an announcement, clean up, data preparation etc., from the actual work. Hooks are defined through context data, so it can be pulled in through predefined contexts, which makes the actual workflow seems less cluttered.

For example,
```yaml
contexts:
  _events:
    '*':
      hooks:
        - on_first_action: workflow_announcement
  opsgenie:
    '*':
      hooks:
        - on_success:
            - snooze_alert

rules:
  - when:
      source:
        system: foo
        trigger: bar
    do:
      call_workflow: do_something

  - when:
      source:
        system: opsgenie
        trigger: alert
    do:
      context: opsgenie
      call_workflow: do_something
```
In the above example, although not specifically spelled out in the rules, both events will trigger the execution of `workflow_announcement` workflow before executing the first action. And if the workflow responding to the `opsgenie.alert` event is successful, `snooze_alert` workflow will be executed.

The supported hooks:

 * on_session: when a workflow session is created, even `{}` no-op session will trigger this hook
 * on_first_action: before a workflow performs first simple action
 * on_action: before performs each simple action in `steps`
 * on_item: before execute each iteration
 * on_success: before workflow exit, and when the workflow is successful
 * on_failure: before workflow exit, and when the workflow is failed
 * on_error: before workflow exit, and when the workflow ran into error
 * on_exit: before workflow exit

## Contextual Data
Contextual data is the key to stitch different events, functions, drivers and workflows together.

### Sources
Every workflow receives contextual data from a few sources:

 * Exported from the event
 * Inherit context from parent workflow
 * Injected from predefined context, `_default`, `_event` and contexts listed through `context` or `contexts`
 * Local context data defined in `with` field
 * Exported from previous steps of the workflow

Since the data are received in that particular order listed above, the later source can override data from previous sources. Child workflow context data is independent from parent workflow, anything defined in `with` or inherited will only be in effect during the life cycle of current workflow, except the exported data. Once a field is exported, it will be available to all outer workflows. You can override this by specifying the list of fields that you don't want to export.

Pay attention to the example `retry_func_count_with_exp_backoff` in the previous section. In order to not contaminate parent context with temporary fields, we use `no_export` to block the exporting of certain fields. For example

The `with` field in the workflow can be a map or a list of maps. If it is a map, each key defines a variable. If it is a list of maps, each map is a layer. The layers are processed in the order they appear. The variables defined in previous layer can be used to define values in later layers.
```yaml
---
call_workflow: something
with:
  - var1: initial value
    var2: val2
    foo:
      - bar
  - var3: '{{ .ctx.var2 }}, {{ .ctx.var1 }}'
    foo+:
      - another bar
```

The final value for `var3` will be `initial value, val2`, and the final value of list `foo` will contain both `bar` and `another bar`.

### Interpolation
We can use interpolation in workflows to make the workflow flexible and versatile. You can use interpolation in most of the fields of a workflow. Besides contextual data, other data available for interpolation includes:

 * `labels` - string values attached to latest received dipper Message indicating session status, IDs, etc.,
 * `ctx` - contextual data,
 * `event` - raw unexposed event data from the original event that triggered the workflow
 * `data` - raw unexposed payload from the latest received dipper message

It is recommended to avoid using `event` and `data` in workflows, and stick to `ctx` as much as possible. The raw unexposed data might eventually be deprecated and hidden. They may still be available in `system` definition.

*DipperCL* provides following ways of interpolation:

 * **path interpolation** - comma separated multiple paths following a dollar sign, e.g. `$ctx.this,ctx.that,ctx.default`, cannot be mixed in strings. can specify a default value using either single, double or tilde quotes if none of the keys are defined in the context, e.g. `$ctx.this,ctx.that,"this value is the default"`. Also, can use `?` following the `$` to indicate that nil value is allowed.
 * **inline go template** - strings with go templates that get rendered at time of the workflow execution, requires quoting if template is at the start of the string
 * **yaml parser** - a string following a `:yaml:` prefix, will be parsed at the time of the execution, can be combined with go template
 * **e-yaml encryption** - a string with `ENC[` prefix, storing base64 encoded encrypted content
 * **file attachment** - a relative path following a `@:` prefix, requires quoting

See [interpolation guide](./interpolation.html) for detail on how to use interpolation.

### Merging Modifier
When data from different data source is merged, by default, map structure is deeply merged, while all other type of data with the same name is replaced by the newer source. One exception is that if the data in the new source is not the same type of the existing data, the old data stays in that case.

For example, undesired merge behaviour:
```yaml
---
workflows
  merge:
    - export:
        data: # original
          foo: bar
          foo_map:
            key1: val1
          foo_list:
            - item1
            - item2
          foo_param: "a string"
    - export:
        data: # overriding
          foo: foo
          foo_map:
            key2: val2
          foo_list:
            - item3
            - item4
          foo_param: # type inconsistent
            key: val
```
After merging with the second step, the final exported data will be like below. Notice the fields that are replaced.
```yaml
data: # final
  foo: foo
  foo_map:
    key1: val1
    key2: val2
  foo_list:
    - item3
    - item4
  foo_param: "a string"
```

We can change the behaviour by using merging modifiers at the end of the overriding data names.

Usage:

`var` is an example name of the overriding data, the following character indicates what type of merge modifier to use.
 * `var-`: only use the new value if the `var` is not already defined and not nil
 * `var+`: if the `var` is a list or string, the new value will be appended to the existing values
 * `var*`: forcefully override the value

Note that, the merging modifier works in layers too. See previous example for details.

## Essential Workflows

We have made a few helper workflows available in the `honeydipper-config-essentials` repo. Hopefully, they will make it easier for you to write your own workflows.

### `notify`

Sending a chat message using configured system. The chat system can be anything that provides a `say` and a `reply` function.

*Required context fields*
 * `chat_system`: system used for sending the message, by default `slack_bot`
 * `message`: the text to be sent, do your own formatting
 * `message_type`: used for formatting/coloring and select recipients
 * `notify`: a list of recipients, slack channel names if using `slack_bot`
 * `notify_on_error`: a list of additional recipients if `message_type` is `error` or `failure`

### `workflow_announcement`

This workflow is intended to be invoked through `on_first_action` hook to send a chat message to announce what will happen.

*Required context fields*
 * `chat_system`: system used for sending the message, by default `slack_bot`
 * `notify`: a list of recipients, slack channel names if using `slack_bot`
 * `_meta_event`: every events export a `_meta_event` showing the driver name and the trigger name, can be overridden in trigger definition
 * `_event_id`: if you export a `_event_id` in your trigger definition, it will be used for display, by default it will be `unspecified`
 * `_event_url`: the display of the `_event_id` will be a link to this url, by default `http://honeydipper.io`
 * `_event_detail`: if specified, will be displayed after the brief announcement
 
Besides the fields above, this workflow also uses a few context fields that are set internally from host workflow(not the hook itself) definition.
 * `_meta_desc`: the `description` from the workflow definition
 * `_meta_name`: the `name` from the workflow definition
 * `performing`: what the workflow is currently performing

### `workflow_status`

This workflow is intended to be invoked through `on_exit`, `on_error`, `on_success` or `on_failure`.
*Required context fields*
 * `chat_system`: system used for sending the message, by default `slack_bot`
 * `notify`: a list of recipients, slack channel names if using `slack_bot`
 * `notify_on_error`: a list of additional recipients if `message_type` is `error` or `failure`
 * `status_detail`: if available, the detail will be attached to the status notification message

Besides the fields above, this workflow also uses a few context fields and `labels` that are set internally from host workflow(not the hook itself).
 * `_meta_desc`: the `description` from the workflow definition
 * `_meta_name`: the `name` from the workflow definition
 * `performing`: what the workflow is currently performing
 * `.labels.status`: the latest function return status
 * `.labels.reason`: the reason for latest failure or error
 
### `send_heartbeat`

This workflow can be used in `on_success` hooks or as a stand-alone step. It sends a heartbeat to the alerting system

*Required context fields*
 * `alert_system`: system used for sending the heartbeat, can be any system that implements a `heartbeat` function, by default `opsgenie`
 * `heatbeat`: the name of the heartbeat

### `snooze_alert`

This workflow can be used in `on_success` hooks or as a stand-alone step. It snooze the alert that triggered the workflow.
 * `alert_system`: system used for sending the heartbeat, can be any system that implements a `snooze` function, by default `opsgenie`
 * `alert_Id`: the ID of the alert

## Running a Kubernetes Job

We can use a predefined `run_kubernetes` workflow from `honeydipper-config-essentials` repo to run kubernetes jobs. A simple example is below

```yaml
---
workflows:
  kubejob:
    run_kubernetes:
      system: samplecluster
      steps:
        - type: python
          command: |
            ...python script here...
        - type: bash
          shell: |
            ...shell script here...
```

### Basic of `run_kubernetes`

`run_kubernetes` workflow requires a `system` context field that points to a predefined system. The system must be extended from `kubernetes` system so that it has `createJob`, `waitForJob` and `getJobLog` function defined. The predefined system should also have the information required to connect to the kubernetes cluster, the namespace to use etc.

The required `steps` context field should tell the workflow what containers to define in the kubernetes job.  If there are more that one step, the steps before the last step are all defined in `initContainters` section of the pod, and the last step is defined in `containers`.

Each step of the job has its type, which defines what docker image to use. The workflow comes with a few types predefined.
 * python
 * python2
 * python3
 * node
 * bash
 * gcloud
 * tf
 * helm
 * git

A `step` can be defined using a `command` or a `shell`. A `command` is a string or a list of strings that are passed to the default entrypoint using `args` in the container spec. A `shell` is a string or a list of strings that passed to a customized shell script entrypoint.

For example

```yaml
---
workflows:
  samplejob:
    run_kubernetes:
      system: samplecluster
      steps:
        - type: python3
          command: 'print("hello world")'
        - type: python3
          shell: |
            cd /opt/app
            pip install -r requirements.txt
            python main.py
```

The first step uses the `command` to directly passing a python command or script to the container, while the second step uses `shell` to run a script using the same container image.

There is a shared `emptyDir` volumes mounted at `/honeydipper` to every step, so that the steps can use the shared storage to pass on information. One thing to be noted is that the steps don't honour the default `WORKDIR` defined in the image, instead all the steps are using `/honeydipper` as `workingDir` in the container `spec`. This can be customized using `workingDir` in the step definition itself.

The workflow will return `success` in `.labels.status` when the job finishes successfully. If it fails to create a job or fails to get the status or job output, the status will be `error`. If the job is created, but failed to complete or return non-zero status code, the `.labels.status` will be set to `failure`. The workflow will export a `log` context field that contains a map from pod name to a map of container name to log output. A simple string version of the output that contains all the concatenated logs are exported as `output` context field.

### Environment Variables and Volumes

You can define environments and volumes to be used in each step or as a global context field to share them across steps. For example,

```yaml
---
workflows:
  samplejob:
    run_kubernetes:
      system: samplecluster
      env:
        - name: CLOUDSDK_CONFIG
          value: /honeydipper/.config/gcloud
      steps:
        - git-clone
        - type: gcloud
          shell: |
            gcloud auth activate-service-account $GOOGLE_APPLICATION_ACCOUNT --key-file=$GOOGLE_APPLICATION_CREDENTIALS
          env:
            - name: GOOGLE_APPLICATIION_ACCOUNT
              value: sample-service-account@foo.iam.gserviceaccount.com
            - name: GOOGLE_APPLICATION_CREDENTIALS
              value: /etc/gcloud/service-account.json
          volumes:
            - mountPath: /etc/gcloud
              volume:
                name: credentials-volume
                secret:
                  defaultMode: 420
                  secretName: secret-gcloud-service-account
                
        - type: tf
          shell: |
            terraform plan -no-color
```

Please note that, the `CLOUDSDK_CONFIG` environment is shared among all the steps. This ensures that all steps use the same gcloud configuration directory. The volume definition here is a combining of `volumes` and `volumeMounts` definition from pod `spec`.

### Predefined Step

To make writing kubernetes job workflows easier, we have created a few `predefined_steps` that you can use instead of writing your own from scratch. To use the `predefined_step`, just replace the step definition with the name of the step. See the example from the previous section, where the first step of the job is `git-clone`.

 * `git-clone`

This step clones the given repo into the shared volume `/honeydipper/repo` folder. It requires that the `system` contains a few field to identify the repo to be cloned. That includes:

 * `git_url` - the url of the repo
 * `git_key_secret` - if a key is required, it should be present in the kubernetes cluster as a secret
 * `git_ref` - branch

We can also use the predefined step as a skeleton to create our steps by overriding the settings. For example,

```yaml
---
workflows:
  samplejob:
    run_kubernetes:
      system: samplecluster
      steps:
        - use: git-clone
          volumes: [] # no need for secret volumes when cloning a public repo
          env:
            - name: REPO
              value: https://github.com/honeydipper/honeydipper
            - name: BRANCH
              value: DipperCL
        - ...
```

Pay attention to `use` field of the step.

### Expanding `run_kubernetes`

If `run_kubernetes` only supports built-in types or predefined steps, it won't be too useful in a lot of places. Luckily, it is very easy to expand the workflow to support more things.

To add a new step type, just extend the `_default` context under `start_kube_job` in the `script_types` field.

For example, to add a type with the `rclone` image,

```yaml
---
contexts:
  _default:
    start_kube_job:
      script_types:
        rclone:
          image: kovacsguido/rclone:latest
          command_prefix: []
          shell_entry: [ "/bin/ash", "-c" ]
```
Supported fields in a type:

 * `image` - the image to use for this type
 * `shell_entry` - the customized entrypoint if you want to run shell script with this image
 * `shell_prefix` - a list of strings to be placed in `args` of the container `spec` before the actual `shell` script
 * `command_entry` - in case you want to customize the entrypoint for using `command`
 * `command_prefix` - a list of strings to be placed in `args` before `command`

Similarly, to add a new predefined step, extend the `predefined_steps` field in the same place.

For example, to add a rclone step

<!-- {% raw %} -->
```yaml
---
contexts:
  _default:
    start_kube_jobs:
      predefined_steps:
        rclone:
          name: backup-replicate
          type: rclone
          command:
            - copy
            - --include
            - '{{ coalesce .ctx.pattern (index (default (dict) .ctx.patterns) (default "" .ctx.from)) "*" }}'
            - '{{ coalesce .ctx.source (index (default (dict) .ctx.sources) (default "" .ctx.from)) }}'
            - '{{ coalesce .ctx.destination (index (default (dict) .ctx.destinations) (default "" .ctx.to)) }}'
          volumes:
            - mountPath: /root/.config/rclone
              volume:
                name: rcloneconf
                secret:
                  defaultMode: 420
                  secretName: rclone-conf-with-ca
```
<!-- {% endraw %} -->

See [Defining steps](#basic-of-run_kubernetes) on how to define a step

### Using `run_kubernetes` in GKE

GKE is a google managed kubernetes cluster service. You can use `run_kubernetes` to run jobs in GKE as you would any kubernetes cluster. There are a few more helper workflows, predefined steps specifically for GKE.

 * **`use_google_credentials` workflow**

If the context variable `google_credentials_secret` is defined, this workflow will add a step in the `steps` list to activate the service account. The service account must exist in the kubernetes cluster as a secret, the service account key can be specified using `google_credentials_secret_key` and defaults to `service-account.json`. This is a great way to run your job with a service account other than the default account defined through the GKE node pool. This step has to be executed before you call `run_kubernetes`, and the following `steps` in the job have to be added through [append modifier](#merging-modifier).

For example:
<!-- {% raw %} -->
```yaml
---
workflows:
  create_cluster:
    steps:
      - call_workflow: use_google_credentials
      - call_workflow: run_kubernetes
        with:
          steps+: # using append modifier here
            - type: gcloud
              shell: gcloud container clusters create {{ .ctx.new_cluster_name }}
```
<!-- {% endraw %} -->

 * **`use_gcloud_kubeconfig` workflow**

This workflow is used for adding a step to run `gcloud container clusters get-credentials` to fetch the kubeconfig data for GKE clusters. This step requires that the `cluster` context variable is defined and describing a GKE cluster with fields like `project`, `cluster`, `zone` or `region`.

For example:
<!-- {% raw %} -->
```yaml
---
workflows:
  delete_job:
    with:
      cluster:
        type: gke # specify the type of the kubernetes cluster
        project: foo
        cluster: bar
        zone: us-central1-a
    steps:
      - call_workflow: use_google_credentials
      - call_workflow: use_gcloud_kubeconfig
      - call_workflow: run_kubernetes:
        with:
          steps+:
            - type: gcloud
              shell: kubectl delete jobs {{ .ctx.job_name }}
```
<!-- {% endraw %} -->

 * **`use_local_kubeconfig` workflow**

This workflow is used for adding a step to clear the kubeconfig file so `kubectl` can use default in-cluster setting to work on local cluster.

For example:
<!-- {% raw %} -->
```yaml
---
workflows:
  copy_deployment_to_local:
    steps:
      - call_workflow: use_google_credentials
      - call_workflow: use_gcloud_kubeconfig
        with:
          cluster:
            project: foo
            cluster: bar
            zone: us-central1-a
      - export:
          steps+:
            - type: gcloud
              shell: kubectl get -o yaml deployment {{ .ctx.deployment }} > kuberentes.yaml
      - call_workflow: use_local_kubeconfig
      - call_workflow: run_kubernetes
        with:
          steps+:
            - type: gcloud
              shell: kubectl apply -f kubernetes.yaml
```
<!-- {% endraw %} -->

## Slash Commands

The new version of `DipperCL` comes with integration with `Slack`, including **slash commands**, right out of the box. Once the integration is setup, we can easily add/customize the slash commands. See integration guide (coming soon) for detailed instruction. There are a few predefined commands that you can try out without need of any further customization.

### Predefined Commands

 * **`help`** - print the list of the supported command and a brief usage info
 * **`reload`** - force honeydipper daemon to check and reload the configuration

### Adding New Commands

Let's say that you have a new workflow that you want to trigger through slash command. Define or extend a `_slashcommands` context to have something like below.

```yaml
contexts:
  _slashcommands:
    slashcommand:
      slashcommands:
        <command>:
          workflow: <workflow>
          usage: just some brief intro to your workflow
          contexts: # optionally you can run your workflow with these contexts
            - my_context
```
Replace the content in `<>` with your own content.

### Mapping Parameters

Most workflows expect certain context variables to be available in order to function, for example, you may need to specify which DB to backup or restore using a `DB` context variable when invoking a backup/restore workflow. When a slash command is defined, a `parameters` context variable is made available as a string that can be accessed through `$ctx.parameters` using path interpolation or <!-- {% raw %} -->`{{ .ctx.parameters }}`<!-- {% endraw %} --> in go templates. We can use the `_slashcommands` context to transform the `parameters` context variable into the actual variables the workflow requires.

For an simple example,
```yaml
contexts:
  _slashcommands:
    slashcommand:
      slashcommands:
        my_greeting:
          workflow: greeting
          usage: respond with greet, take a single word as greeter

    greeting: # here is the context applied to the greeting workflow
      greeter: $ctx.parameters # the parameters is transformed into the variable required
```

In case you want a list of words,
<!-- {% raw %} -->
```yaml
contexts:
  _slashcommands:
    slashcommand:
      slashcommands:
        my_greeting:
          workflow: greeting
          usage: respond with greet, take a list of greeters

    greeting: # here is the context applied to the greeting workflow
      greeters: :yaml:{{ splitList " " .ctx.parameters }} # this generates a list
```
<!-- {% endraw %} -->

Some complex example, command with subcommands
<!-- {% raw %} -->
```yaml
contexts:
  _slashcommands:
    slashcommand:
      slashcommands:
        jobs:
          workflow: jobHandler
          usage: handling internal jobs

    jobHandler:
      command: '{{ splitList " " .ctx.parameters | first }}'
      name: '{{ splitList " " .ctx.parameters | rest | first }}'
      jobParams: ':yaml:{{ splitList " " .ctx.parameters | slice 2 | toJson }}'
```
<!-- {% endraw %} -->

### Messages and notifications

By default, a slashcommand will send acknowledgement and return status message to the channel where the command is launched. The messages will only be visible to the sender, in other words, is `ephemeral`. We can define a list of channels to receive the acknowledgement and return status in addition to the sender. This increases the visibility and auditability. This is simply done by adding a `slash_notify` context variable to the `slashcommand` workflow in the `_slashcommands` context.

For example,
```yaml
contexts:
  _slashcommands:
    slashcommand:
      slash_notify:
        - "#my_team_channel"
        - "#security"
        - "#dont_tell_the_ceo"
      slashcommands:
        ...
```

### Secure the commands

When defining each command, we can use `allowed_channels` field to define a whitelist of channels from where the command can be launched. For example, it is recommended to override the `reload` command to be launched only from the whitelist channels like below.

```yaml
contexts:
  _slashcommands:
    slashcommand:
      slashcommands:
        reload: # predefined
          allowed_channels:
            - "#sre"
            - "#ceo"
```
