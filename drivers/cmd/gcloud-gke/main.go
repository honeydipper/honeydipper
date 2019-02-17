// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/honeyscience/honeydipper/pkg/dipper"
	"golang.org/x/oauth2"
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

func getGKEService(serviceAccountBytes string) (*container.Service, *oauth2.Token) {
	var (
		client *http.Client
		token  *oauth2.Token
		err    error
	)
	if len(serviceAccountBytes) > 0 {
		conf, err := google.JWTConfigFromJSON([]byte(serviceAccountBytes), "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			panic(errors.New("invalid service account"))
		}
		client = conf.Client(context.Background())
		token, err = conf.TokenSource(context.Background()).Token()
		if err != nil {
			panic(errors.New("failed to fetch a access_token"))
		}
	} else {
		client, err = google.DefaultClient(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			panic(errors.New("unable to create gcloud client credential"))
		}
		tokenSource, err := google.DefaultTokenSource(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			panic(errors.New("failed to get a token source"))
		}
		token, err = tokenSource.Token()
		if err != nil {
			panic(errors.New("failed to fetch a access_token"))
		}
	}

	containerService, err := container.New(client)
	if err != nil {
		panic(errors.New("unable to create container service client"))
	}
	return containerService, token
}

func getKubeCfg(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, _ := dipper.GetMapDataStr(params, "service_account")
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
	var name string
	if regional {
		name = fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)
	} else {
		name = fmt.Sprintf("projects/%s/zones/%s/clusters/%s", project, location, cluster)
	}

	containerService, token := getGKEService(serviceAccountBytes)

	execContext, cancel := context.WithTimeout(context.Background(), time.Second*10)
	var (
		clusterObj *container.Cluster
		err        error
	)
	func() {
		defer cancel()
		clusterObj, err = containerService.Projects.Locations.Clusters.Get(name).Context(execContext).Do()
	}()
	if err != nil {
		panic(errors.New("failed to fetch cluster info from gcloud"))
	}
	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"Host":   clusterObj.Endpoint,
			"Token":  token.AccessToken,
			"CACert": clusterObj.MasterAuth.ClusterCaCertificate,
		},
	}
}
