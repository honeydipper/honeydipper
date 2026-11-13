# Honeydipper Interpolation Guide

Tips: use [Honeydipper config check](./configuration.md#config-check) feature to quickly identify errors and issues before committing your configuration changes, or setup
your configuration repo with CI to run config check upon every push or PR.

<!-- toc -->

- [Prefix interpolation](#prefix-interpolation)
  * [*`ENC[driver,ciphertext/base64==]`* Encrypted content](#encdriverciphertextbase64-encrypted-content)
  * [*`LOOKUP[driver,path]`* Secret Lookup](#lookupdriverpath-secret-lookup)
  * [*:regex:* Regular expression pattern](#regex-regular-expression-pattern)
  * [*:yaml:* Building data structure with yaml](#yaml-building-data-structure-with-yaml)
  * [*:yaml_safe:* Building data structure with yaml without interpolation](#yaml_safe-building-data-structure-with-yaml-without-interpolation)
  * [*\\* Backslash escape mechanism](#-backslash-escape-mechanism)
  * [*$* Referencing context data with given path](#-referencing-context-data-with-given-path)
- [Inline go template](#inline-go-template)
  * [Caveat: What does "inline" mean?](#caveat-what-does-inline-mean)
  * [Sprig Library](#sprig-library)
  * [go template](#go-template)
  * [Functions offered by Honeydipper](#functions-offered-by-honeydipper)
    + [return](#return)
    + [duration](#duration)
    + [render](#render)
    + [fromPath](#frompath)
    + [toYaml](#toyaml)
    + [now](#now)
    + [ISO8601](#iso8601)
    + [cue_validate_error](#cue_validate_error)
    + [decrypt](#decrypt)
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
prefix is the name of the driver that can be used for decrypting the content. Following the driver name is a `,` and the base64 encoded
ciphertext.

Can be used in system data, event conditions.

For example:
```yaml
systems:
  kubenetes:
    data:
      service_account: ENC[gcloud-kms,...]
```

### *`LOOKUP[driver,path]`* Secret Lookup

The `LOOKUP` prefix is used to fetch secrets from a secret store driver (e.g., Vault, AWS Secrets Manager) at runtime. It works similarly to `ENC` but retrieves the secret dynamically rather than decrypting an encrypted value.

Syntax:
```
LOOKUP[driver,path][:printf_pattern]
```

- `driver`: The name of the driver that implements the `lookup` RPC method.
- `path`: The path or identifier of the secret in the secret store.
- `printf_pattern` (optional): A printf-style pattern to format the retrieved secret.

You can prefix the path with `?` to make the lookup optional (swallow errors if the secret is not found).

For example:
```yaml
systems:
  myapp:
    data:
      api_key: LOOKUP[vault,secret/data/myapp/apikey]
      optional_setting: LOOKUP[vault,secret/data/myapp/optional]:%s
      db_password: LOOKUP[aws-secretsmanager,myapp/db_password]
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

### *:yaml_safe:* Building data structure with yaml without interpolation

The `:yaml_safe:` prefix works similarly to `:yaml:`, but it will parse the string as YAML and return the resulting data structure **without** performing any further Go templating on it. This is useful when the YAML string contains Go template-like syntax that should be preserved as-is.

Can be used in workflow definitions(data, content), workflow condition, function parameters.

For example:
<!-- {% raw %} -->
```yaml
workflows:
  pass_through:
    content: |
      :yaml_safe:
      - "{{ .some.var }}"
      - "preserve {{ this }}"
```
<!-- {% endraw %} -->

### *\* Backslash escape mechanism

In some cases, you may need to prevent Honeydipper from interpreting special prefixes like `{{` or `$`. You can use a backslash `\` as an escape mechanism. When a string starts with a backslash followed by a prefix, Honeydipper will strip the backslash and return the rest of the string as-is without any interpolation.

For example:
```yaml
# This will be rendered as "{{ .ctx.value }}" without being interpolated
escaped_value: \{{ .ctx.value }}
```

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

### Sprig Library

Honeydipper includes the full [Sprig library](http://masterminds.github.io/sprig/) of template functions. Sprig provides over 100 template
functions for string manipulation, math, date handling, data structure manipulation, and more. Some commonly used functions include:

- `trim`, `trimAll`, `trimPrefix`, `trimSuffix`: String trimming
- `upper`, `lower`, `title`, `untitle`: String case manipulation
- `replace`: String replacement
- `split`, `join`: String splitting and joining
- `b64enc`, `b64dec`: Base64 encoding/decoding
- `dict`, `list`: Creating maps and lists
- `merge`, `append`: Data structure manipulation
- `date`, `now`, `duration`: Date and time functions

### go template

Here are some available resources for go template:
 * How to use go template? [https://golang.org/pkg/text/template/](https://golang.org/pkg/text/template/)
 * [sprig functions](http://masterminds.github.io/sprig/)

### Functions offered by Honeydipper

#### return

The `return` function captures any value and prevents it from being rendered as a string. This is useful when you need to pass a complex
data structure (map, list, etc.) to a function parameter instead of a string representation.

For example:
<!-- {% raw %} -->
```yaml
workflows:
  capture_data:
    steps:
      - call_workflow: process
        with:
          items: '{{ return .ctx.items_list }}'
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

#### render

The `render` function allows for recursive nested interpolation. It takes a template string and a data map, and renders the template
with the provided data. This is useful for dynamically constructing and rendering templates.

For example:
<!-- {% raw %} -->
```yaml
workflows:
  render_template:
    steps:
      - call_workflow: something
        with:
          content: '{{ render "Hello, {{ .name }}" .ctx }}'
```
<!-- {% endraw %} -->

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

#### cue_validate_error

The `cue_validate_error` function validates a YAML content against a CUE schema. It takes three parameters: the schema name, the schema
definition, and the YAML content to validate. If validation fails, it returns the validation error message; otherwise, it returns an
empty string.

For example:
<!-- {% raw %} -->
```yaml
workflows:
  validate_config:
    steps:
      - call_workflow: something
        with:
          validation_error: '{{ cue_validate_error "myschema" .sysData.myschema .ctx.yaml_content }}'
```
<!-- {% endraw %} -->

#### decrypt

The `decrypt` function is injected by the operator for secret decryption. It can be used in templates to decrypt values that were
encrypted using the `ENC[...]` prefix. This function is typically used internally by Honeydipper but is available for advanced use cases.

For example:
<!-- {% raw %} -->
```yaml
workflows:
  use_secret:
    steps:
      - call_workflow: something
        with:
          secret: '{{ decrypt .ctx.encrypted_value }}'
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
