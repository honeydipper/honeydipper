package ollamahelper

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/ollama/ollama/api"
)

// NewOllamaClient creates a new Ollama client using the provided host or environment settings.
var NewOllamaClient = func(ollamaHost string) (*api.Client, error) {
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
