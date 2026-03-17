// Copyright 2025 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.
//

package main

import (
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	mock_main "github.com/honeydipper/honeydipper/v3/drivers/cmd/hd-driver-embeddings/mock_hd-driver-embeddings"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		f, _ := os.Create("test.log")
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}

// TestVertexAI tests the vertexAI function with mocked GenAIClientModels.
func TestVertexAI(t *testing.T) {
	// Setup mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock client
	mockClient := mock_main.NewMockGenAIClientModels(ctrl)

	// Setup test data
	questions := []interface{}{"What is the capital of France?", "How tall is Mount Everest?"}
	expectedEmbeddings := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}

	// Setup mock response
	mockResponse := &genai.EmbedContentResponse{
		Embeddings: []*genai.ContentEmbedding{
			{Values: expectedEmbeddings[0]},
			{Values: expectedEmbeddings[1]},
		},
	}

	// Setup expected mock call
	mockClient.EXPECT().
		EmbedContent(
			gomock.Any(),
			"text-embedding-005",
			gomock.Any(),
			gomock.Any(),
		).
		DoAndReturn(func(ctx context.Context, model string, contents []*genai.Content, config *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
			// Verify the content parts match our questions
			assert.Equal(t, len(questions), len(contents[0].Parts))
			for i, part := range contents[0].Parts {
				assert.Equal(t, questions[i].(string), part.Text)
			}

			// Verify config parameters
			assert.Equal(t, "SEMANTIC_SIMILARITY", config.TaskType)
			assert.NotNil(t, config.OutputDimensionality)
			assert.Equal(t, int32(768), *config.OutputDimensionality)

			return mockResponse, nil
		})

	// Setup driver and options
	driver = &dipper.Driver{
		Options: map[string]interface{}{
			"data": map[string]interface{}{
				"vertex-ai": map[string]interface{}{
					"project":  "test-project",
					"location": "us-central1",
				},
			},
		},
	}

	// Create a function to replace NewGenAIClientModels
	origNewGenAIClientModels := NewGenAIClientModels
	defer func() { NewGenAIClientModels = origNewGenAIClientModels }()
	NewGenAIClientModels = func(ctx context.Context, cfg *genai.ClientConfig) GenAIClientModels {
		// Verify config parameters
		assert.Equal(t, "test-project", cfg.Project)
		assert.Equal(t, "us-central1", cfg.Location)
		assert.Equal(t, genai.BackendVertexAI, cfg.Backend)

		return mockClient
	}

	// Create message with test payload
	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"questions": questions,
		},
		Reply: make(chan dipper.Message, 1),
	}

	// Call the function under test
	vertexAI(msg)

	// Get the response
	response := <-msg.Reply

	// Verify the response
	embeddings, ok := response.Payload.(map[string]any)["embeddings"].([][]float32)
	assert.True(t, ok, "Expected embeddings to be [][]float32")
	assert.Equal(t, len(expectedEmbeddings), len(embeddings))

	for i, embedding := range embeddings {
		assert.Equal(t, expectedEmbeddings[i], embedding)
	}
}

// TestVertexAIWithServiceAccount tests the vertexAI function with a service account.
func TestVertexAIWithServiceAccount(t *testing.T) {
	// Setup mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock client
	mockClient := mock_main.NewMockGenAIClientModels(ctrl)

	// Setup mock response with minimal data for this test
	mockResponse := &genai.EmbedContentResponse{
		Embeddings: []*genai.ContentEmbedding{
			{Values: []float32{0.1, 0.2, 0.3}},
		},
	}

	// Setup expected mock call - we're only testing the service account here
	mockClient.EXPECT().
		EmbedContent(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).
		Return(mockResponse, nil)

	// Setup driver and options with service account
	driver = &dipper.Driver{
		Options: map[string]interface{}{
			"data": map[string]interface{}{
				"vertex-ai": map[string]interface{}{
					"project":         "test-project",
					"location":        "us-central1",
					"service_account": "{\"type\":\"service_account\",\"project_id\":\"test\"}",
				},
			},
		},
	}

	// Create a function to replace NewGenAIClientModels
	origNewGenAIClientModels := NewGenAIClientModels
	defer func() { NewGenAIClientModels = origNewGenAIClientModels }()
	NewGenAIClientModels = func(ctx context.Context, cfg *genai.ClientConfig) GenAIClientModels {
		// Verify service account was set
		assert.NotNil(t, cfg.Credentials, "Expected credentials to be set")

		return mockClient
	}

	// Create message with minimal test payload
	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"questions": []interface{}{"test"},
		},
		Reply: make(chan dipper.Message, 1),
	}

	// Call the function under test
	vertexAI(msg)

	// Get the response (we don't need to verify it for this test)
	<-msg.Reply
}
