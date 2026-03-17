// Package main implements an Ollama chat client integration for Honeydipper.
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/honeydipper/honeydipper/v3/drivers/pkg/ai"
	"github.com/honeydipper/honeydipper/v3/drivers/pkg/ollamahelper"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/ollama/ollama/api"
)

// IsChatStream indicates if this driver supports streaming chat responses.
var IsChatStream = true

// OllamaClientInterface defines the interface for interacting with the Ollama API.
type OllamaClientInterface interface {
	Chat(context.Context, *api.ChatRequest, api.ChatResponseFunc) error
}

// NewOllamaClient creates a new Ollama client using the provided host or environment settings.
func NewOllamaClient(ollamaHost string) (OllamaClientInterface, error) {
	c, e := ollamahelper.NewOllamaClient(ollamaHost)
	if e != nil {
		e = fmt.Errorf("error creating ollama client: %w", e)
	}

	return c, e
}

// ollamaSession represents an active chat session with the Ollama model.
type ollamaSession struct {
	driver  *dipper.Driver
	wrapper ai.ChatWrapperInterface

	client      OllamaClientInterface
	chat        *api.ChatRequest
	model       string
	chatOptions map[string]any
	messages    []api.Message
}

// newSession creates a new chat session with the configured parameters.
func newSession(driver *dipper.Driver, msg *dipper.Message, wrapper ai.ChatWrapperInterface) ai.Chatter {
	s := &ollamaSession{
		driver:  driver,
		wrapper: wrapper,
	}

	// Setting client and chat request.
	ollamaHost, _ := dipper.GetMapDataStr(msg.Payload, "ollama_host")
	s.client = dipper.Must(NewOllamaClient(ollamaHost)).(OllamaClientInterface)

	// Setup model configuration.
	s.model = wrapper.Engine()
	if n, ok := driver.GetOption("data.engine." + s.model + ".model"); ok {
		s.model = n.(string)
	}

	// Setup temperature for response randomness.
	t, hasTemp := dipper.GetMapData(msg.Payload, "temperature")
	if hasTemp {
		s.chatOptions = map[string]any{
			"temperature": t.(float64),
		}
	} else if n, ok := s.driver.GetOption("data.engine." + s.wrapper.Engine() + ".temperature"); ok {
		s.chatOptions = map[string]any{
			"temperature": n.(float64),
		}
	}

	// Setup chat request with model and options.
	s.chat = &api.ChatRequest{
		Model:   s.model,
		Options: s.chatOptions,
		Tools:   dipper.MustGetMapData(s.driver.Options, "data.tools_list").([]api.Tool),
	}

	return s
}

// Stream processes chat messages with history and handles streaming responses.
func (s *ollamaSession) Stream(
	msg any,
	hist []byte,
	streamHandler func(string, bool),
	toolCallHandler func(string, map[string]any, string, string),
) {
	dipper.Must(json.Unmarshal(hist, &s.messages))
	s.StreamWithFunctionReturn(msg, streamHandler, toolCallHandler)
}

// StreamWithFunctionReturn handles streaming responses and tool calls from the model.
func (s *ollamaSession) StreamWithFunctionReturn(
	ret any,
	streamHandler func(string, bool),
	toolCallHandler func(string, map[string]any, string, string),
) {
	s.messages = append(s.messages, *ret.(*api.Message))
	s.chat.Messages = s.messages

	var toolCallMessage *api.Message
	dipper.Must(s.client.Chat(s.wrapper.Context(), s.chat, func(response api.ChatResponse) error {
		var ret error
		func() {
			defer func() {
				if r := recover(); r != nil {
					ret = r.(error)
				}
			}()
			if toolCallMessage != nil { // skip the empty message after the tool call request.
				return
			}

			s.messages = append(s.messages, response.Message)

			if response.Message.ToolCalls == nil {
				streamHandler(response.Message.Content, response.Done)
			} else {
				copy := response.Message
				toolCallMessage = &copy
			}
		}()

		return ret
	}))

	if toolCallMessage != nil {
		msg := string(dipper.Must(json.Marshal(toolCallMessage)).([]byte))
		toolCallHandler(msg, toolCallMessage.ToolCalls[0].Function.Arguments, toolCallMessage.ToolCalls[0].Function.Name, "")
	}
}

// InitMessages initializes the chat session with system prompts.
func (s *ollamaSession) InitMessages(engine string) []any {
	var ret []any

	systemPrompt, _ := s.driver.GetOptionStr("data.engine." + engine + ".system_prompt")
	if len(systemPrompt) > 0 {
		ret = append(ret, &api.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	return ret
}

// BuildMessage creates an assistant message with the given text.
func (s *ollamaSession) BuildMessage(text string) any {
	return &api.Message{
		Role:    "assistant",
		Content: text,
	}
}

// BuildUserMessage formats a user message with quotes.
func (s *ollamaSession) BuildUserMessage(user, text string) any {
	return &api.Message{
		Role:    "user",
		Content: fmt.Sprintf("%s says :quote_start: %s :quote_end", user, text),
	}
}

// BuildToolReturnMessage creates a tool response message in JSON format.
func (s *ollamaSession) BuildToolReturnMessage(name string, callID string, b []byte) any {
	return &api.Message{
		Role:    "tool",
		Content: fmt.Sprintf(`{"function": "%s", "return": %s}`, name, string(b)),
	}
}
