# Driver Developer's Guide

This document is intended for Honeydipper driver developers. Some programming experience is expected. Theoretically, we can use any
programming language, even bash, to develop a driver for honeydipper. For now, there is a go library named honeydipper/dipper that makes it
easier to do this in golang.

<!-- toc -->

- [Basics](#basics)
- [By Example](#by-example)
- [Driver lifecycle and states](#driver-lifecycle-and-states)
- [Messages](#messages)
- [RPC](#rpc)
- [Driver Options](#driver-options)
- [Collapsed Events](#collapsed-events)
- [Provide Commands](#provide-commands)
- [Publishing and packaging](#publishing-and-packaging)

<!-- tocstop -->

## Basics

 * Drivers are running in separate processes, so they are executables
 * Drivers communicate with daemon through stdin/stdout, logs into stderr
 * The name of the service that the driver is working for is passed in as an argument

## By Example

Below is a simple driver that does nothing but restarting itself every 20 seconds.
```go
package main

import (
  "flag"
  "github.com/honeydipper/honeydipper/pkg/dipper"
  "os"
  "time"
)

var driver *dipper.Driver

func main() {
  flag.Parse()

  driver = dipper.NewDriver(os.Args[1], "dummy")
  if driver.Service == "receiver" {
    driver.Start = waitAndSendDummyEvent
  }
  driver.Run()
}

func waitAndSendDummyEvent(msg *dipper.Message) {
  go func() {
    time.Sleep(20 * time.Second)
    driver.SendMessage(&dipper.Message{
      Channel: "eventbus",
      Subject: "message",
      Payload: map[string]interface{}{"data": []string{"line 1", "line 2"}},
    })
    driver.State = "cold"
    driver.Ping(msg)
  }()
}
```

The first thing that a driver does is to parse the command line arguments so the service name can be retrieved through *os.Args[1]*.
Following that, the driver creates a helper object with *dipper.NewDriver*.  The helper object provides hooks for driver to define the
functions to be executed at various stage in the life cycle of the driver. A call to the *Run()* method will start the event loop to receive
communication from the daemon.

There are 4 types of hooks offered by the driver helper objects.
 * Lifecycle events
 * Message handler
 * RPC handler 
 * Command handler

Note that the *waitAndSendDummyEvent* method is assigned to *Start hook*. The *Start hook* needs to return immediately, so the method
launches another event loop in a go routine and return the control to the helper object. The second event loop is where the driver actually
receives events externally and use *driver.SendMessage* to relay to the service.

In this example, the dummy driver just manifest a fake event with json data as 
```json
{"data": ["line 1", "line 2"]}
```

The method also sets its status to "cold", meaning cold restart needed, and uses the *Ping* command to send its own state to the daemon, so
it can be restarted.

## Driver lifecycle and states

The driver will be in "loaded" state initially. When the *Run()* method is invoked, it will start fetching messages from the daemon. The
first message is always "command:options" which carries the data and configuration required by the driver to perform its job. The helper
object has a built-in handler for this and will dump the data into a structure which can later be queried using *driver.GetOption* or
*driver.GetOptionStr* method.

Following the "command:options" is the "command:start" message. The helper object also has a built-in handler for the "command:start"
message. It will first call the *Start hook* function, if defined, then change the driver state to "alive" then report the state back to
daemon with *Ping* method. One important thing here is that if the daemon doesn't receive the "alive" state within 10 seconds, it will
consider the driver failed to start and kill the process by closing the stdin/stdout channels. You can see why the Start hook has to return
immediately.

When the daemon loads an updated version of the config, it will use "command:options" and "command:start" again to signal the driver to
reload. Instead of calling *Start hook*, it will call *Reload hook* for reloading. If *Reload hook* is not defined, it will report to the
daemon with "cold" state to demand a cold restart.

There is a handler for "command:stop" which calls the *Stop hook* for gracefully shutting down the driver. Although this is not needed most
of time, assuming the driver is stateless, it does have some uses if the driver uses some resources that cannot be released gracefully by
exiting.

## Messages

Every message has an envelope, a list of labels and a payload. The envelope is a string ends with a newline, with fields separated by
space(s). An valid envelope has following fields in the exact order:
 * Channel
 * Subject
 * Number of labels
 * Size

Following the envelop are a list of labels, each label is made up with a label definition line and a list of bytes as label value. The
label definition includes
 * Name of the label
 * Size of the label in bytes

The payload is usually a byte array with certain encoding. As of now, the only encoding we use is "json".
An example of sending a message to the daemon:
```go
driver.SendMessage(&dipper.Message{
  Channel: "eventbus",
  Subject: "message",
  Labels: map[string]string{
    "label1": "value1",
  },
  Payload: map[string]interface{}{"data": []string{"line 1", "line 2"},
  IsRaw: false, # default
})
```
The payload data will be encoded automatically. You can also send raw message if use `IsRaw` as `true`, meaning that
the driver will not attempt to encode the data for you, instead it will use the payload as bytes array directly.
In case you need to encode the message yourself, there are two methods, *dipper.SerializePayload*
accepts a \*dipper.Message and put the encoded content back into the message, or *dipper.SerializeContent* which
accepts bytes array and return the data structure as map.

When a message is received through the *Run()* event loop, it will be passed to various handlers as a \*dipper.Message
struct with raw bytes as payload. You can call *dipper.DeserializeContent* which accepts a byte array to decode
the byte array, and you can also use *dipper.DeserializePayload* which accepts a \*dipper.Message and place the
decoded payload right back into the message.

Currently, we are categorizing the messages into 3 different channels:
 * eventbus: messages that are used by *engine* service for workflow processing, subject could be `message`, `command` or `return`
 * RPC: messages that invoke another driver to run some function, subject could be `call` or `return`
 * state: the local messages between driver and daemon to manage the lifecycle of drivers

## RPC

Within the driver helper object, there are two helper objects that are meant for helping with RPC related activities.
 * *dipper.Driver.RPC.Caller*
 * *dipper.Driver.RPC.Provider*

To make a RPC Call, you don't have to use the `Caller` object directly, just use `RPCCall` or `RPCCallRaw` method,
Both method block for return with 10 seconds timeout. The timeout is not tunable at this time. For example,
calling the `gcloud-kms` driver for decryption

```go
decrypted, err := driver.RPCCallRaw("driver:gcloud-kms", "decrypt", encrypted)
```

To offer a RPC method for the system to call, create the function that accept a single parameter `*dipper.Message`. Add the method
to `Provider.RPCHandlers` map, for example

```go
driver.RPC.Provider.RPCHandler["mymethod"] = MyFunc

func MyFunc(m *dipper.Message) {
  ...
}
```

Feel free to panic in your method, the wrapper will send an error response to the caller if that happens. To return data to the caller
use the channel `Reply` on the incoming message. For example:

```go
func MyFunc(m *dipper.Message) {
  dipper.DeserializePayload(m)
  if m.Payload != nil {
    panic(errors.New("not expecting any parameter"))
  }
  m.Reply <- dipper.Message{
    Payload: map[string]interface{}{"mydata": "myvalue"},
  }
}
```

## Driver Options

As mentioned earlier, the driver receives the options / configurations from the daemon automatically through the
helper object. As the data is stored in hashmap, the helper method *driver.GetOption* will accept a path and return an
*Interface()* object. The path consists of the dot-delimited key names. If the returned data is
also a map, you can use *dipper.GetMapData* or *dipper.GetMapDataStr* to retrieve information from them as well.
If you are sure the data is a *string*, you can use *driver.GetOptionStr* to directly receive it as *string*.

The helper functions follow the golang convention of returning the value along with a bool to indicate if it is
acceptable or not. See below for example.
```go
  NewAddr, ok := driver.GetOptionStr("data.Addr")
  if !ok {
    NewAddr = ":8080"
  }

  hooksObj, ok := driver.GetOption("dynamicData.collapsedEvents")
  ...
  somedata, ok := dipper.GetMapDataStr(hooksObj, "key1.subkey")
  ...
```

There is always a *data* section in the driver options, which comes from the configuration file, e.g.:
```yaml
---
...
drivers:
  webhook:
    Addr: :880
...
```

## Collapsed Events

Usually an event receiver driver just fires raw events to the daemon; it doesn't have to know what the daemon is expecting. There are some
exceptions, for example, the webhook driver needs to know if the daemon is expecting some kind of webhook so it can decide what response to
send to the web request sender, 200, 404 etc. A collapsed event is an event definition that has all the conditions, including the conditions
from events that the current event is inheriting from. Dipper sends the collapsed events to the driver in the options with key name
"dynamicData.collapsedEvents". Drivers can use the collapsed events to setup the filtering of the events before sending them to daemon. Not
only does this allow the driver to generate meaningful feedback to the external requesters, but it also serves as the first line of defence
against DDoS attacks on the daemon.

Below is an example of using the collapsed events data in webhook driver:

```go
func loadOptions(m *dipper.Message) {
  hooksObj, ok := driver.GetOption("dynamicData.collapsedEvents")
  if !ok {
    log.Panicf("[%s] no hooks defined for webhook driver", driver.Service)
  }
  hooks, ok = hooksObj.(map[string]interface{})
  if !ok {
    log.Panicf("[%s] hook data should be a map of event to conditions", driver.Service)
  }
  ...
}

func hookHandler(w http.ResponseWriter, r *http.Request) {
  eventData := extractEventData(w, r)

  matched := false
  for SystemEvent, hook := range hooks {
    for _, condition := range hook.([]interface{}) {
      if dipper.CompareAll(eventData, condition) {
        matched = true
        break
      }
    }
    if matched {
      break
    }
  }

  if matched {
    ...
  } else {
    ...
  }
}
```

The helper function *dipper.CompareAll* will try to match your event data to the conditions. Daemon uses the same
function to determine if a `rawEvent` is triggering events defined in systems.

## Provide Commands

A command is a raw function that provides response to an event. The workflow engine service sends "eventbus:command" messages to the operator
service, and operator service will map the message to the corresponding driver and raw function, then forward the message to the
corresponding driver with all the parameters as a "collapsed function". The driver helper provides ways to map raw actions to the function
and handle the communications to back to the daemon.

A command handler is very much like the RPC handler mentioned earlier. All you need to do is add it to the `driver.CommandProvider.Commands`
map. The command handler function should always return a value or panic. If it exists without a return, it can block invoking workflow until
it times out. If you don't have any data to return, just send a blank message back like below.

```go
func main() {
  ...
  driver.CommandProvider.Commands["wait10min"] = wait10min
  ...
}

func wait10min(m *dipper.Message) {
  go func() {
    time.Sleep(10 * time.Minute)
    m.Reply <- dipper.Message{}
  }()
}
```

Note that the reply is sent in a go routine; it is useful if you want to make your code asynchronous.

## Publishing and packaging

To make it easier for users to adopt your driver, and use it efficiently, you can create a public git repo and let users
load some predefined configurations to jump start the integration.  The configuration in the repo should usually include:

 * `driver` definition and `fearture` loading under the `daemon` section;
 * some wrapper `system` to define some `trigger`, `function` that can be used in rules;
 * some `workflow` to help users use the `function`s, see [Workflow composing guide](./workflow.md) for detail

For example, I created a hypothetical integration for a z-wave switch, the configuration might look like:
<!-- {% raw %} -->
```yaml
---
daemon:
  drivers:
    myzwave:
      name: myzwave
      data:
        Type: go
        Package: github.com/example/cmd/myzwave
  features:
    receiver:
      - "driver:myzwave"
    operator:
      - "driver:myzwave"

system:
  lightwitch:
    data:
      token: "placeholder"
    triggers:
      driver: myzwave
      rawEvent: turned_on
      conditions:
        device_id: "placeholder"
        token: "{{ .sysData.token }}"
    functions:
      driver: myzwave
      rawAction: turn_on
      parameters:
        device_id: "placeholder"
        token: "{{ .sysData.token }}"

workflows:
  all_lights_on:
    - content: foreach_parallel
      data:
        items:
          - list
          - of
          - device_ids
          - to_be_override
        work:
          - type: function
            content:
              target:
                system: lightswitch
                function: turn_on
              parameters:
                device_id: '{{ `{{ .wfdata.current }}` }}'
```
<!-- {% endraw %} -->

Assuming the configuration is in github.com/example/myzwave-config/init.yaml, the users only need to load the below snippet into
their bootstrap repo to load your driver and configurations, and start to customizing.

```yaml
repos:
  ...
  - repo: https://github.com/example/myzwave-config
  ...
```
