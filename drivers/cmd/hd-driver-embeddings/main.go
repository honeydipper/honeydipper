package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"cloud.google.com/go/auth"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"google.golang.org/genai"
)

// GenAIClientModels interface defines methods used from genai.Client.
type GenAIClientModels interface {
	EmbedContent(
		ctx context.Context,
		model string,
		contents []*genai.Content,
		config *genai.EmbedContentConfig,
	) (*genai.EmbedContentResponse, error)
}

// NewGenAIClientModels creates a new GenAI client.
var NewGenAIClientModels = func(ctx context.Context, cfg *genai.ClientConfig) GenAIClientModels {
	return dipper.Must(genai.NewClient(ctx, cfg)).(*genai.Client).Models
}

// initFlags initializes command line flags and usage information.
func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports operator service.\n")
		fmt.Printf("  This program provides honeydipper AI with a variety of way of embeddings.\n")
	}
}

// driver is the global driver instance for handling RPC calls.
var driver *dipper.Driver

// main initializes and runs the embeddings driver.
func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "embeddings")
	driver.RPCHandlers["vertex-ai"] = vertexAI
	driver.RPCHandlers["ollama"] = ollama
	driver.Run()
}

// vertexAI handles embedding generation requests using Google's Vertex AI.
func vertexAI(m *dipper.Message) {
	// Deserialize the incoming message payload.
	m = dipper.DeserializePayload(m)
	q := dipper.MustGetMapData(m.Payload, "questions")

	dipper.Logger.Debugf("[embeddings] %v", q)
	// Convert questions into genai Parts.
	parts := make([]*genai.Part, len(q.([]any)))
	for i, question := range q.([]any) {
		parts[i] = &genai.Part{
			Text: question.(string),
		}
	}

	// Get configuration from driver options.
	project := dipper.MustGetMapDataStr(driver.Options, "data.vertex-ai.project")
	location := dipper.MustGetMapDataStr(driver.Options, "data.vertex-ai.location")
	serviceAccount, _ := dipper.GetMapDataStr(driver.Options, "data.vertex-ai.service_account")

	ctx := context.Background()

	// Configure the embedding model client.
	embCfg := &genai.ClientConfig{
		Project:  project,
		Location: location,
		Backend:  genai.BackendVertexAI,
	}
	if serviceAccount != "" {
		embCfg.Credentials = auth.NewCredentials(&auth.CredentialsOptions{
			JSON: []byte(serviceAccount),
		})
	}
	embClient := NewGenAIClientModels(ctx, embCfg)

	// Generate embeddings using the configured model.
	var dim int32 = 768
	resp := dipper.Must(embClient.EmbedContent(
		ctx,
		"text-embedding-005",
		[]*genai.Content{{Parts: parts}},
		&genai.EmbedContentConfig{
			TaskType:             "SEMANTIC_SIMILARITY",
			OutputDimensionality: &dim,
		},
	)).(*genai.EmbedContentResponse)

	// Extract embedding values from response.
	ret := make([][]float32, len(resp.Embeddings))
	for i, embedding := range resp.Embeddings {
		ret[i] = embedding.Values
	}

	// Send back the generated embeddings.
	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"embeddings": ret,
		},
	}
}
