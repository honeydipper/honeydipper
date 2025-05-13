// Package main implements a Honeydipper driver for Qdrant vector database integration.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/qdrant/go-client/qdrant"
)

// initFlags sets up command line flags and usage information.
func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports operator service.\n")
		fmt.Printf("  This program provides honeydipper AI with RAG capability using Qdrant.\n")
	}
}

// driver is the global Driver instance used by this service.
var driver *dipper.Driver

// main initializes and runs the Qdrant driver service.
func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "qdrant")
	driver.RPCHandlers["query"] = query
	driver.Run()
}

// Info represents the structure of information retrieved from Qdrant.
type Info struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// query handles vector similarity search requests to Qdrant.
func query(m *dipper.Message) {
	// Deserialize and extract query parameters.
	m = dipper.DeserializePayload(m)
	question := dipper.MustGetMapDataStr(m.Payload, "question")
	kb := dipper.MustGetMapDataStr(m.Payload, "knowledge_base")

	// Get configuration for the specified knowledge base.
	host := dipper.MustGetMapDataStr(driver.Options, "data.knowledge_base."+kb+".host")
	port := dipper.MustGetMapDataInt(driver.Options, "data.knowledge_base."+kb+".port")
	collection := dipper.MustGetMapDataStr(driver.Options, "data.knowledge_base."+kb+".collection")
	embeddingMethod := dipper.MustGetMapDataStr(driver.Options, "data.knowledge_base."+kb+".embedding_method")
	embeddingModel, _ := dipper.GetMapDataStr(driver.Options, "data.knowledge_base."+kb+".embedding_model")

	// Generate embeddings for the question.
	embedding := dipper.DeserializeContent(dipper.Must(driver.Call("driver:embeddings", embeddingMethod, map[string]interface{}{
		"model":     embeddingModel,
		"questions": []any{question},
	})).([]byte)).(map[string]any)["embeddings"].([]any)[0].([]any)

	// converting the return to float32 for qdrant.
	values := make([]float32, len(embedding))
	for i, v := range embedding {
		values[i] = float32(v.(float64))
	}

	// Initialize Qdrant client.
	qdrantClient := dipper.Must(qdrant.NewClient(&qdrant.Config{
		Host: host,
		Port: port,
	})).(*qdrant.Client)
	defer qdrantClient.Close()

	// Perform vector similarity search.
	ctx := context.Background()
	limit := uint64(5)
	related := dipper.Must(qdrantClient.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQuery(values...),
		WithPayload:    qdrant.NewWithPayload(true),
		Limit:          &limit,
	})).([]*qdrant.ScoredPoint)

	// Process and format search results.
	ret := make([]Info, len(related))
	for i, r := range related {
		ret[i] = Info{
			Text: r.Payload["text"].GetStringValue(),
			URL:  r.Payload["url"].GetStringValue(),
		}
	}

	// Send response with related information.
	m.Reply <- dipper.Message{
		Payload: map[string]any{
			"related_info": ret,
		},
	}
}
