# Workflow Composing Guide

<!-- toc -->

- [Basics of workflow](#basics-of-workflow)
- [Types of workflows](#types-of-workflows)
  * [Named workflow](#named-workflow)
  * [Conditional workflow](#conditional-workflow)
  * [Pipe workflow](#pipe-workflow)
  * [Parallel workflow](#parallel-workflow)
- [Workflow helper](#workflow-helper)
  * [Repeat](#repeat)
  * [For each (pipe)](#for-each-pipe)
  * [For each (parallel)](#for-each-parallel)
  * [Wanted](#wanted)

<!-- tocstop -->

## Basics of workflow
`workflow` defines what to do and how to do when an event is triggered. A simplest workflow is to call a `function`. A `function` can be a `rawAction` that a driver provides or a `function` defined in a system, which is just a wrapper around a `rawAction` with some contextual data. `workflow` can be defined in rules directly in the `do` section, or it can be defined independently with a name so it can be re-used/shared among multiple rules. An example of a workflow defined in a rule calling a raw action:

```yaml
rules:
  - when:
      driver: webhook
      conditions:
        url: /test1
    do:
      type: function
      content:
        driver: web
        rawAction: request
        parameters:
          URL: http://example.com
```

An example of a workflow defined in a rule calling a system function:

```yaml
systems:
  slack:
    functions:
      say:
        driver: web
        rawAction: request
        parameters:
          URL: https://example.net/sdfa/sdaffasd
          header:
            Content-Type: application/json
          method: POST
rules:
  - when:
      driver: webhook
      conditions:
        url: /test2
    do:
      type: function
      content:
        target:
          system: slack
          function: say
        parameters:
          content:
            text: something happend!
```

## Types of workflows
To be able to do fun stuff such as reusing, combining multiple tasks, conditional executing etc., we need a flexible construct to define workflows. Different types of workflows are introduced. Every `workflow` has a `content` field and a `type` field. Depending on the value of `type` field, the `content` field means different things.

 * **named workflow**
 * **conditional workflow**
 * **pipe workflow**
 * **parallel workflow**
 * **switch workflow**
 * **suspend workflow**

### Named workflow
When the `type` is not defined or empty, the `content` will be interpreted as a name pointing to a `workflow` defined in the `workflows` section of the config. This enables re-using of the workflow among rules. See below for example:

```yaml
workflows:
  announce:
    type: function
    content:
      target:
        system: slack
        function: say
      parameters:
        content:
          text: I am doing something
rules:
  - when:
      driver: webhook
      conditions:
        url: /test1
    do:
      content: announce
  - when:
      source:
        system: some_other_system
        event: something
    do:
      content: announce
```

### Conditional workflow
When the `type` is 'if', an additional `condition` field is required. The `content` should be a list of at most two child workflows. If the value of `condition` field is "truy", such as 1, "true", "True" etc., the first child workflow will be executed, otherwise, the second child workflow will be executed if present. To use interpolation in the `condition` field see the [Interpoation Guide](./interpolation.md) for detail.

For example:
<!-- {% raw %} -->
```yaml
rules:
  - when:
      source:
        system: server_room
        event: temperature_reading
  - do:
      type: if
      condition: '{{ gt .event.json.temp.value 80 }}'
      content:
        - type: function
          content:
            target:
              system: server_room_ac
              function: turn_on
```
<!-- {% endraw %} -->

### Pipe workflow
When the `type` is set to 'pipe', the `content` will be interpreted as a list of child workflows. The child workflows will be executed one by one in the order listed. The return information of the previous child workflow will be available to the next children through interpolation. See [Interpoation Guide](./interpolation.md) for detail.

For example:

<!-- {% raw %} -->
```yaml
rules:
  - when:
      source:
        system: code_repo
        event: new_commit
  - do:
      type: pipe
      content:
        - content: run_test
        - type: if
          condition: '{{ eq .labels.status "success" }}'
          content:
            - content: run_build
        - type: if
          condition: '{{ eq .labels.status "success" }}'
          content:
            - content: run_deploy
```         
<!-- {% endraw %} -->

### Parallel workflow
Similiar to `pipe` workflow, the `content` of `parallel` workflow is also a list of child workflows. The child workflows will be executed in parallel.

For example:

```yaml
workflows:
  notify_all:
    type: parallel
    content:
      - content: notify_sre
      - content: notify_dev
      - content: notify_security
```

### Switch workflow
`switch` workflow is similar to `switch` statement in some programing languages. It choose one workflow from the branches of child workflows based on `condition` field. The value of `content` should be a  map from strings to child workflows.

For example:

<!-- {% raw %} -->
```yaml
workflows:
  slashcommands:
    type: switch
    condition: '{{ .wfdata.command }}'
    content:
      help:
        content: show_help
      reload:
        content: reload_config
      "*":
        content: show_unknown_command_err
```
<!-- {% endraw %} -->

### Suspend workflow
A workflow can be suspended to wait for manual approval or manual intervention. It is useful in conjunction with slack (or other chat system) interactive components, web dashboards, etc. to inject manual interaction into the automation. The `content` should be a globally unique string serving as an identifer for the workflow.  The workflow will continue when a message with "resume_session" subject carries the identifier in `key` field of the payload.

For example

<!-- {% raw %} -->
```yaml
workflows:
  confirm_then_apply:
    type: pipe
    data:
      interactive_id: '{{ randAlphaNum 16 }}'
    content:
      - content: tf_plan
      - content: slack_result_with_interactive
        data:
          callback_id: '{{ .wfdata.interactive_id }}'
      - type: suspend
        content: '{{ .wfdata.interactive_id }}'
      - type: if
        condition: '{{ eq .data.reply "yes" }}'
        content:
          - content: tf_apply
```
<!-- {% endraw %} -->

## Workflow helper
With the combination of the various type of workflows, it is possible to create a lot of complex workflows. To keep our rules and workflows simple and DRY, we have created a few helper workflows that can be used in some common cases. They are implemented using the same building blocks introduced in the above chapters. Feel free to check them out in the `workflow_helper.yaml`. They also serve as a showcase on how we can use the building blocks.

### Repeat
Repeat the same `work` for the specified number of times, the `work` should be a workflow. The remaining times can be accessed through wfdata using interpolation. Please note that, the interpolation in the `work` should be escaped. See [Interpoation Guide](./interpolation.md) for detail.

For example:

<!-- {% raw %} -->
```yaml
rules:
  - when:
      source:
        system: accounting
        event: revenue_yoy_growth
  - do:
      type: if
      condition: '{{ gt .event.growth_percent 100 }}'
      content:
        - content: repeat
          data:
            times: 3
            work:
              type: function
              content:
                target:
                  system: slack
                  function: say
                parameters:
                  content:
                    text: hooray! {{ "{{ .wfdata.times }}" }}
                  channel: '#corp'
```
<!-- {% endraw %} -->

### For each (pipe)
Repeat the same workflow in `work` for each of the item listed in the `items`. While the loop is running, the remaining items can be accessed through `wfdata` using interpolation. Again, the interpolation in `work` should be escaped.  See [Interpoation Guide](./interpolation.md)  for detail.

For example:

<!-- {% raw %} -->
```yaml
workflows:
  tell_my_favorite:
    content: foreach
    data:
      items:
        - apple
        - orange
        - strawberry
      work:
        type: function
        content:
          target:
            system: slack
            function: say
          parameters:
            content:
              text: I like {{ "{{ first .wfdata.items }}" }}
```
<!-- {% endraw %} -->

### For each (parallel)
Same as the `pipe` version of the `foreach`, `foreach_parallel` will repeat the `work` for each item listed.  The difference is that the parallel version will run the child workflow with all the items in parallel.  There is no guanrantee of order, there is no "remaing items".  The current item can still be accessed through `.wfdata.current`.

For example:

<!-- {% raw %} -->
```yaml
workflows:
  notify_all:
    content: foreach
    data:
      items:
        - '#sre'
        - '#core'
        - '@tom'
      work:
        type: function
        content:
          target:
            system: slack
            function: say
          parameters:
            content:
              text: Notifying something
            channel: '{{ "{{ .wfdata.current }}" }}'
```
<!-- {% endraw %} -->

### Wanted
More helpers wanted

 * repeat_when_fail
 * foreach_when_success
 * when

