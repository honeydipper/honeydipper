package main

import (
	"context"
	"encoding/json"
	"iter"

	"github.com/honeydipper/honeydipper/v3/drivers/pkg/ai"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"google.golang.org/genai"
)

// ChatInterface defines the subset of methods from genai.Chat that we use.
type ChatInterface interface {
	History(bool) []*genai.Content
	SendMessageStream(ctx context.Context, parts ...genai.Part) iter.Seq2[*genai.GenerateContentResponse, error]
}

// gemSession represents a chat session with the Gemini AI model.
type gemSession struct {
	driver  *dipper.Driver
	wrapper ai.ChatWrapperInterface
	cfg     *genai.GenerateContentConfig
	chat    ChatInterface
	client  *genai.Client
	model   string
}

// createChatFn creates a chat instance. Override this variable in tests to inject the mock.
var createChatFn = func(
	client *genai.Client,
	ctx context.Context,
	model string,
	cfg *genai.GenerateContentConfig,
	history []*genai.Content,
) ChatInterface {
	return dipper.Must(client.Chats.Create(ctx, model, cfg, history)).(ChatInterface)
}

// newClientFn creates a new Gemini API client. Override this variable in tests to inject the mock.
var newClientFn = func(ctx context.Context, cfg *genai.ClientConfig) *genai.Client {
	return dipper.Must(genai.NewClient(ctx, cfg)).(*genai.Client)
}

// newSession creates and initializes a new Gemini chat session.
func newSession(d *dipper.Driver, m *dipper.Message, w ai.ChatWrapperInterface) *gemSession {
	s := &gemSession{
		driver:  d,
		wrapper: w,
	}

	s.model = w.Engine()
	if m, ok := dipper.GetMapDataStr(d.Options, "data.engine."+w.Engine()+".model"); ok {
		s.model = m
	}

	s.cfg = s.generateConfig(m)
	s.client = s.newClient()

	return s
}

// newClient creates a new Gemini API client using the provided API token.
func (s *gemSession) newClient() *genai.Client {
	return newClientFn(s.wrapper.Context(), &genai.ClientConfig{})
}

// generateConfig creates the configuration for the Gemini API with temperature and tools settings.
func (s *gemSession) generateConfig(msg *dipper.Message) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{}
	t, hasTemp := dipper.GetMapData(msg.Payload, "temperature")
	if hasTemp {
		temp := float32(t.(float64))
		cfg.Temperature = &temp
	} else if n, ok := s.driver.GetOption("data.engine." + s.wrapper.Engine() + ".temperature"); ok {
		temp := float32(n.(float64))
		cfg.Temperature = &temp
	}
	cfg.Tools = dipper.MustGetMapData(s.driver.Options, "data.tools_list").([]*genai.Tool)

	return cfg
}

// Stream initializes a chat session with history and starts streaming responses.
func (s *gemSession) Stream(
	msg any,
	hist []byte,
	streamHandler func(string, bool),
	toolCallHandler func(string, map[string]any, string, string),
) {
	var messages []*genai.Content
	dipper.Must(json.Unmarshal(hist, &messages))
	s.chat = createChatFn(s.client, s.wrapper.Context(), s.model, s.cfg, messages)

	s.StreamWithFunctionReturn(msg, streamHandler, toolCallHandler)
}

// StreamWithFunctionReturn handles streaming responses and function calls from the AI.
func (s *gemSession) StreamWithFunctionReturn(
	ret any,
	streamHandler func(text string, done bool),
	toolCallHandler func(jsonMessage string, args map[string]any, name string, callID string),
) {
	var toolCallContent *genai.Content
	for resp, err := range s.chat.SendMessageStream(s.wrapper.Context(), *ret.(*genai.Content).Parts[0]) {
		if err != nil {
			dipper.Logger.Warningf("[gemini] error when relaying ai chat response: %v", err)
			s.wrapper.Cancel()

			break
		}
		if len(resp.Candidates[0].Content.Parts) == 0 {
			dipper.Logger.Warningf(
				"[gemini] ai chat response has no parts: %v, %v.",
				resp.Candidates[0].FinishReason,
				resp.Candidates[0].FinishMessage,
			)
			s.wrapper.Cancel()

			break
		}
		if resp.Candidates[0].Content.Parts[0].FunctionCall != nil {
			toolCallContent = resp.Candidates[0].Content

			continue
		}

		streamHandler(resp.Candidates[0].Content.Parts[0].Text, resp.Candidates[0].FinishReason != "")
	}
	if toolCallContent != nil {
		jsonMessage := string(dipper.Must(json.Marshal(toolCallContent)).([]byte))
		name := toolCallContent.Parts[0].FunctionCall.Name
		args := toolCallContent.Parts[0].FunctionCall.Args
		callID := toolCallContent.Parts[0].FunctionCall.ID
		// genai copySanitizedModelContent function will strip all function info making the history
		// no usable.  So we need to save the function call info in the history.
		history := s.chat.History(false)
		history[len(history)-1] = toolCallContent
		s.chat = createChatFn(s.client, s.wrapper.Context(), s.model, s.cfg, history)

		toolCallHandler(jsonMessage, args, name, callID)
	}
}

// InitMessages initializes the chat with system prompts if configured.
func (s *gemSession) InitMessages(engine string) []any {
	var ret []any

	systemPrompt, _ := s.driver.GetOptionStr("data.engine." + s.wrapper.Engine() + ".system_prompt")
	if len(systemPrompt) > 0 {
		ret = append(ret, &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{{Text: systemPrompt}},
		})
		ret = append(ret, &genai.Content{
			Role:  "assistant",
			Parts: []*genai.Part{{Text: "Understood!"}},
		})
	}

	return ret
}

// BuildMessage creates an assistant message with the given text.
func (s *gemSession) BuildMessage(text string) any {
	return &genai.Content{
		Role:  "assistant",
		Parts: []*genai.Part{{Text: text}},
	}
}

// BuildUserMessage formats a user message with quote markers.
func (s *gemSession) BuildUserMessage(user, text string) any {
	return &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: user + " says :quote_start: " + text + ":quote_end:"}},
	}
}

// BuildToolReturnMessage creates a tool response message with function call results.
func (s *gemSession) BuildToolReturnMessage(name, callID string, content []byte) any {
	var data map[string]any
	dipper.Must(json.Unmarshal(content, &data))

	return &genai.Content{
		Role: "tool",
		Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{
			Name:     name,
			Response: data,
			ID:       callID,
		}}},
	}
}
