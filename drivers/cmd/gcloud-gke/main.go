package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/honeyscience/honeydipper/pkg/dipper"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/container/v1"
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of interacting with gcloud")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "gcloud-gke")
	driver.RPC.Provider.RPCHandlers["getKubeCfg"] = getKubeCfg
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func getKubeCfg(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, ok := dipper.GetMapDataStr(params, "service_account")
	if !ok {
		panic(errors.New("service_account required"))
	}
	project, ok := dipper.GetMapDataStr(params, "project")
	if !ok {
		panic(errors.New("project required"))
	}
	location, ok := dipper.GetMapDataStr(params, "location")
	if !ok {
		panic(errors.New("location required"))
	}
	regional := false
	if regionalData, ok := dipper.GetMapData(params, "regional"); ok {
		regional = regionalData.(bool)
	}
	cluster, ok := dipper.GetMapDataStr(params, "cluster")
	if !ok {
		panic(errors.New("cluster required"))
	}
	conf, err := google.JWTConfigFromJSON([]byte(serviceAccountBytes), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		panic(errors.New("invalid service account"))
	}
	containerService, err := container.New(conf.Client(context.Background()))
	if err != nil {
		panic(errors.New("unable to create gcloud client"))
	}
	var name string
	if regional {
		name = fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)
	} else {
		name = fmt.Sprintf("projects/%s/zones/%s/clusters/%s", project, location, cluster)
	}
	execContext, cancel := context.WithTimeout(context.Background(), time.Second*10)
	var clusterObj *container.Cluster
	func() {
		defer cancel()
		clusterObj, err = containerService.Projects.Locations.Clusters.Get(name).Context(execContext).Do()
	}()
	if err != nil {
		panic(errors.New("failed to fetch cluster info from gcloud"))
	}
	tokenSource := conf.TokenSource(context.Background())
	token, err := tokenSource.Token()
	if err != nil {
		panic(errors.New("failed to fetch a access_token"))
	}
	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"Host":   clusterObj.Endpoint,
			"Token":  token.AccessToken,
			"CACert": clusterObj.MasterAuth.ClusterCaCertificate,
		},
	}
}
