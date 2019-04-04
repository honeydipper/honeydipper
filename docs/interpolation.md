# Honeydipper Interpolation Guide

Tips: use [Honeydipper config check](./configuration.md#config-check) feature to quickly identify errors and issues before committing your configuration changes, or setup
your configuration repo with CI to run config check upon every push or PR.

<!-- toc -->

- [Prefix interpolation](#prefix-interpolation)
  * [*`ENC[driver,ciphertext/base64==]`* Encrypted content](#encdriverciphertextbase64-encrypted-content)
  * [*:regex:* Regular expression pattern](#regex-regular-expression-pattern)
  * [*:yaml:* Building data structure with yaml](#yaml-building-data-structure-with-yaml)
  * [*:path:* Referencing context data with given path](#path-referencing-context-data-with-given-path)
- [Inline go template](#inline-go-template)
  * [Caveat: What does "inline" mean?](#caveat-what-does-inline-mean)
  * [go template](#go-template)
  * [Functions offerred by Honeydipper](#functions-offerred-by-honeydipper)
  * [fromPath](#frompath)
- [Escaping function parameter interpolation](#escaping-function-parameter-interpolation)
- [Workflow contextual data](#workflow-contextual-data)
  * [Workflow Interpolation](#workflow-interpolation)
  * [Function Parameters Interpolation](#function-parameters-interpolation)

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
the following string as a regular expresstion pattern used for matching the conditions.

Can be used in system data, event conditions.

For example:
```yaml
rules:
  - when:
      driver: webhook
      conditions:
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
  foreach_parallel:
    type: if
    condition: '{{ gt (len .wfdata.items) 0 }}'
    content:
      - |
          :yaml:---
          type: parallel
          content:
            {{- range .wfdata.items }}
            - type: pipe
              content:
                - :path:wfdata.work
              data:
                current: "{{ . }}"
            {{- end }}
```
<!-- {% endraw %} -->

### *:path:* Referencing context data with given path

When Honeydipper executes a `workflow`, some data is kept in the context. We can use either the `:path:` prefix or the inline go template to
fetch the context data. The benefit of using `:path:` prefix is that we can get the data as a structure such as map or list instead of a
string representation.

Can be used in workflow definitions(data, content), workflow condition, function parameters.

For example:
<!-- {% raw %} -->
```yaml
workflows:
  next_if_success:
    type: if
    condition: '{{ eq .labels.status "success" }}'
    content: :path:wfdata.work
```
<!-- {% endraw %} -->

## Inline go template

Besides the `:path:` prefix, we can also use inline go template to access the workflow context data. The inline go template can be used in
workflow definitions(data, content), workflow conidtion, and function parameters.

### Caveat: What does "inline" mean?

Unlike in typical templating languages, where templates were executed before yaml rendering, Honeydipper renders all configuration yaml at
boot time or when reloading, and only executes the template when the particular content is needed. This allows Honeydipper to provide
runtime data to the template when it is executed. However, that also means that templates can only be stored in strings. You can't wrap yaml
tags in templates, unless you store the yaml as text like in the example for `:yaml:` prefix interpolation. Also, you can't use <!-- {% raw %} -->`{{`<!-- {% endraw %} --> at the
begining of a string without quoting, because the yaml renderer may treat it as the start of a data structure.

### go template

Here are some available resources for go template:
 * How to use go template? [https://golang.org/pkg/text/template/]
 * [sprig functions](http://masterminds.github.io/sprig/)

### Functions offerred by Honeydipper

### fromPath

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

## Escaping function parameter interpolation
Honeydipper executes the inline go templates and run `:path:`, `:yaml:` interpolations at two places:
 1. When a workflow starts
 2. When a driver function is invoked

When a workflow starts, Honeydipper workflow engine needs all information to determine how the workflow should be executed, so the `data`, `content` and `condition` fields are interpolated. It will not interpolate/execute the function parameters, because the contextual data for the function call has not been finalized yet, and it may very well be overridden by the children workflows. So, we want to leave the function parameters to the `operator` service to interpolate when all the data is available and finalized. Sometimes, we want to define some abstract workflows, such as `repeat` `foreach`, where the actual workflow content including the parameters is passed as workflow data, we will need to escape the parameters interpolation, if used, so workflow engine won't interpolate them too early.

For example:
<!-- {% raw %} -->
```yaml
rules:
  - when:
      source:
        system: something
        event: happened
  - do:
      content: foreach_parallel
      data:
        items:
          - '@user1'
          - '#a_channel'
        work:
          type: function
          content:
            target:
              system: slack
              function: say
            parameters:
              content:
                text: something happened
              # see below how to escape the interpolation
              channel: '{{ "{{ .wfdata.current }}" }}'
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
  * **wfdata**: the data passed to the workflow when it is invoked
  * **event**: the event payload that triggered the original workflow

### Function Parameters Interpolation
This happens at `operator` side, before the final `parameters` are passed to the `action driver`.

  * **data**: the payload of previous driver function return
  * **labels**: the workflow data attached to the dipper.Message
    * **status**: the status of the previous workflow, "success", "failure" (driver failure), "blocked" (failed in daemon)
    * **reason**: a string describe why the previous workflow is not successful
    * **sessionID**
  * **wfdata**: the data passed to the workflow when it is invoked
  * **event**: the event payload that triggered the original workflow
  * **sysData**: the data defined in the system the function belongs to
  * **params**: the parameter that is passed to the function

### Trigger Condition Interpolation
This happens at the startup of the `receiver` service.  All the used events are processed into `collapsed` events. The  `conditions` in the collapsed events are interpolated before being passed to `event driver`.

  * **sysData**: the data defined in the system the event belongs to
