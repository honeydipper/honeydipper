// Copyright 2025 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package hd-driver-ollama enables Honeydipper to use ollama to run AI models.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/ollama/ollama/api"
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports operator service.")
		fmt.Printf("  This program provides honeydipper with capability of running AI models using ollama API.")
	}
}

var (
	driver     *dipper.Driver
	chatStream bool = true

	ErrCancelled = errors.New("cancelled")
)

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "ollama")
	driver.Commands["chat"] = chat
	driver.Commands["chatContinue"] = chatContinue
	driver.Commands["chatStop"] = chatStop
	driver.Commands["chatListen"] = chatListen
	driver.Start = loadOptions
	driver.Run()
}

func loadOptions(m *dipper.Message) {
	setupTools()
}

func chat(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	// initializing
	convID := dipper.MustGetMapDataStr(msg.Payload, "convID")
	prefix := "ollama/conv/" + convID + "/"
	engine, _ := dipper.GetMapDataStr(msg.Payload, "engine")
	if engine == "" {
		engine = "default"
	}

	// locking both the ollama instance and the conversation
	timeout, _ := dipper.GetMapDataStr(driver.Options, fmt.Sprintf("data.engine.%s.timeout", engine))
	if timeout == "" {
		timeout = "10m"
	}
	ollamaHost, _ := dipper.GetMapDataStr(msg.Payload, "ollama_host")
	ollamaLock := "ollama/lock" + "/" + ollamaHost
	if _, err := driver.Call("locker", "lock", map[string]any{"name": ollamaLock, "expire": timeout}); err != nil {
		msg.Reply <- dipper.Message{Payload: map[string]any{"busy": true}}

		return
	}
	if _, err := driver.Call("locker", "lock", map[string]any{"name": prefix + "lock", "expire": timeout}); err != nil {
		msg.Reply <- dipper.Message{Payload: map[string]any{"busy": true}}

		return
	}

	// obtain step id (turn id) and setup cancallation flag.
	c := dipper.Must(driver.Call("cache", "incr", map[string]any{"key": prefix + "counter"})).([]byte)
	counter := string(c)
	step := prefix + counter
	dipper.Must(driver.Call("cache", "save", map[string]any{"key": step, "value": "1"}))

	// start relay in chatWrapper.
	wrapper := newWrapper(msg, engine, prefix, ollamaHost)
	ctx, _ := wrapper.run(step)

	// schedule cleanup
	go func() {
		defer dipper.SafeExitOnError("[ollama] failed to clean up ai chat session")
		defer func() { dipper.Must(driver.Call("locker", "unlock", map[string]any{"name": ollamaLock})) }()
		defer func() { dipper.Must(driver.Call("cache", "del", map[string]any{"key": step})) }()
		defer func() { dipper.Must(driver.Call("locker", "unlock", map[string]any{"name": prefix + "lock"})) }()
		<-ctx.Done()
		dipper.Logger.Debugf("ollama chat session completed: %+v", ctx.Err())
	}()

	// return
	msg.Reply <- dipper.Message{Payload: map[string]any{"counter": counter, "convID": convID}}
}

func chatContinue(msg *dipper.Message) {
	msg.Reply <- dipper.Message{Labels: map[string]string{"no-timeout": "true"}}

	msg = dipper.DeserializePayload(msg)
	convID := dipper.MustGetMapDataStr(msg.Payload, "convID")
	counter := dipper.MustGetMapDataStr(msg.Payload, "counter")
	timeout := "30s"
	if timeoutSec, ok := msg.Labels["timeout"]; ok && len(timeoutSec) > 0 {
		timeout = timeoutSec + "s"
	}
	step := "ollama/conv/" + convID + "/" + counter

	dipper.Logger.Debugf("chatContinue: %+v", step)

	resp, _ := driver.CallWithMessage(&dipper.Message{
		Labels: map[string]string{
			"feature": "cache",
			"method":  "blpop",
			"timeout": timeout,
		},
		Payload: map[string]any{"key": step + "/response"},
	})

	ret := dipper.Message{}
	if len(resp) == 0 {
		cancelled := len(dipper.Must(driver.CallRaw("cache", "exists", []byte(step))).([]byte)) == 0
		ret.Payload = map[string]any{"done": cancelled, "content": "", "type": ""}
	} else {
		ret.Payload = resp
		ret.IsRaw = true
	}

	msg.Reply <- ret
}

func chatStop(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	convID := dipper.MustGetMapDataStr(msg.Payload, "convID")
	prefix := "ollama/conv/" + convID + "/"

	counter, _ := dipper.GetMapDataStr(msg.Payload, "counter")
	if counter == "" {
		counter = string(dipper.Must(driver.Call("cache", "load", map[string]any{"key": prefix + "counter"})).([]byte))
	}

	step := "ollama/conv/" + convID + "/" + counter
	dipper.Must(driver.CallNoWait("cache", "del", map[string]any{"key": step}))
	msg.Reply <- dipper.Message{}
}

func chatListen(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	convID := dipper.MustGetMapDataStr(msg.Payload, "convID")
	prefix := "ollama/conv/" + convID + "/"
	inConversation := len(dipper.Must(driver.CallRaw("cache", "exists", []byte(prefix+"history"))).([]byte)) > 0
	if !inConversation {
		msg.Reply <- dipper.Message{}

		return
	}

	user := dipper.MustGetMapDataStr(msg.Payload, "user")
	userMessage := api.Message{
		Role:    "user",
		Content: fmt.Sprintf("%s says :start quote: %s\n\n :end quote:", user, dipper.MustGetMapDataStr(msg.Payload, "prompt")),
	}
	jsonUserMessage := string(dipper.Must(json.Marshal(userMessage)).([]byte))

	dipper.Must(driver.CallNoWait("cache", "rpush", map[string]any{"key": prefix + "history", "value": jsonUserMessage}))

	msg.Reply <- dipper.Message{}
}
