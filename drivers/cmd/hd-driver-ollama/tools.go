package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/ollama/ollama/api"
)

type ToolSpec struct {
	Tool     api.Tool        `json:"tool" mapstructure:"tool"`
	Workflow config.Workflow `json:"workflow" mapstructure:"workflow"`
}

func setupTools() {
	dipper.Logger.Debugf("ollama options: %+v", driver.Options)
	t, ok := driver.GetOption("data.tools")
	if !ok {
		return
	}

	tools := map[string]ToolSpec{}
	for name, spec := range t.(map[string]any) {
		var tool ToolSpec
		dipper.Must(mapstructure.Decode(spec, &tool))
		tools[name] = tool
	}

	driver.Options.(map[string]any)["data"].(map[string]any)["tools"] = tools
	dipper.Logger.Debugf("ollama options after : %+v", driver.Options)
}

func attachTools(req *api.ChatRequest) {
	specsData, hasTools := dipper.GetMapData(driver.Options, "data.tools")
	if !hasTools {
		return
	}
	specs := specsData.(map[string]ToolSpec)
	for name, spec := range specs {
		dipper.Logger.Debugf("Adding tool: %s\n%+v", name, specs[name])
		req.Tools = append(req.Tools, spec.Tool)
	}
}

func reportToolError(wrapper *chatWrapper, name, tmpl string, args ...any) {
	errMsg := api.Message{
		Role: "tool",
		Content: string(dipper.Must(json.Marshal(map[string]any{
			"function": name,
			"error":    fmt.Sprintf(tmpl, args...),
		})).([]byte)),
	}
	wrapper.chat.Messages = append(wrapper.chat.Messages, errMsg)
	dipper.Must(driver.CallNoWait("cache", "rpush", map[string]any{
		"key":   wrapper.prefix + "history",
		"value": string(dipper.Must(json.Marshal(errMsg)).([]byte)),
	}))
}

func handleToolCalls(wrapper *chatWrapper, resp *api.ChatResponse) bool {
	if len(resp.Message.ToolCalls) == 0 {
		return false
	}

	dipper.Must(driver.CallNoWait("cache", "rpush", map[string]any{
		"key":   wrapper.prefix + "history",
		"value": string(dipper.Must(json.Marshal(resp.Message)).([]byte)),
	}))

	calls := sync.WaitGroup{}
	for _, toolCall := range resp.Message.ToolCalls {
		dipper.Logger.Debugf("Getting message from ai: %+v", resp)

		wrapper.chat.Messages = append(wrapper.chat.Messages, resp.Message)
		spec := dipper.MustGetMapData(driver.Options, "data.tools").(map[string]ToolSpec)[toolCall.Function.Name]

		id := driver.EmitEvent(map[string]any{
			"do":   spec.Workflow,
			"data": toolCall.Function.Arguments,
		})

		calls.Add(1)
		go func(id string, toolCall api.ToolCall) {
			defer calls.Done()
			defer func() {
				if r := recover(); r != nil {
					reportToolError(wrapper, toolCall.Function.Name, "local error: %s", r)
				}
			}()

			dipper.Logger.Debugf("Waiting for event ID to finish %s", id)
			b := dipper.Must(driver.CallWithMessage(&dipper.Message{
				Labels: map[string]string{
					"feature": "cache",
					"method":  "blpop",
					"timeout": "15m",
				},
				Payload: map[string]any{"key": "honeydipper/result/" + id},
			})).([]byte)
			dipper.Logger.Debugf("Got result from ai: %+v", string(b))

			toolResponse := api.Message{
				Role:    "tool",
				Content: fmt.Sprintf(`{"function": "%s", "return": %s}`, toolCall.Function.Name, string(b)),
			}
			wrapper.chat.Messages = append(wrapper.chat.Messages, toolResponse)
			dipper.Must(driver.CallNoWait("cache", "rpush", map[string]any{
				"key":   wrapper.prefix + "history",
				"value": string(dipper.Must(json.Marshal(toolResponse)).([]byte)),
			}))
		}(id, toolCall)
	}
	calls.Wait()
	dipper.Must(wrapper.client.Chat(wrapper.ctx, wrapper.chat, wrapper.streamReceiver))

	return true
}
