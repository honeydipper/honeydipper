// Copyright 2022 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.
//

package main

import (
	"testing"

	"github.com/golang/mock/gomock"
	mock_main "github.com/honeydipper/honeydipper/v3/drivers/cmd/hd-driver-embeddings/mock_hd-driver-embeddings"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
)

func TestOllama(t *testing.T) {
	// Setup mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock client
	mockClient := mock_main.NewMockOllamaClientInterface(ctrl)

	// Setup test data
	questions := []interface{}{"What is the capital of France?", "How tall is Mount Everest?"}
	expectedEmbeddings := [][]float64{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}

	// Setup driver with default model
	driver = &dipper.Driver{
		Options: map[string]interface{}{
			"data": map[string]interface{}{
				"ollama": map[string]interface{}{
					"model": "llama2",
				},
			},
		},
	}

	// Setup mock responses for each question
	for i, question := range questions {
		mockClient.EXPECT().
			Embeddings(
				gomock.Any(),
				&api.EmbeddingRequest{
					Model:  "llama2",
					Prompt: question.(string),
				},
			).
			Return(&api.EmbeddingResponse{
				Embedding: expectedEmbeddings[i],
			}, nil)
	}

	// Create a function to replace NewOllamaClient
	origNewOllamaClient := NewOllamaClient
	defer func() { NewOllamaClient = origNewOllamaClient }()
	NewOllamaClient = func(ollamaHost string) (OllamaClientInterface, error) {
		// Verify no host is provided in this test
		assert.Equal(t, "", ollamaHost)

		return mockClient, nil
	}

	// Create message with test payload
	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"questions": questions,
		},
		Reply: make(chan dipper.Message, 1),
	}

	// Call the function under test
	ollama(msg)

	// Get the response
	response := <-msg.Reply

	// Verify the response
	embeddings, ok := response.Payload.(map[string]any)["embeddings"].([][]float64)
	assert.True(t, ok, "Expected embeddings to be [][]float64")
	assert.Equal(t, len(expectedEmbeddings), len(embeddings))

	for i, embedding := range embeddings {
		assert.Equal(t, expectedEmbeddings[i], embedding)
	}
}

func TestOllamaWithCustomHostAndModel(t *testing.T) {
	// Setup mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock client
	mockClient := mock_main.NewMockOllamaClientInterface(ctrl)

	// Setup test data
	customHost := "http://custom-ollama:11434"
	customModel := "mistral"
	questions := []interface{}{"Test question"}
	expectedEmbeddings := [][]float64{
		{0.7, 0.8, 0.9},
	}

	// Setup driver with default model (should be overridden by payload)
	driver = &dipper.Driver{
		Options: map[string]interface{}{
			"data": map[string]interface{}{
				"ollama": map[string]interface{}{
					"model": "llama2", // This should be ignored in favor of the custom model
					"host":  customHost,
				},
			},
		},
	}

	// Setup mock response
	mockClient.EXPECT().
		Embeddings(
			gomock.Any(),
			&api.EmbeddingRequest{
				Model:  customModel,
				Prompt: questions[0].(string),
			},
		).
		Return(&api.EmbeddingResponse{
			Embedding: expectedEmbeddings[0],
		}, nil)

	// Create a function to replace NewOllamaClient
	origNewOllamaClient := NewOllamaClient
	defer func() { NewOllamaClient = origNewOllamaClient }()
	NewOllamaClient = func(ollamaHost string) (OllamaClientInterface, error) {
		// Verify custom host is provided
		assert.Equal(t, customHost, ollamaHost)

		return mockClient, nil
	}

	// Create message with test payload including custom host and model
	msg := &dipper.Message{
		Payload: map[string]interface{}{
			"questions": questions,
			"model":     customModel,
		},
		Reply: make(chan dipper.Message, 1),
	}

	// Call the function under test
	ollama(msg)

	// Get the response
	response := <-msg.Reply

	// Verify the response
	embeddings, ok := response.Payload.(map[string]any)["embeddings"].([][]float64)
	assert.True(t, ok, "Expected embeddings to be [][]float64")
	assert.Equal(t, len(expectedEmbeddings), len(embeddings))
	assert.Equal(t, expectedEmbeddings[0], embeddings[0])
}
