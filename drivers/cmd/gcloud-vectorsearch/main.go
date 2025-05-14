package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	aiplatform "cloud.google.com/go/aiplatform/apiv1beta1"
	"cloud.google.com/go/aiplatform/apiv1beta1/aiplatformpb"
	"github.com/honeydipper/honeydipper/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"google.golang.org/api/option"
)

var redisOptions *redisclient.Options

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports operator service.\n")
		fmt.Printf("  This program provides honeydipper AI with RAG capability using Google vector search.\n")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "gcloud-vectorsearch")
	driver.RPCHandlers["query"] = query
	driver.Start = func(_ *dipper.Message) {
		redisOptions = redisclient.GetRedisOpts(driver)
	}
	driver.Run()
}

type Info struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

func query(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	question := dipper.MustGetMapDataStr(m.Payload, "question")
	kb := dipper.MustGetMapDataStr(m.Payload, "knowledge_base")

	location := dipper.MustGetMapDataStr(driver.Options, "data.knowledge_base."+kb+".location")
	serviceAccount, _ := dipper.GetMapDataStr(driver.Options, "data.knowledge_base."+kb+".service_account")
	idxeName := dipper.MustGetMapDataStr(driver.Options, "data.knowledge_base."+kb+".index_endpoint")
	didxID := dipper.MustGetMapDataStr(driver.Options, "data.knowledge_base."+kb+".deployed_index_id")
	redisPrefix := dipper.MustGetMapDataStr(driver.Options, "data.knowledge_base."+kb+".redis_prefix")
	embeddingMethod := dipper.MustGetMapDataStr(driver.Options, "data.knowledge_base."+kb+".embedding_method")
	embeddingModel, _ := dipper.GetMapDataStr(driver.Options, "data.knowledge_base."+kb+".embedding_model")

	ctx := context.Background()
	var clientOptions []option.ClientOption
	if len(serviceAccount) > 0 {
		clientOptions = append(clientOptions, option.WithCredentialsJSON([]byte(serviceAccount)))
	}

	// Generate embeddings for the question.
	dipper.Logger.Warningf("embedding question: %s", embeddingMethod)
	embedding := dipper.DeserializeContent(dipper.Must(driver.Call("driver:embeddings", embeddingMethod, map[string]interface{}{
		"model":     embeddingModel,
		"questions": []any{question},
	})).([]byte)).(map[string]any)["embeddings"].([]any)[0].([]any)

	// converting the return to float32 for qdrant.
	values := make([]float32, len(embedding))
	for i, v := range embedding {
		values[i] = float32(v.(float64))
	}

	// find the index endpoint
	regionalEndpoint := option.WithEndpoint(location + "-aiplatform.googleapis.com:443")
	ideClient := dipper.Must(aiplatform.NewIndexEndpointClient(
		ctx,
		append(clientOptions, regionalEndpoint)...,
	)).(*aiplatform.IndexEndpointClient)
	defer ideClient.Close()
	ide := dipper.Must(ideClient.GetIndexEndpoint(ctx, &aiplatformpb.GetIndexEndpointRequest{
		Name: idxeName,
	})).(*aiplatformpb.IndexEndpoint)

	// establish match service client
	endPointOption := option.WithEndpoint(ide.GetPublicEndpointDomainName() + ":443")
	matchClient := dipper.Must(aiplatform.NewMatchClient(
		ctx,
		append(clientOptions, endPointOption)...,
	)).(*aiplatform.MatchClient)
	defer matchClient.Close()

	// query the match service
	neighbors := dipper.Must(matchClient.FindNeighbors(ctx, &aiplatformpb.FindNeighborsRequest{
		IndexEndpoint:   idxeName,
		DeployedIndexId: didxID,
		Queries: []*aiplatformpb.FindNeighborsRequest_Query{
			{
				Datapoint: &aiplatformpb.IndexDatapoint{
					FeatureVector: values,
				},
				NeighborCount: 5,
			},
		},
	})).(*aiplatformpb.FindNeighborsResponse)

	// prepare the redis client
	rclient := redisclient.NewClient(redisOptions)
	defer rclient.Close()

	// convert the datapoint to actual text chunks
	info := make([]Info, len(neighbors.NearestNeighbors[0].Neighbors))
	for i, n := range neighbors.NearestNeighbors[0].Neighbors {
		txtKey := filepath.Join(redisPrefix, n.Datapoint.DatapointId, "text")
		urlKey := filepath.Join(redisPrefix, n.Datapoint.DatapointId, "url")
		dipper.Logger.Warningf("retrieving %s, %f", txtKey, n.Distance)
		text := dipper.Must(rclient.Get(context.Background(), txtKey).Result()).(string)
		url := dipper.Must(rclient.Get(context.Background(), urlKey).Result()).(string)
		info[i] = Info{text, url}
	}

	// return to sender
	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"related_info": info,
		},
	}
}
