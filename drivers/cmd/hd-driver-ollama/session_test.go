package main

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/v3/drivers/pkg/ai/mock_ai"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
)

type MockOllamaClient struct {
	responses []api.ChatResponse
	err       error
}

func (m *MockOllamaClient) Chat(ctx context.Context, req *api.ChatRequest, fn api.ChatResponseFunc) error {
	if m.err != nil {
		return m.err
	}
	for _, resp := range m.responses {
		if err := fn(resp); err != nil {
			return err
		}
	}

	return nil
}

func TestNewSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	driver := &dipper.Driver{
		Options: map[string]interface{}{
			"data": map[string]interface{}{
				"engine": map[string]interface{}{
					"test-model": map[string]interface{}{
						"model":       "llama2",
						"temperature": 0.7,
					},
				},
				"tools_list": []api.Tool{
					{
						Type: "function",
						Function: api.ToolFunction{
							Name:        "test_tool",
							Description: "A test tool",
							Parameters: struct {
								Type       string   `json:"type"`
								Defs       any      `json:"$defs,omitempty"`
								Items      any      `json:"items,omitempty"`
								Required   []string `json:"required"`
								Properties map[string]struct {
									Type        api.PropertyType `json:"type"`
									Items       any              `json:"items,omitempty"`
									Description string           `json:"description"`
									Enum        []any            `json:"enum,omitempty"`
								} `json:"properties"`
							}{
								Type: "object",
								Properties: map[string]struct {
									Type        api.PropertyType `json:"type"`
									Items       any              `json:"items,omitempty"`
									Description string           `json:"description"`
									Enum        []any            `json:"enum,omitempty"`
								}{
									"param1": {
										Type:        api.PropertyType{"string"},
										Description: "Test parameter",
									},
								},
								Required: []string{"param1"},
							},
						},
					},
				},
			},
		},
	}

	mockWrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	mockWrapper.EXPECT().Engine().Return("test-model").AnyTimes()
	mockWrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	tests := []struct {
		name        string
		msg         *dipper.Message
		wantModel   string
		wantTemp    float64
		wantHost    string
		wantToolLen int
	}{
		{
			name:        "default configuration",
			msg:         &dipper.Message{},
			wantModel:   "llama2",
			wantTemp:    0.7,
			wantToolLen: 1,
		},
		{
			name: "with custom temperature",
			msg: &dipper.Message{
				Payload: map[string]interface{}{
					"temperature": 0.9,
				},
			},
			wantModel:   "llama2",
			wantTemp:    0.9,
			wantToolLen: 1,
		},
		{
			name: "with custom host",
			msg: &dipper.Message{
				Payload: map[string]interface{}{
					"ollama_host": "http://localhost:11434",
				},
			},
			wantModel:   "llama2",
			wantTemp:    0.7,
			wantToolLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := newSession(driver, tt.msg, mockWrapper)
			ollamaSession := session.(*ollamaSession)

			assert.Equal(t, tt.wantModel, ollamaSession.model)
			if tt.wantTemp > 0 {
				temp, ok := ollamaSession.chatOptions["temperature"]
				assert.True(t, ok)
				assert.Equal(t, tt.wantTemp, temp)
			}
			assert.Equal(t, tt.wantToolLen, len(ollamaSession.chat.Tools))
		})
	}
}

func TestStream(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	mockWrapper.EXPECT().Engine().Return("test-model").AnyTimes()
	mockWrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	mockClient := &MockOllamaClient{
		responses: []api.ChatResponse{
			{
				Message: api.Message{
					Role:    "assistant",
					Content: "test response",
				},
				Done: true,
			},
		},
	}

	session := &ollamaSession{
		driver:  &dipper.Driver{},
		wrapper: mockWrapper,
		client:  mockClient,
		chat:    &api.ChatRequest{},
	}

	var streamedContent string
	var isDone bool
	streamHandler := func(content string, done bool) {
		streamedContent = content
		isDone = done
	}

	toolCallHandler := func(msg string, args map[string]any, name string, id string) {
		// Tool call handling logic for testing
	}

	hist := []byte(`[{"role":"system","content":"test message"}]`)
	msg := &api.Message{
		Role:    "user",
		Content: "test request",
	}

	session.Stream(msg, hist, streamHandler, toolCallHandler)

	assert.Equal(t, 3, len(session.messages))
	assert.Equal(t, "test response", streamedContent)
	assert.True(t, isDone)
}

func TestStreamWithToolCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	toolCall := api.ToolCall{
		Function: api.ToolCallFunction{
			Name: "test_function",
			Arguments: map[string]any{
				"param1": "value",
			},
		},
	}

	mockClient := &MockOllamaClient{
		responses: []api.ChatResponse{
			{
				Message: api.Message{
					Role:      "assistant",
					ToolCalls: []api.ToolCall{toolCall},
				},
				Done: true,
			},
		},
	}

	mockWrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	mockWrapper.EXPECT().Engine().Return("test-model").AnyTimes()
	mockWrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	session := &ollamaSession{
		driver:  &dipper.Driver{},
		wrapper: mockWrapper,
		client:  mockClient,
		chat:    &api.ChatRequest{},
	}

	streamHandler := func(content string, done bool) {
		t.Error("Stream handler should not be called for tool calls")
	}

	var toolCallMsg string
	var toolCallArgs map[string]any
	var toolCallName string
	toolCallHandler := func(msg string, args map[string]any, name string, id string) {
		toolCallMsg = msg
		toolCallArgs = args
		toolCallName = name
	}

	hist := []byte(`[{"role":"user","content":"test message"}]`)
	msg := &api.Message{
		Role:    "assistant",
		Content: "test response",
	}

	session.Stream(msg, hist, streamHandler, toolCallHandler)

	assert.NotEmpty(t, toolCallMsg)
	assert.Equal(t, "test_function", toolCallName)
	assert.Equal(t, "value", toolCallArgs["param1"])
}

func TestInitMessages(t *testing.T) {
	driver := &dipper.Driver{
		Options: map[string]interface{}{
			"data": map[string]interface{}{
				"engine": map[string]interface{}{
					"test-model": map[string]interface{}{
						"system_prompt": "You are a helpful assistant",
					},
				},
			},
		},
	}

	session := &ollamaSession{
		driver: driver,
	}

	messages := session.InitMessages("test-model")
	assert.Equal(t, 1, len(messages))
	msg := messages[0].(*api.Message)
	assert.Equal(t, "system", msg.Role)
	assert.Equal(t, "You are a helpful assistant", msg.Content)
}

func TestBuildMessage(t *testing.T) {
	session := &ollamaSession{}
	msg := session.BuildMessage("test message")
	assert.Equal(t, "assistant", msg.(*api.Message).Role)
	assert.Equal(t, "test message", msg.(*api.Message).Content)
}

func TestBuildUserMessage(t *testing.T) {
	session := &ollamaSession{}
	msg := session.BuildUserMessage("testuser", "hello")
	assert.Equal(t, "user", msg.(*api.Message).Role)
	assert.Equal(t, "testuser says :quote_start: hello :quote_end", msg.(*api.Message).Content)
}

func TestBuildToolReturnMessage(t *testing.T) {
	session := &ollamaSession{}
	returnData := []byte(`{"result": "success"}`)
	msg := session.BuildToolReturnMessage("test_function", "123", returnData)
	assert.Equal(t, "tool", msg.(*api.Message).Role)

	expectedContent := `{"function": "test_function", "return": {"result": "success"}}`
	assert.Equal(t, expectedContent, msg.(*api.Message).Content)
}
