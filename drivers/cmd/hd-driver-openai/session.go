// Package main implements an OpenAI chat client integration for Honeydipper.
package main

import (
	"context"
	"encoding/json"

	"github.com/honeydipper/honeydipper/drivers/pkg/ai"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// openAISession represents an active chat session with the openAI models.
type openAISession struct {
	driver  *dipper.Driver
	wrapper ai.ChatWrapperInterface

	client      openai.Client
	chatOptions openai.ChatCompletionNewParams
	messages    []openai.ChatCompletionMessageParamUnion
	isStreaming bool
}

// newSession creates a new chat session with the configured parameters.
func newSession(driver *dipper.Driver, msg *dipper.Message, wrapper ai.ChatWrapperInterface) ai.Chatter {
	s := &openAISession{
		driver:  driver,
		wrapper: wrapper,
	}

	// Setup model configuration.
	modelEntry := wrapper.Engine()
	dipper.Logger.Debugf("using engine: %s", modelEntry)
	if n, ok := driver.GetOption("data.engine." + modelEntry + ".model"); ok {
		s.chatOptions.Model = n.(string)
	} else {
		s.chatOptions.Model = modelEntry
	}

	// Determine if the engine use streaming API.
	if streaming, ok := dipper.GetMapDataBool(driver.Options, "data.engine."+modelEntry+".streaming"); ok {
		s.isStreaming = streaming
	}

	// Setup temperature for response randomness.
	t, hasTemp := dipper.GetMapData(msg.Payload, "temperature")
	if hasTemp {
		s.chatOptions.Temperature = openai.Float(t.(float64))
	} else if n, ok := s.driver.GetOption("data.engine." + modelEntry + ".temperature"); ok {
		s.chatOptions.Temperature = openai.Float(n.(float64))
	}

	// Setup tools.
	if toolsList, ok := dipper.GetMapData(s.driver.Options, "data.tools_list"); ok {
		s.chatOptions.Tools = toolsList.([]openai.ChatCompletionToolUnionParam)
	}

	// Setup client options.
	options := []option.RequestOption{}
	if openaiBaseURL, ok := dipper.GetMapDataStr(s.driver.Options, "data.engine."+modelEntry+".base_url"); ok {
		dipper.Logger.Debugf("(%s) using base url: %s", modelEntry, openaiBaseURL)
		options = append(options, option.WithBaseURL(openaiBaseURL))
	}
	if apiKey, ok := dipper.GetMapDataStr(s.driver.Options, "data.engine."+modelEntry+".api_key"); ok {
		options = append(options, option.WithAPIKey(apiKey))
	}

	// Create new client.
	s.client = openai.NewClient(options...)

	return s
}

// Stream processes chat messages with history and handles streaming responses.
func (s *openAISession) Stream(
	msg any,
	hist []byte,
	streamHandler func(string, bool),
	toolCallHandler func(string, map[string]any, string, string),
) {
	dipper.Must(json.Unmarshal(hist, &s.messages))
	s.StreamWithFunctionReturn(msg, streamHandler, toolCallHandler)
}

// Streamer interface used for mocking in tests.
type Streamer interface {
	Next() bool
	Current() openai.ChatCompletionChunk
}

// _getStreamerFn returns a sse stream to receive streaming chat response chunks.
var (
	_getStreamerFn = func(s *openai.ChatCompletionService, ctx context.Context, body openai.ChatCompletionNewParams) Streamer {
		return s.NewStreaming(ctx, body)
	}
)

// _getCompletionFn fetches the complete chat response (non-streaming).
var (
	_getCompletionFn = func(
		s *openai.ChatCompletionService,
		ctx context.Context,
		body openai.ChatCompletionNewParams,
	) (*openai.ChatCompletion, error) {
		return s.New(ctx, body)
	}
)

// relayToSteam relays a non-streaming chat response into the stream.
func (s *openAISession) relayToStream(
	ret any,
	streamHandler func(string, bool),
	toolCallHandler func(string, map[string]any, string, string),
) {
	s.messages = append(s.messages, ret.(openai.ChatCompletionMessageParamUnion))
	body := s.chatOptions
	body.Messages = s.messages
	dipper.Logger.Debugf("sending chat request :%s", dipper.Must(json.Marshal(body)).([]byte))

	resp := dipper.Must(_getCompletionFn(&s.client.Chat.Completions, s.wrapper.Context(), body)).(*openai.ChatCompletion)

	if resp.Choices[0].FinishReason == "tool_calls" {
		jsonMessage := string(dipper.Must(resp.Choices[0].Message.ToParam().MarshalJSON()).([]byte))
		args := map[string]any{}
		tool := resp.Choices[0].Message.ToolCalls[0]
		dipper.Must(json.Unmarshal([]byte(tool.Function.Arguments), &args))
		toolCallHandler(jsonMessage, args, tool.Function.Name, tool.ID)

		return
	}

	text := resp.Choices[0].Message.Content
	streamHandler(text, true)
}

// StreamWithFunctionReturn handles streaming responses and tool calls from the model.
func (s *openAISession) StreamWithFunctionReturn(
	ret any,
	streamHandler func(string, bool),
	toolCallHandler func(string, map[string]any, string, string),
) {
	if !s.isStreaming {
		s.relayToStream(ret, streamHandler, toolCallHandler)

		return
	}

	s.messages = append(s.messages, ret.(openai.ChatCompletionMessageParamUnion))
	body := s.chatOptions
	body.Messages = s.messages

	dipper.Logger.Debugf("sending chat request :%s", dipper.Must(json.Marshal(body)).([]byte))
	acc := openai.ChatCompletionAccumulator{}
	streamer := _getStreamerFn(&s.client.Chat.Completions, s.wrapper.Context(), body)

	for streamer.Next() {
		chunk := streamer.Current()
		dipper.Logger.Debugf("[openai] got chunk %+v", chunk)
		acc.AddChunk(chunk)

		// if using tool calls
		if tool, ok := acc.JustFinishedToolCall(); ok {
			jsonMessage := string(dipper.Must(acc.Choices[0].Message.ToParam().MarshalJSON()).([]byte))
			args := map[string]any{}
			dipper.Must(json.Unmarshal([]byte(tool.Arguments), &args))
			toolCallHandler(jsonMessage, args, tool.Name, tool.ID)

			return
		}

		// handling refusal.
		if refusal, ok := acc.JustFinishedRefusal(); ok {
			streamHandler(refusal, true)

			return
		}

		// stream content to client.
		_, finished := acc.JustFinishedContent()
		text := ""
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			text = chunk.Choices[0].Delta.Content
		}
		streamHandler(text, finished)
	}
}

// InitMessages initializes the chat session with system prompts.
func (s *openAISession) InitMessages(engine string) []any {
	var ret []any

	systemPrompt, _ := s.driver.GetOptionStr("data.engine." + engine + ".system_prompt")
	if len(systemPrompt) > 0 {
		ret = append(ret, openai.SystemMessage(systemPrompt))
	}

	return ret
}

// BuildMessage creates an assistant message with the given text.
func (s *openAISession) BuildMessage(text string) any {
	return openai.AssistantMessage(text)
}

// BuildUserMessage formats a user message with quotes.
func (s *openAISession) BuildUserMessage(user, text string) any {
	ret := openai.UserMessage(text)
	ret.OfUser.Name = openai.String(user)

	return ret
}

// BuildToolReturnMessage creates a tool response message in JSON format.
func (s *openAISession) BuildToolReturnMessage(name string, callID string, b []byte) any {
	return openai.ToolMessage(string(b), callID)
}
