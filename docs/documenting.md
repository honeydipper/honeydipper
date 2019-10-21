# DipperCL Document Automatic Generation

<!-- toc -->

- [Documenting a Driver](#documenting-a-driver)
- [Document a System](#document-a-system)
- [Document a Workflow](#document-a-workflow)
- [Formatting](#formatting)
- [Building](#building)
- [Publishing](#publishing)

<!-- tocstop -->

Honeydipper configuration language (`DipperCL`) supports storing meta information of the configurations such as the purpose of the
configuration, the fields or parameters definition and examples. The meta information can be used for automatic document generating and
publishing.

The meta information are usually recorded using `meta` field, or `description` field. They can be put under any of the below list of locations

 1. `drivers.daemon.drivers.*` - meta information for a `driver`
 2. `systems.*` - each system can have its meta information here
 3. `systems.*.functions.*` - each system function can have its meta information here
 4. `systems.*.triggers.*` - each system triggers can have its meta information here
 5. `workflows.*` - each workflow can have its meta information here

The `description` field is usually a simple string that will be a paragraph in the document immediately following the name of the entry.
The `meta` field is a map of different items based on what entry the meta is for.

Since the `description` field does not support formatting, and the it could be used for log generating at runtime, it is recommended to
not use the `description` field, and instead use a `description` field under the `meta` field.

## Documenting a Driver

Following fields are allowed under the `meta` field for a driver,

   * `description` - A list of items to be rendered as paragraphs following the top level description
   * `configurations` - A list of `name`, `description` pairs describing the items needed to be configured for this driver
   * `notes` - A list of items to be rendered as paragraphs following the configurations
   * `rawActions` - A list of meta information for `rawAction`, see below for detail
   * `rawEvents` - A list of meta information for `rawEvents`, see below for detail
   * `RPCs` - A list of meta information for `RPCs`, see below for detail

For each of the `rawActions`,

   * `description` - A list of items to be rendered as paragraphs following the name of the action
   * `parameters` - A list of `name`, `description` pairs describing the context variables needed for this action
   * `returns` - A list of `name`, `description` pairs describing the context variables exported by this action
   * `notes` - A list of items to be rendered as paragraphs following the above items

For each of the `rawEvents`,

   * `description` - A list of items to be rendered as paragraphs following the name of the event
   * `returns` - A list of `name`, `description` pairs describing the context variables exported by this event
   * `notes` - A list of items to be rendered as paragraphs following the above items

For each of the `RPCs`,

   * `description` - A list of items to be rendered as paragraphs following the name of the RPC
   * `parameters` - A list of `name`, `description` pairs describing the parameters needed for this RPC
   * `returns` - A list of `name`, `description` pares describing the values returned from this RPC
   * `notes` - A list of items to be rendered as paragraphs following the above items

For example, to define the meta information for a driver:

```yaml
---
drivers:
  daemon:
    drivers:
      my-driver:
        description: The driver is to enable Honeydipper to integrate with my service with my APIs.
        meta:
          description:
            - ... brief description for the system ...

          configurations:
            - name: foo
              description: A brief description for foo
            - name: bar
              description: A brief description for bar
              ...

          rawEvents:
            myEvent:
              description:
                - |
                  paragraph .....
              returns:
                - name: key1
                  description: description for key1
                - name: key2
                  description: description for key2

          notes:
            - some notes as text
            - example:
                ...
        ...
```

## Document a System

Following fields are allowed under the `meta` field for a `system`,

   * `description` - A list of items to be rendered as paragraphs following the top level description
   * `configurations` - A list of `name`, `description` pairs describing the items needed to be configured for this in system data
   * `notes` - A list of items to be rendered as paragraphs following the configurations

For each of the `functions`,

   * `description` - A list of items to be rendered as paragraphs following the name of the function
   * `inputs` - A list of `name`, `description` pairs describing the context variables needed for this function
   * `exports` - A list of `name`, `description` pares describing the context variables exported by this function
   * `notes` - A list of items to be rendered as paragraphs following the above items

For each of the `triggers`,

   * `description` - A list of items to be rendered as paragraphs following the name of the trigger
   * `exports` - A list of `name`, `description` pares describing the context variables exported by this trigger
   * `notes` - A list of items to be rendered as paragraphs following the above items

For example, to define the mata information for a system,
```yaml
---
systems:
  mysystem:
    meta:
      description:
        - ... brief description for the system ...

      configurations:
        - name: key1
          description: ... brief description for key1 ...
        - name: key2
          description: ... brief description for key2 ...

      notes:
        - ... some notes ...
        - example: |
          ... sample in yaml ...

    data:
      ...

    functions:
      myfunc:
        meta:
          description:
            - ... brief description for the function ...
          inputs:
            - name: key1
              description: ... brief description for key1 ...
            - name: key2
              description: ... brief description for key2 ...
          exports:
            - name: key3
              description: ... brief description for key3 ...
            - name: key4
              description: ... brief description for key4 ...
          notes:
            - ... some notes ...
            - example: |
                ... sample in yaml ...
```

## Document a Workflow

Following fields are allowed under the `meta` field for a `workflow`,

   * `description` - A list of items to be rendered as paragraphs following the top level description
   * `inputs` - A list of `name`, `description` pairs describing the context variables needed for this workflow
   * `exports` - A list of `name`, `description` pares describing the context variables exported by this workflow
   * `notes` - A list of items to be rendered as paragraphs following the above items

For example, to define the mata information for a workflow,
```yaml
---
workflows:
  myworkflow:
    meta:
      description:
        - ... brief description for the workflow ...
      inputs:
        - name: key1
          description: ... brief description for key1 ...
        - name: key2
          description: ... brief description for key2 ...
      exports:
        - name: key3
          description: ... brief description for key3 ...
        - name: key4
          description: ... brief description for key4 ...
      notes:
        - ... some notes ...
        - example: |
          ... sample in yaml ...

```

## Formatting

Both `description` and `notes` fields under `meta` support formatting. They accept a list of items that each
will be rendered as a paragraph in the documents. The only difference between them is the location where they will appear
in the documents.

Honeydipper `docgen` uses `sphinx` to render the documents, so the source document is in `rst` format. You can use
`rst` format in each of the paragraphs. You can also let `docgen` to format your paragraph by specify a data structure
as the item instead of plain text.

For example, plain text paragraph,
```yaml
description:
  - This is a plain text paragraph.
```

Highlighting the paragraph, see [sphinx document](https://sphinx-rtd-theme.readthedocs.io/en/stable/demo/demo.html#admonitions) for detail on highlight type.
```yaml
notes:
  - highlight: This paragraph will be highlighted.
  - highlight: This paragraph will be highlighted with type `error`.
    type: error
```

Specify a code block,
```yaml
notes:
  - See below for an example
  - example: | # by default, yaml
      ---
      rules:
        - when: ...
          do: ...
  - example: |
      func dosomething() {
      }
    type: go
```

## Building

In order to build the document for local viewing, follow below steps.

 1. install sphinx following the [sphinx installation document](http://www.sphinx-doc.org/en/master/usage/installation.html)
 2. install markdown extension for sphinx following the [recommonmark installation document](https://www.sphinx-doc.org/en/master/usage/markdown.html)
 3. install the `read the docs` theme for sphinx following the [readthedoc theme installation document](https://sphinx-rtd-theme.readthedocs.io/en/latest/installing.html)
 4. clone the `honeydipper-sphinx` repo
    ```bash
    git clone https://github.com/honeydipper/honeydipper-sphinx.git
    ```
 5. generating the source document for sphinx
    ```bash
    cd honeydipper-sphinx
    docker run -it -v $PWD/docgen:/docgen -v $PWD/source:/source -e DOCSRC=/docgen -e DOCDST=/source honeydipper/honeydipper:1.0.0 docgen
    ```
 6. build the documents
    ```bash
    # cd honeydipper-sphinx
    make html
    ```
  7. view your documents
```bash
# cd honeydipper-sphinx
open build/html/index.html
```

## Publishing

In order to including your document in the Honeydipper community repo section of the documents, follow below steps.

 1. clone the `honeydipper-sphinx` repo
    ```bash
    git clone https://github.com/honeydipper/honeydipper-sphinx.git
    ```
 2. modify the `docgen/docgen.yaml` to add your repo under the `repos` section
 3. submit a PR
