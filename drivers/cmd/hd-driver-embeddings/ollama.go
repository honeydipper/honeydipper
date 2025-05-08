package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/ollama/ollama/api"
)

// OllamaClientInterface defines the interface for interacting with the Ollama API.
type OllamaClientInterface interface {
	// Embeddings generates vector embeddings for the given text prompt using the specified model.
	Embeddings(ctx context.Context, req *api.EmbeddingRequest) (*api.EmbeddingResponse, error)
}

// NewOllamaClient creates a new Ollama client using the provided host or environment settings.
var NewOllamaClient = func(ollamaHost string) (OllamaClientInterface, error) {
	// If a host is provided, create a client with that specific host.
	if len(ollamaHost) > 0 {
		u, err := url.ParseRequestURI(ollamaHost)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ollama host: %w", err)
		}

		return api.NewClient(u, http.DefaultClient), nil
	}

	// Otherwise, create a client using environment settings.
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama client: %w", err)
	}

	return client, nil
}

// ollama handles the embedding generation request for multiple questions.
func ollama(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)

	// Setting up client using the provided host or environment settings.
	ollamaHost, _ := dipper.GetMapDataStr(driver.Options, "data.ollama.host")
	client := dipper.Must(NewOllamaClient(ollamaHost)).(OllamaClientInterface)

	// Setting up model from payload or falling back to driver options.
	model, _ := dipper.GetMapDataStr(msg.Payload, "model")
	if model == "" {
		model = dipper.MustGetMapDataStr(driver.Options, "data.ollama.model")
	}

	// Extract questions from payload and prepare return slice.
	q := dipper.MustGetMapData(msg.Payload, "questions")
	ret := make([][]float64, len(q.([]interface{})))
	ctx := context.Background()

	// Generate embeddings for each question in the payload.
	for i, question := range q.([]interface{}) {
		ret[i] = dipper.Must(client.Embeddings(ctx, &api.EmbeddingRequest{
			Model:  model,
			Prompt: question.(string),
		})).(*api.EmbeddingResponse).Embedding
	}

	// Send the generated embeddings back in the response.
	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"embeddings": ret,
		},
	}
}
