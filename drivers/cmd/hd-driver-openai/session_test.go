// session_test.go: Unit tests for session.go using mockgen-generated mocks.
package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	mockhd "github.com/honeydipper/honeydipper/drivers/cmd/hd-driver-openai/mock_hd-driver-openai"
	"github.com/honeydipper/honeydipper/drivers/pkg/ai/mock_ai"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
)

func TestOpenAISession_Stream_Content(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3_5-turbo").AnyTimes()
	wrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	var opts map[string]interface{}
	dipper.Must(json.Unmarshal([]byte(`
	{
		"data": {
			"engine": {
				"gpt-3_5-turbo": {
					"base_url": "https://api.openai.com/v1",
					"api_key": "testkey",
					"model": "gpt-3.5-turbo",
					"temperature": 0.7,
					"streaming": true
				}
			},
			"tools_list": [
				{
					"name": "example_tool",
					"description": "An example tool for testing.",
					"parameters": {
						"type": "object",
						"properties": {
							"input": {
								"type": "string",
								"description": "Input for the tool."
							}
						},
						"required": ["input"]
					}
				}
			]
		}
	}
	`), &opts))
	opts["data"].(map[string]interface{})["tools_list"] = []openai.ChatCompletionToolUnionParam{
		{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        "example_tool",
					Description: openai.String("An example tool for testing."),
					Parameters: openai.FunctionParameters{
						"type": "object",
						"properties": map[string]any{
							"input": map[string]any{
								"type":        "string",
								"Description": "Input for the tool.",
							},
						},
						"required": []string{"input"},
					},
				},
			},
		},
	}

	drv := &dipper.Driver{Options: opts}
	msg := &dipper.Message{Payload: map[string]interface{}{}}

	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	sess := newSession(drv, msg, wrapper).(*openAISession)

	// Prepare mock streamer (using mockgen-generated mock)
	mockStr := mockhd.NewMockStreamer(ctrl)
	chunks := []openai.ChatCompletionChunk{
		{
			Choices: []openai.ChatCompletionChunkChoice{
				{Delta: openai.ChatCompletionChunkChoiceDelta{Content: "Hello"}},
			},
		},
		{
			Choices: []openai.ChatCompletionChunkChoice{
				{Delta: openai.ChatCompletionChunkChoiceDelta{Content: " world"}},
			},
		},
	}
	idx := 0
	mockStr.EXPECT().Next().DoAndReturn(func() bool {
		if idx < len(chunks) {
			idx++

			return true
		}

		return false
	}).Times(len(chunks) + 1)
	mockStr.EXPECT().Current().DoAndReturn(func() openai.ChatCompletionChunk {
		return chunks[idx-1]
	}).AnyTimes()

	// Patch _getStreamerFn
	orig := _getStreamerFn
	defer func() { _getStreamerFn = orig }()
	_getStreamerFn = func(_ *openai.ChatCompletionService, _ context.Context, _ openai.ChatCompletionNewParams) Streamer {
		return mockStr
	}

	hist, _ := json.Marshal([]openai.ChatCompletionMessageParamUnion{})
	var got string
	streamHandler := func(s string, done bool) { got += s }
	toolCallHandler := func(_ string, _ map[string]any, _ string, _ string) {}

	sess.Stream(openai.UserMessage("hi"), hist, streamHandler, toolCallHandler)

	assert.Equal(t, "Hello world", got)
}

func TestOpenAISession_InitMessages(t *testing.T) {
	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3_5-turbo").AnyTimes()
	wrapper.EXPECT().Context().Return(context.Background()).AnyTimes()
	drv := &dipper.Driver{Options: map[string]interface{}{
		"data": map[string]interface{}{
			"engine": map[string]interface{}{
				"gpt-3_5-turbo": map[string]interface{}{
					"system_prompt": "You are a helpful AI.",
				},
			},
		},
	}}

	sess := newSession(drv, &dipper.Message{Payload: map[string]interface{}{}}, wrapper).(*openAISession)
	msgs := sess.InitMessages("gpt-3_5-turbo")
	assert.Len(t, msgs, 1)
}

func TestOpenAISession_BuildUserMessage(t *testing.T) {
	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3_5-turbo").AnyTimes()
	drv := &dipper.Driver{Options: map[string]interface{}{}}
	sess := newSession(drv, &dipper.Message{Payload: map[string]interface{}{}}, wrapper).(*openAISession)
	msg := sess.BuildUserMessage("bob", "hi")
	m, ok := msg.(openai.ChatCompletionMessageParamUnion)
	assert.True(t, ok)
	assert.Equal(t, "hi", m.OfUser.Content.OfString.Value)
	assert.Equal(t, "bob", m.OfUser.Name.Value)
}

func TestOpenAISession_Stream_FunctionCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3_5-turbo").AnyTimes()
	wrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	// Set streaming to false in driver options
	opts := map[string]interface{}{
		"data": map[string]interface{}{
			"engine": map[string]interface{}{
				"gpt-3_5-turbo": map[string]interface{}{
					"streaming": true,
				},
			},
		},
	}
	drv := &dipper.Driver{Options: opts}
	msg := &dipper.Message{Payload: map[string]interface{}{}}

	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	sess := newSession(drv, msg, wrapper).(*openAISession)

	chunk := openai.ChatCompletionChunk{}
	chunk.UnmarshalJSON([]byte(`
	{
		"choices":
		[{
			"delta": {
				"tool_calls": [
					{
						"index": 0,
						"id": "tool-call-123",
						"type": "function",
						"function": {
							"name": "function-name",
							"arguments": "{\"arg1\":\"value1\"}"
						}
					}
				]
			}
		}]

	}`))

	endChunk := openai.ChatCompletionChunk{}
	endChunk.UnmarshalJSON([]byte(`
	{
		"choices":
		[{
			"delta": {}
		}]
	}`))

	// Prepare mock streamer (using mockgen-generated mock)
	mockStr := mockhd.NewMockStreamer(ctrl)
	chunks := []openai.ChatCompletionChunk{chunk, endChunk}

	idx := 0
	mockStr.EXPECT().Next().DoAndReturn(func() bool {
		if idx < len(chunks) {
			idx++

			return true
		}

		return false
	}).Times(len(chunks))
	mockStr.EXPECT().Current().DoAndReturn(func() openai.ChatCompletionChunk {
		return chunks[idx-1]
	}).AnyTimes()

	// Patch _getStreamerFn
	orig := _getStreamerFn
	defer func() { _getStreamerFn = orig }()
	_getStreamerFn = func(_ *openai.ChatCompletionService, _ context.Context, _ openai.ChatCompletionNewParams) Streamer {
		return mockStr
	}

	hist, _ := json.Marshal([]openai.ChatCompletionMessageParamUnion{})
	var toolCallData string
	toolCallHandler := func(jsonMessage string, args map[string]any, toolName, toolID string) {
		toolCallData = toolName + ":" + toolID
	}
	streamHandler := func(s string, done bool) {}

	sess.Stream(openai.UserMessage("call function"), hist, streamHandler, toolCallHandler)

	assert.Equal(t, "function-name:tool-call-123", toolCallData) // Replace with expected tool name and ID
}

func TestOpenAISession_BuildToolReturnMessage(t *testing.T) {
	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3_5-turbo").AnyTimes()
	drv := &dipper.Driver{Options: map[string]interface{}{}}
	sess := newSession(drv, &dipper.Message{Payload: map[string]interface{}{}}, wrapper).(*openAISession)
	b := []byte(`{"result":42}`)
	msg := sess.BuildToolReturnMessage("mytool", "callid123", b)
	m, ok := msg.(openai.ChatCompletionMessageParamUnion)
	assert.True(t, ok)
	assert.Equal(t, "callid123", m.OfTool.ToolCallID)
}

func TestNewSession_Basic(t *testing.T) {
	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3.5-turbo").AnyTimes()
	drv := &dipper.Driver{Options: map[string]interface{}{}}
	msg := &dipper.Message{Payload: map[string]interface{}{}}

	sess := newSession(drv, msg, wrapper)
	assert.NotNil(t, sess)
}

func TestOpenAISession_Relay_NonStreamingContent(t *testing.T) {
	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3.5-turbo").AnyTimes()
	wrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	drv := &dipper.Driver{Options: map[string]any{}}
	msg := &dipper.Message{Payload: map[string]interface{}{
		"temperature": 0.5,
	}}
	sess := newSession(drv, msg, wrapper).(*openAISession)

	// Patch _getCompletionFn to return a mock response
	orig := _getCompletionFn
	defer func() { _getCompletionFn = orig }()
	_getCompletionFn = func(_ *openai.ChatCompletionService, _ context.Context, _ openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		return &openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "Non-streamed response",
					},
					FinishReason: "stop",
				},
			},
		}, nil
	}

	hist, _ := json.Marshal([]openai.ChatCompletionMessageParamUnion{})
	var got string
	streamHandler := func(s string, done bool) { got += s }
	toolCallHandler := func(_ string, _ map[string]any, _ string, _ string) {}

	sess.Stream(openai.UserMessage("hi"), hist, streamHandler, toolCallHandler)

	assert.Equal(t, "Non-streamed response", got)
}

func TestOpenAISession_Stream_NonStreamingFunctionCall(t *testing.T) {
	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3.5-turbo").AnyTimes()
	wrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	// Set streaming to false in driver options
	opts := map[string]interface{}{
		"data": map[string]interface{}{
			"engine": map[string]interface{}{
				"gpt-3_5-turbo": map[string]interface{}{
					"streaming": false,
				},
			},
		},
	}
	drv := &dipper.Driver{Options: opts}
	msg := &dipper.Message{Payload: map[string]interface{}{}}
	sess := newSession(drv, msg, wrapper).(*openAISession)

	// Patch _getCompletionFn to return a mock response with a tool call
	orig := _getCompletionFn
	defer func() { _getCompletionFn = orig }()
	_getCompletionFn = func(_ *openai.ChatCompletionService, _ context.Context, _ openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		return &openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						ToolCalls: []openai.ChatCompletionMessageToolCallUnion{
							{
								ID:   "tool-call-456",
								Type: "function",
								Function: openai.ChatCompletionMessageFunctionToolCallFunction{
									Name:      "my_function",
									Arguments: `{"foo":"bar"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}, nil
	}

	hist, _ := json.Marshal([]openai.ChatCompletionMessageParamUnion{})
	var gotToolName, gotToolID string
	var gotArgs map[string]any
	toolCallHandler := func(_ string, args map[string]any, toolName, toolID string) {
		gotToolName = toolName
		gotToolID = toolID
		gotArgs = args
	}
	streamHandler := func(s string, done bool) {}

	sess.Stream(openai.UserMessage("call function"), hist, streamHandler, toolCallHandler)

	assert.Equal(t, "my_function", gotToolName)
	assert.Equal(t, "tool-call-456", gotToolID)
	assert.Equal(t, map[string]any{"foo": "bar"}, gotArgs)
}

func TestOpenAISession_BuildMessage(t *testing.T) {
	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3.5-turbo").AnyTimes()
	drv := &dipper.Driver{Options: map[string]interface{}{}}
	sess := newSession(drv, &dipper.Message{Payload: map[string]interface{}{}}, wrapper).(*openAISession)

	msg := sess.BuildMessage("hello world")
	m, ok := msg.(openai.ChatCompletionMessageParamUnion)
	assert.True(t, ok)
	assert.Equal(t, "hello world", m.OfAssistant.Content.OfString.Value)
}

func Test_getStreamerFn_Default(t *testing.T) {
	// This test creates a real ChatCompletionService and checks _getStreamerFn returns a Streamer.
	client := openai.NewClient()
	service := &client.Chat.Completions
	ctx := context.Background()
	body := openai.ChatCompletionNewParams{}
	streamer := _getStreamerFn(service, ctx, body)
	assert.NotNil(t, streamer)
}

func Test_getCompletionFn_Default(t *testing.T) {
	// This test creates a real ChatCompletionService and checks _getCompletionFn returns a ChatCompletion (or error).
	client := openai.NewClient()
	service := &client.Chat.Completions
	ctx := context.Background()
	body := openai.ChatCompletionNewParams{
		Model: shared.ChatModelChatgpt4oLatest,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hello!"),
		},
	}
	resp, err := _getCompletionFn(service, ctx, body)
	// We expect either a valid response or an error (e.g., due to missing API key).
	assert.True(t, resp != nil || err != nil)
}

func TestOpenAISession_Stream_RefusalChunk(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3_5-turbo").AnyTimes()
	wrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	opts := map[string]interface{}{
		"data": map[string]interface{}{
			"engine": map[string]interface{}{
				"gpt-3_5-turbo": map[string]interface{}{
					"streaming": true,
				},
			},
		},
	}
	drv := &dipper.Driver{Options: opts}
	msg := &dipper.Message{Payload: map[string]interface{}{}}

	devnull := dipper.Must(os.OpenFile(os.DevNull, os.O_WRONLY, 0)).(*os.File)
	dipper.GetLogger("test", "debug", devnull, devnull)

	sess := newSession(drv, msg, wrapper).(*openAISession)

	// Prepare a refusal chunk (e.g., OpenAI may refuse to answer certain prompts)
	refusalChunk := openai.ChatCompletionChunk{}
	refusalChunk.UnmarshalJSON([]byte(`
    {
        "choices": [{
            "delta": {
                "refusal": "I'm sorry, I can't assist with that."
            }
        }]
    }`))

	endChunk := openai.ChatCompletionChunk{}
	endChunk.UnmarshalJSON([]byte(`
    {
        "choices": [{
            "delta": {}
        }]
    }`))

	mockStr := mockhd.NewMockStreamer(ctrl)
	chunks := []openai.ChatCompletionChunk{refusalChunk, endChunk}
	idx := 0
	mockStr.EXPECT().Next().DoAndReturn(func() bool {
		if idx < len(chunks) {
			idx++
			return true
		}
		return false
	}).Times(len(chunks))
	mockStr.EXPECT().Current().DoAndReturn(func() openai.ChatCompletionChunk {
		return chunks[idx-1]
	}).AnyTimes()

	orig := _getStreamerFn
	defer func() { _getStreamerFn = orig }()
	_getStreamerFn = func(_ *openai.ChatCompletionService, _ context.Context, _ openai.ChatCompletionNewParams) Streamer {
		return mockStr
	}

	hist, _ := json.Marshal([]openai.ChatCompletionMessageParamUnion{})
	var got string
	streamHandler := func(s string, done bool) { got += s }
	toolCallHandler := func(_ string, _ map[string]any, _ string, _ string) {}

	sess.Stream(openai.UserMessage("refusal test"), hist, streamHandler, toolCallHandler)

	assert.Equal(t, "I'm sorry, I can't assist with that.", got)
}
