package main

import (
	"context"
	"encoding/json"
	"iter"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/v3/drivers/pkg/ai/mock_ai"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

// MockChat implements ChatInterface for testing.
type MockChat struct {
	history     []*genai.Content
	sendResults []*genai.GenerateContentResponse
	sendErrors  []error
}

func (m *MockChat) History(bool) []*genai.Content {
	return m.history
}

func (m *MockChat) SendMessageStream(ctx context.Context, parts ...genai.Part) iter.Seq2[*genai.GenerateContentResponse, error] {
	return func(yield func(*genai.GenerateContentResponse, error) bool) {
		for i := range m.sendResults {
			if !yield(m.sendResults[i], m.sendErrors[i]) {
				break
			}
		}
	}
}

func TestNewSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	mockWrapper.EXPECT().Engine().Return("gemini").AnyTimes()
	mockWrapper.EXPECT().Context().Return(context.Background()).AnyTimes()

	mockClient := &genai.Client{}
	originalNewClientFn := newClientFn
	defer func() { newClientFn = originalNewClientFn }()
	newClientFn = func(ctx context.Context, opts *genai.ClientConfig) *genai.Client {
		return mockClient
	}

	driver := &dipper.Driver{
		Options: map[string]interface{}{
			"data": map[string]interface{}{
				"engine": map[string]interface{}{
					"gemini": map[string]interface{}{
						"model": "gemini-pro",
					},
				},
				"tools_list": []*genai.Tool{},
			},
		},
	}

	msg := &dipper.Message{}

	session := newSession(driver, msg, mockWrapper)

	assert.NotNil(t, session)
	assert.Equal(t, "gemini-pro", session.model)
}

func TestGenerateConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	mockWrapper.EXPECT().Engine().Return("gemini").AnyTimes()

	tests := []struct {
		name       string
		msg        *dipper.Message
		driverOpts map[string]interface{}
		wantTemp   *float32
		wantTools  []*genai.Tool
	}{
		{
			name: "with message temperature",
			msg: &dipper.Message{
				Payload: map[string]interface{}{
					"temperature": float64(0.7),
				},
			},
			driverOpts: map[string]interface{}{
				"data": map[string]interface{}{
					"tools_list": []*genai.Tool{},
				},
			},
			wantTemp:  float32Ptr(0.7),
			wantTools: []*genai.Tool{},
		},
		{
			name: "with driver temperature",
			msg:  &dipper.Message{},
			driverOpts: map[string]interface{}{
				"data": map[string]interface{}{
					"engine": map[string]interface{}{
						"gemini": map[string]interface{}{
							"temperature": float64(0.8),
						},
					},
					"tools_list": []*genai.Tool{},
				},
			},
			wantTemp:  float32Ptr(0.8),
			wantTools: []*genai.Tool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := &dipper.Driver{Options: tt.driverOpts}
			session := &gemSession{
				driver:  driver,
				wrapper: mockWrapper,
			}

			cfg := session.generateConfig(tt.msg)

			assert.Equal(t, tt.wantTemp, cfg.Temperature)
			assert.Equal(t, tt.wantTools, cfg.Tools)
		})
	}
}

func TestStream(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	mockWrapper.EXPECT().Context().Return(context.Background()).AnyTimes()
	mockWrapper.EXPECT().Engine().Return("gemini").AnyTimes()

	mockChat := &MockChat{
		sendResults: []*genai.GenerateContentResponse{
			{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Parts: []*genai.Part{
								{Text: "Hello"},
							},
						},
						FinishReason: "STOP",
					},
				},
			},
		},
		sendErrors: []error{nil},
	}

	mockClient := &genai.Client{}
	originalNewClientFn := newClientFn
	defer func() { newClientFn = originalNewClientFn }()
	newClientFn = func(ctx context.Context, opts *genai.ClientConfig) *genai.Client {
		return mockClient
	}

	// Override createChatFn for testing
	originalCreateChatFn := createChatFn
	defer func() { createChatFn = originalCreateChatFn }()
	createChatFn = func(client *genai.Client, ctx context.Context, model string, cfg *genai.GenerateContentConfig, history []*genai.Content) ChatInterface {
		return mockChat
	}

	session := &gemSession{
		wrapper: mockWrapper,
		model:   "gemini-pro",
	}

	messages := []*genai.Content{
		{
			Role:  "user",
			Parts: []*genai.Part{{Text: "Hi"}},
		},
	}
	historyJSON, _ := json.Marshal(messages)

	var receivedText string
	var receivedDone bool

	session.Stream(
		&genai.Content{
			Role:  "user",
			Parts: []*genai.Part{{Text: "Hello"}},
		},
		historyJSON,
		func(text string, done bool) {
			receivedText = text
			receivedDone = done
		},
		nil,
	)

	assert.Equal(t, "Hello", receivedText)
	assert.True(t, receivedDone)
}

func TestBuildMessages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWrapper := mock_ai.NewMockChatWrapperInterface(ctrl)
	mockWrapper.EXPECT().Engine().Return("gemini").AnyTimes()

	driver := &dipper.Driver{
		Options: map[string]interface{}{
			"data": map[string]interface{}{
				"engine": map[string]interface{}{
					"gemini": map[string]interface{}{
						"system_prompt": "You are a helpful assistant",
					},
				},
			},
		},
	}

	session := &gemSession{
		driver:  driver,
		wrapper: mockWrapper,
	}

	messages := session.InitMessages("gemini")
	assert.Len(t, messages, 2)

	userMsg := session.BuildUserMessage("user1", "hello")
	content := userMsg.(*genai.Content)
	assert.Equal(t, "user", content.Role)
	assert.Equal(t, "user1 says :quote_start: hello:quote_end:", content.Parts[0].Text)

	assistantMsg := session.BuildMessage("response")
	content = assistantMsg.(*genai.Content)
	assert.Equal(t, "assistant", content.Role)
	assert.Equal(t, "response", content.Parts[0].Text)
}

func TestBuildToolReturnMessage(t *testing.T) {
	session := &gemSession{}

	content := []byte(`{"result": "success"}`)
	msg := session.BuildToolReturnMessage("test_function", "call_1", content)

	toolMsg := msg.(*genai.Content)
	assert.Equal(t, "tool", toolMsg.Role)
	assert.Equal(t, "test_function", toolMsg.Parts[0].FunctionResponse.Name)
	assert.Equal(t, "call_1", toolMsg.Parts[0].FunctionResponse.ID)
	assert.Equal(t, map[string]interface{}{"result": "success"}, toolMsg.Parts[0].FunctionResponse.Response)
}

// Helper function to create float32 pointer.
func float32Ptr(v float32) *float32 {
	return &v
}
