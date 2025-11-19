// Package main provides a OpenAI chat driver for Honeydipper.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/honeydipper/honeydipper/drivers/pkg/ai"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/openai/openai-go/v3"
)

// initFlags sets up command line flags and usage information.
func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports operator service.")
		fmt.Printf("  This program provides honeydipper with capability of access AI models using openAI API.")
	}
}

// openAIDriver represents the openAI driver.
type openAIDriver struct {
	driver *dipper.Driver
}

// ToolSpec defines the structure for tool specifications combining openAI tools with workflows.
type ToolSpec struct {
	Tool     openai.ChatCompletionToolUnionParam `json:"tool" mapstructure:"tool"`
	Workflow config.Workflow                     `json:"workflow" mapstructure:"workflow"`
}

// newOpenAI creates and initializes a new OpenAI driver instance.
func newOpenAI() *openAIDriver {
	this := &openAIDriver{}
	this.driver = dipper.NewDriver(os.Args[1], "openai")

	return this
}

// openAI is the Global instance of the openAI driver.
var openAI *openAIDriver

// main is the entry point for the openAI driver.
func main() {
	initFlags()
	flag.Parse()

	openAI = newOpenAI()

	// Register command handlers.
	openAI.driver.Commands["chat"] = openAI.chat
	openAI.driver.Commands["chatContinue"] = func(m *dipper.Message) { ai.ChatContinue(openAI.driver, m) }
	openAI.driver.Commands["chatStop"] = func(m *dipper.Message) { ai.ChatStop(openAI.driver, m) }
	openAI.driver.Commands["chatListen"] = func(m *dipper.Message) { ai.ChatListen(openAI.driver, m, (&openAISession{}).BuildUserMessage) }
	openAI.driver.Start = openAI.init
	openAI.driver.Run()
}

// init initializes the driver by loading and processing tool configurations.
func (o *openAIDriver) init(_ *dipper.Message) {
	toolMap := map[string]any{}
	if toolData, ok := o.driver.GetOption("data.tools"); ok {
		toolMap, _ = toolData.(map[string]any)
	}
	tools := []openai.ChatCompletionToolUnionParam{}
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
	if data, ok := o.driver.GetOption("data"); ok {
		data.(map[string]any)["tools_list"] = tools
	} else {
		data = map[string]any{"tools_list": tools}
		o.driver.Options.(map[string]any)["data"] = data
	}
}

// chat handles the chat command by creating a new chat session and relaying messages.
func (o *openAIDriver) chat(msg *dipper.Message) {
	wrapper := ai.NewWrapper(o.driver, msg, func(w ai.ChatWrapperInterface) ai.Chatter {
		return newSession(o.driver, msg, w)
	})
	wrapper.ChatRelay(msg)
}
