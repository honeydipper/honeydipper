// session_test.go: Unit tests for session.go using mockgen-generated mocks.
package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	mockhd "github.com/honeydipper/honeydipper/drivers/cmd/hd-driver-openai/mock_hd-driver-openai"
	"github.com/honeydipper/honeydipper/drivers/pkg/ai/mock_ai"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
)

func TestOpenAISession_Stream_Content(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3_5-turbo").AnyTimes()
	wrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	drv := &dipper.Driver{Options: map[string]interface{}{}}
	msg := &dipper.Message{Payload: map[string]interface{}{}}

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

	drv := &dipper.Driver{Options: map[string]interface{}{}}
	msg := &dipper.Message{Payload: map[string]interface{}{}}

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	wrapper.EXPECT().Engine().Return("gpt-3.5-turbo").AnyTimes()
	drv := &dipper.Driver{Options: map[string]interface{}{}}
	msg := &dipper.Message{Payload: map[string]interface{}{}}

	sess := newSession(drv, msg, wrapper)
	assert.NotNil(t, sess)
}
