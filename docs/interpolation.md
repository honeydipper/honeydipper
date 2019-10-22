# Honeydipper Interpolation Guide

Tips: use [Honeydipper config check](./configuration.md#config-check) feature to quickly identify errors and issues before committing your configuration changes, or setup
your configuration repo with CI to run config check upon every push or PR.

<!-- toc -->

- [Prefix interpolation](#prefix-interpolation)
  * [*`ENC[driver,ciphertext/base64==]`* Encrypted content](#encdriverciphertextbase64-encrypted-content)
  * [*:regex:* Regular expression pattern](#regex-regular-expression-pattern)
  * [*:yaml:* Building data structure with yaml](#yaml-building-data-structure-with-yaml)
  * [*$* Referencing context data with given path](#-referencing-context-data-with-given-path)
- [Inline go template](#inline-go-template)
  * [Caveat: What does "inline" mean?](#caveat-what-does-inline-mean)
  * [go template](#go-template)
  * [Functions offered by Honeydipper](#functions-offerred-by-honeydipper)
    + [fromPath](#frompath)
    + [now](#now)
    + [duration](#duration)
    + [ISO8601](#iso8601)
    + [toYaml](#toyaml)
- [Workflow contextual data](#workflow-contextual-data)
  * [Workflow Interpolation](#workflow-interpolation)
  * [Function Parameters Interpolation](#function-parameters-interpolation)
  * [Trigger Condition Interpolation](#trigger-condition-interpolation)

<!-- tocstop -->

Honeydipper functions and workflows are dynamic in nature. Parameters, system data, workflow data can be overridden at various phases, and
we can use interpolation to tweak the function calls to pick up the parameters dynamically, or even to change the flow of execution at
runtime.

## Prefix interpolation
When a string value starts with certain prefixes, Honeydipper will transform the value based on the function specified by the prefix.

### *`ENC[driver,ciphertext/base64==]`* Encrypted content

Encrypted contents are usually kept in system data. The value should be specified in `eyaml` style, start with `ENC[` prefix. Following the
prefix is the name of the driver that can be used for decrypting the content. Following the driver name is a "," and the base64 encoded
ciphertext.

Can be used in system data, event conditions.

For example:
```yaml
systems:
  kubenetes:
    data:
      service_account: ENC[gcloud-kms,...]
```

### *:regex:* Regular expression pattern

yaml doesn't have native support for regular expressions. When Honeydipper detects a string value starts with this prefix, it will interpret
the following string as a regular expression pattern used for matching the conditions.

Can be used in system data, event conditions.

For example:
```yaml
rules:
  - when:
      driver: webhook
      if_match:
        url: :regex:/test_.*$
  - do:
    ...
```

### *:yaml:* Building data structure with yaml

At first look, It may seem odd to have this prefix, since the config is yaml to begin with. In some cases, combining with the inline Go
template, we can dynamically generate complex yaml structure that we can't write at config time.

Can be used in workflow definitions(data, content), workflow condition, function parameters.

For example:
<!-- {% raw %} -->
```yaml
workflows:
  create_list:
    export:
      items: |
        :yaml:---
        {{- range .ctx.results }}
        - name: {{ .name }}
          value: {{ .value }}
        {{- end }}
```
<!-- {% endraw %} -->

### *$* Referencing context data with given path

When Honeydipper executes a `workflow`, some data is kept in the context. We can use either the `$` prefix or the inline go template to
fetch the context data. The benefit of using `$` prefix is that we can get the data as a structure such as map or list instead of a
string representation.

Can be used in workflow definitions(data, content), workflow condition, function parameters.

For example:
<!-- {% raw %} -->
```yaml
workflows:
  next_if_success:
    if:
      - $ctx.result
    call_workflow: $ctx.work
```
<!-- {% endraw %} -->

The data available for `$` referencing includes

 * `ctx` - context data
 * `data` - the latest received dipper message payload
 * `event` - the original dipper message payload from the event
 * `labels` - the latest receive dipper message labels

The `$` reference can be used with multiple data entry separated by `,`. The first non empty result will be used. For example,

```yaml
workflows:
  find_first:
    call_workflow: show_name
    with:
      name: $ctx.name,ctx.full_name,ctx.nick_name # choose the first non empty value from the listed varialbes
```

We can also specify a default value with quotes, either single quotes, double quotes or back ticks, if all the listed variables
are empty or nil. For example

```yaml
workflows:
  do_something:
    call_workflow: something
    with:
      timeout: $ctx.timeout,ctx.default_timeout,"1800"
```

We can also allow nil or empty value using a `?` mark. For example

```yaml
workflows:
  do_something:
    call_workflow: something
    with:
      timeout: $ctx.timeout,ctx.default_timeout,"1800"
      previous: $?ctx.previous
```


## Inline go template

Besides the `$` prefix, we can also use inline go template to access the workflow context data. The inline go template can be used in
workflow definitions(data, content), workflow condition, and function parameters.

### Caveat: What does "inline" mean?

Unlike in typical templating languages, where templates were executed before yaml rendering, Honeydipper renders all configuration yaml at
boot time or when reloading, and only executes the template when the particular content is needed. This allows Honeydipper to provide
runtime data to the template when it is executed. However, that also means that templates can only be stored in strings. You can't wrap yaml
tags in templates, unless you store the yaml as text like in the example for `:yaml:` prefix interpolation. Also, you can't use <!-- {% raw %} -->`{{`<!-- {% endraw %} --> at the
beginning of a string without quoting, because the yaml renderer may treat it as the start of a data structure.

### go template

Here are some available resources for go template:
 * How to use go template? [https://golang.org/pkg/text/template/](https://golang.org/pkg/text/template/)
 * [sprig functions](http://masterminds.github.io/sprig/)

### Functions offered by Honeydipper

#### fromPath

Like the `:path:` prefix interpolation, the `fromPath` function takes a parameter as path and return the data the path points to. It is
similar to the `index` built in function, but uses a more condensed path expression.

For example:
<!-- {% raw %} -->
```yaml
systems:
  opsgenie:
    functions:
      snooze:
        driver: web
        rawAction: request
        parameters:
          URL: https://api.opsgenie.com/v2/alerts/{{ fromPath . .params.alertIdPath }}/snooze
          header:
            Content-Type: application/json
            Authorization: GenieKey {{ .sysData.API_KEY }}
...

rules:
  - when:
      source:
        system: some_system
        event: some_event
    do:
      target:
        system: opsgenie
        function: snooze
      parameters:
        alertIdPath: event.json.alert.Id
```
<!-- {% endraw %} -->

#### now

This function returns current timestamps.

<!-- {% raw %} -->
```yaml
---
workflows:
  do_something:
    call_workflow: something
    with:
      time: '{{ now | toString }}'
```
<!-- {% endraw %} -->

#### duration

This function parse the duration string and can be used for date time calculation.

<!-- {% raw %} -->
```yaml
---
workflows:
  do_something:
    steps:
      - wait: '{{ duration "1m" }}'
      - call_workflow: something
```
<!-- {% endraw %} -->

#### ISO8601

This function format the timestamps into the ISO8601 format.

<!-- {% raw %} -->
```yaml
---
workflows:
  do_something:
    steps:
      - call_workflow: something
        with:
          time_str: '{{ now | ISO8601 }}'
```
<!-- {% endraw %} -->

#### toYaml

This function converts the given data structure into a yaml string

<!-- {% raw %} -->
```yaml
---
workflows:
  do_something:
    steps:
      - call_workflow: something
        with:
          yaml_str: '{{ .ctx.parameters | toYaml }}'
```
<!-- {% endraw %} -->

## Workflow contextual data
Depending on where the interpolation is executed, 1) workflow engine, 2) operator (function parameters), the available contextual data is slightly different.

### Workflow Interpolation
This happens when workflow `engine` is parsing and executing the workflows, but haven't sent the action definition to the `operator` yet.

  * **data**: the payload of previous driver function return
  * **labels**: the workflow data attached to the dipper.Message
    * **status**: the status of the previous workflow, "success", "failure" (driver failure), "blocked" (failed in daemon)
    * **reason**: a string describe why the previous workflow is not successful
    * **sessionID**
  * **ctx**: the data passed to the workflow when it is invoked
  * **event**: the event payload that triggered the original workflow

### Function Parameters Interpolation
This happens at `operator` side, before the final `parameters` are passed to the `action driver`.

  * **data**: the payload of previous driver function return
  * **labels**: the workflow data attached to the dipper.Message
    * **status**: the status of the previous workflow, "success", "failure" (driver failure), "blocked" (failed in daemon)
    * **reason**: a string describe why the previous workflow is not successful
    * **sessionID**
  * **ctx**: the data passed to the workflow when it is invoked
  * **event**: the event payload that triggered the original workflow
  * **sysData**: the data defined in the system the function belongs to
  * **params**: the parameter that is passed to the function

### Trigger Condition Interpolation
This happens at the start up of the `receiver` service.  All the used events are processed into `collapsed` events. The  `conditions` in the collapsed events are interpolated before being passed to `event driver`.

  * **sysData**: the data defined in the system the event belongs to
