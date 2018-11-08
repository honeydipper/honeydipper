# Honeydipper Driver Developer's Guide

This document is intended for the honeydipper driver developers. Some programming experience is expected.
Theoriadically, we can use any programing langulage, even bash, to develop a driver for honeydipper. For
now, there is a go library named honeydipper/dipper that makes it easier to do this in golang.

<!-- toc -->

- [Basics](#basics)
- [By Example](#by-example)
- [Driver lifecycle and states](#driver-lifecycle-and-states)
- [Messages](#messages)
- [RPC](#rpc)
- [Driver Options](#driver-options)

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
	"github.com/honeyscience/honeydipper/dipper"
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
		driver.SendMessage("eventbus", "message", map[string]interface{}{"data": []string{"line 1", "line 2"}})
		driver.State = "cold"
		driver.Ping(msg)
	}()
}
```
The first thing that a driver does is to parse the command line arguments so the service name can be
retrieved through *os.Args[1]*. Following that, the driver creates a helper object with *dipper.NewDriver*.
The helper object provides hooks for driver to define the functions to be executed at various stage in
the life cycle of the driver. A call to the *Run()* method will start the event loop to receive communication
from the daemon.

There are 3 types of hooks offered by the driver helper objects.
 * Lifecycle events
 * Message handler
 * RPC handler 

Note that the *waitAndSendDummyEvent* method is assigned to *Start hook*. The *Start hook* needs to return
immediately, so the method luanches another event loop in a go routine and return the control to the 
helper object. The second event loop is where the driver actually receives events externally and use
*driver.SendMessage* to relay to the service.

In this example, the dummy driver just manifest a fake event with json data as 
```json
{"data": ["line 1", "line 2"]}
```
The driver also sets its status to "cold", meaning cold restart needed, and uses *Ping* command to send its
own state to the daemon, so it can be restarted.

## Driver lifecycle and states

The driver will be in "loaded" state initially. When the *Run()* method is invoked, it will
start fetching messages from daemon. The first message is always "command:options" which
carries the data and configuration required by the driver to perform its job. The helper
object has a builtin handler for this and will dump the data into a structure which can
later be queried using *driver.GetOption* or *driver.GetOptionStr* method.

Following the "command:options" is the "command:start" message. The helper object also has a builtin handler for
the "command:start" message. It will first call the *Start hook* function, if defined, then change the driver state
to "alive" then report the state back to daemon with *Ping* method. One important thing here is that if the daemon doesn't
receive the "alive" state within 10 seconds, it will consider the driver failed to start and kill the process
by closing the stdin/stdout channels. You can see why the Start hook has to return immediately.

When daemon loads an updated version of the config, it will use "command:options" and "command:start"
again to signal the driver to reload.  Instead of calling *Start hook*, it will call *Reload hook* for reloading.
If *Reload hook* is not defined, it will report to the daemon with "cold" state to demand a cold restart.

There is a handler for "command:stop" which calls the *Stop hook* for gracefully shutting down the 
driver.  Although this is not needed most of time, assuming the driver is stateless, it does have some uses if the
driver uses some resources that cannot be released gracefully by exiting.

## Messages

Every message has an envelope and a payload.  The envelope is a string ends with a newline, with fields separated by
space(s). An valid envelope has following fields in the exact order:
 * Channel
 * Subject
 * Size

The payload is usually a byte array with certain encoding.  As of now, the only encoding we use is "json".
An example of sending message to daemon
```go
driver.SendMessage("eventbus", "message", map[string]interface{}{"data": []string{"line 1", "line 2"}})
```
The payload data will be encoded automatically.  There is also a *SendRawMessage* method that you can pass the
byte array directly. In case you need to encode the message yourself, there are two methods, *dipper.SerializePayload*
accepts a \*dipper.Message and put the encoded content back into the message, or *dipper.SerializeContent* which
accepts bytes array and return the data structure as map.

When a messge is received through the *Run()* event loop, it will be passed to various handlers as a \*dipper.Message
struct with raw bytes as payload.  You can call *dipper.DeserializeContent* which accepts a byte array to decode
the byte array, and you can also use *dipper.DeserializePayload* which accepts a \*dipper.Message and place the
decoded paylod right back into the message.

Currently, we are distinguish the messages into 4 different channels.
 * eventbus: messages that need to reach *engine* service for workflow processing
 * command: messages local the service and driver for lifecycle and driver state handling
 * rpc: messages that invoke another driver to run some function
 * execute: messages to *operator* driver for executing an action in response to the events

## RPC

There are a few wrapper method for you to make PRC calls with the helper object.
 * caller.RPCCall: accepts the method as "driver:<driver_name>.<method_name>" and parameter as a map
 * caller.RPCCallRaw: same as RPCCall except that expects the parameter to be byte array

Both method block for return with 10 seconds timeout.  The timeout is not tunable at this time.

Example:
```go
	retbytes, err := driver.RPCCall("driver:gcloud.getKubeCfg", cfg)
	if err != nil {
		log.Panicf("[%s] failed call gcloud to get kubeconfig %+v", driver.Service, err)
	}
```

To implement a RPC method, we add a special RPCHandlers hook. the function implements the method needs have a
signature like below. The *driver.RPCError* is used to return error, and *driver.RPCReturn* and *driver.RPCReturnRaw*
is used to return data to the caller.
```go
func main() {
  ...
	driver.RPCHandlers["decrypt"] = decrypt
  ...
}

func decrypt(from string, rpcID string, payload []byte) {
  ...
	if err != nil {
		driver.RPCError(from, rpcID, "failed to create kms client")
	}
  ...
	driver.RPCReturnRaw(from, rpcID, resp.Plaintext)
```

## Driver Options

As mentioned earlier, the driver receives the options/configuratins from daemon automatically through the
helper object. As the data is stored in hashmap, the helper method *driver.GetOption* will accept a path and return an
*Interface()* ojbect.  The path is a dot separated key names traverse into the data structure. If the returned data is
also a map, you can use *dipper.GetMapData* or *dipper.GetMapDataStr* to retrive information from them as well.
If you are sure the data is a *string*, you can use *driver.GetOptionStr* to directly receive it as *string*.

The helper functions follow the golang idiologic style, that returns the value along with a bool to indicate if it is
acceptable or not. See below for example.
```go
	NewAddr, ok := driver.GetOptionStr("data.Addr")
	if !ok {
		NewAddr = ":8080"
	}
```
```go
	hooksObj, ok := driver.GetOption("dynamicData.collapsedEvents")
  ...
  somedata, ok := dipper.GetMapDataStr(hooksObj, "key1.subkey")
  ...
```

There is always a *data* section in the driver options, which comes from the configuration file like below
```yaml
---
...
drivers:
  webhook:
    Addr: :880
...
```

For event receivers, there is a "dynamicData.collapsedEvents" section that stores a mapping between event names to their
conditions, driver uses this to determine if an event should be fired or not. For example
```go
func hookHandler(w http.ResponseWriter, r *http.Request) {
	eventData := extractEventData(w, r)

	matched := []string{}
	for SystemEvent, hook := range hooks {
		for _, condition := range hook.([]interface{}) {
			if dipper.CompareAll(eventData, condition) {
				matched = append(matched, SystemEvent)
				break
			}
		}
	}

	if len(matched) > 0 {
		payload := map[string]interface{}{
			"events": matched,
			"data":   eventData,
		}

		driver.SendMessage("eventbus", "message", payload)

		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "Done\n")
		return
	}

	http.NotFound(w, r)
}
```
The helper function *dipperCompareAll* will try to match your event data to the conditions.
