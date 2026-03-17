// Copyright 2025 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package hd-driver-ollama enables Honeydipper to use ollama to run AI models.
package main

// Required imports for the driver functionality.
import (
	"flag"
	"fmt"
	"os"

	"github.com/honeydipper/honeydipper/v3/drivers/pkg/ai"
	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/ollama/ollama/api"
)

// initFlags sets up command line flags and usage information.
func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports operator service.")
		fmt.Printf("  This program provides honeydipper with capability of running AI models using ollama API.")
	}
}

// ToolSpec defines the structure for tool specifications including workflow configuration.
type ToolSpec struct {
	Tool     api.Tool        `json:"tool" mapstructure:"tool"`
	Workflow config.Workflow `json:"workflow" mapstructure:"workflow"`
}

// Global driver instance.
var driver *dipper.Driver

// main initializes and runs the driver.
func main() {
	initFlags()
	flag.Parse()

	initDriver()
	driver.Run()
}

// initDriver sets up the driver with required commands and handlers.
func initDriver() {
	driver = dipper.NewDriver(os.Args[1], "ollama")
	driver.Commands["chat"] = chat
	driver.Commands["chatContinue"] = func(m *dipper.Message) { ai.ChatContinue(driver, m) }
	driver.Commands["chatStop"] = func(m *dipper.Message) { ai.ChatStop(driver, m) }
	driver.Commands["chatListen"] = func(m *dipper.Message) { ai.ChatListen(driver, m, (&ollamaSession{}).BuildUserMessage) }
	driver.Start = setup
}

// chat handles new chat sessions with the AI model.
func chat(m *dipper.Message) {
	wrapper := ai.NewWrapper(driver, m, func(w ai.ChatWrapperInterface) ai.Chatter {
		return newSession(driver, m, w)
	})
	wrapper.ChatRelay(m)
}

// setup initializes the driver by loading and processing tool configurations.
func setup(_ *dipper.Message) {
	toolMap := map[string]any{}
	if toolData, ok := driver.GetOption("data.tools"); ok {
		toolMap, _ = toolData.(map[string]any)
	}
	tools := []api.Tool{}
	// Process each tool specification.
	for k, v := range toolMap {
		toolSpec := ToolSpec{}
		if mapstructure.Decode(v, &toolSpec) == nil {
			toolMap[k].(map[string]any)["workflow"] = &toolSpec.Workflow
			tools = append(tools, toolSpec.Tool)
		}
		dipper.Logger.Debugf("tools loaded: %s", k)
	}

	// Store processed tools in driver options.
	if data, ok := driver.GetOption("data"); ok {
		data.(map[string]any)["tools_list"] = tools
	} else {
		data = map[string]any{"tools_list": tools}
		driver.Options.(map[string]any)["data"] = data
	}
}
