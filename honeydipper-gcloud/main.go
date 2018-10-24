package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/container/v1"
	"os"
	"time"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of interacting with gcloud")
	}
}

var driver *dipper.Driver

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "gcloud")
	driver.RPCHandlers["getKubeCfg"] = getKubeCfg
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func getKubeCfg(from string, rpcID string, payload []byte) {
	params := dipper.DeserializeContent(payload)
	serviceAccountBytes, ok := dipper.GetMapDataStr(params, "service_account")
	if !ok {
		driver.RPCError(from, rpcID, "service_account required")
	}
	project, ok := dipper.GetMapDataStr(params, "project")
	if !ok {
		driver.RPCError(from, rpcID, "project required")
	}
	location, ok := dipper.GetMapDataStr(params, "location")
	if !ok {
		driver.RPCError(from, rpcID, "location required")
	}
	regional := false
	if regionalData, ok := dipper.GetMapData(params, "regional"); ok {
		regional = regionalData.(bool)
	}
	cluster, ok := dipper.GetMapDataStr(params, "cluster")
	if !ok {
		driver.RPCError(from, rpcID, "cluster required")
	}
	conf, err := google.JWTConfigFromJSON([]byte(serviceAccountBytes), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		driver.RPCError(from, rpcID, "invalid service account")
	}
	containerService, err := container.New(conf.Client(oauth2.NoContext))
	if err != nil {
		driver.RPCError(from, rpcID, "unable to create gcloud client")
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
		driver.RPCError(from, rpcID, "failed to fetch cluster info from gcloud")
	}
	tokenSource := conf.TokenSource(oauth2.NoContext)
	token, err := tokenSource.Token()
	if err != nil {
		driver.RPCError(from, rpcID, "failed to fetch a access_token")
	}
	driver.RPCReturn(from, rpcID, map[string]interface{}{
		"Host":   clusterObj.Endpoint,
		"Token":  token.AccessToken,
		"CACert": clusterObj.MasterAuth.ClusterCaCertificate,
	})
}
