// Package main provides a Gemini AI driver for Honeydipper.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/honeydipper/honeydipper/drivers/pkg/ai"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/genai"
)

// initFlags sets up command line flags and usage information.
func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports operator service.")
		fmt.Printf("  This program provides honeydipper with capability of access Gemini AI models using genAI API.")
	}
}

// gemini represents the Gemini AI driver.
type gemini struct {
	driver *dipper.Driver
}

// ToolSpec defines the structure for tool specifications combining Gemini tools with workflows.
type ToolSpec struct {
	Tool     *genai.Tool     `json:"tool" mapstructure:"tool"`
	Workflow config.Workflow `json:"workflow" mapstructure:"workflow"`
}

// newGemini creates and initializes a new Gemini driver instance.
func newGemini() *gemini {
	this := &gemini{}
	this.driver = dipper.NewDriver(os.Args[1], "gemini")

	return this
}

// Global instance of the Gemini driver.
var gem *gemini

// main is the entry point for the Gemini driver.
func main() {
	initFlags()
	flag.Parse()

	gem = newGemini()

	// Register command handlers.
	gem.driver.Commands["chat"] = gem.chat
	gem.driver.Commands["chatContinue"] = func(m *dipper.Message) { ai.ChatContinue(gem.driver, m) }
	gem.driver.Commands["chatStop"] = func(m *dipper.Message) { ai.ChatStop(gem.driver, m) }
	gem.driver.Commands["chatListen"] = func(m *dipper.Message) { ai.ChatListen(gem.driver, m, (&gemSession{}).BuildUserMessage) }
	gem.driver.Start = gem.init
	gem.driver.Run()
}

// init initializes the driver by loading and processing tool configurations.
func (g *gemini) init(_ *dipper.Message) {
	toolMap := map[string]any{}
	if toolData, ok := g.driver.GetOption("data.tools"); ok {
		toolMap, _ = toolData.(map[string]any)
	}
	tools := []*genai.Tool{}
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
	if data, ok := g.driver.GetOption("data"); ok {
		data.(map[string]any)["tools_list"] = tools
	} else {
		data = map[string]any{"tools_list": tools}
		g.driver.Options.(map[string]any)["data"] = data
	}
}

// chat handles the chat command by creating a new chat session and relaying messages.
func (g *gemini) chat(msg *dipper.Message) {
	wrapper := ai.NewWrapper(g.driver, msg, func(w ai.ChatWrapperInterface) ai.Chatter {
		return newSession(g.driver, msg, w)
	})
	wrapper.ChatRelay(msg)
}
