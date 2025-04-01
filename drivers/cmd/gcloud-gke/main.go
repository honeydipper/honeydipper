// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
)

var (
	// ErrMissingProject means missing project.
	ErrMissingProject = errors.New("project required")
	// ErrMissingLocation means missing location.
	ErrMissingLocation = errors.New("location required")
	// ErrMissingCluster means missing location.
	ErrMissingCluster = errors.New("cluster required")
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
	driver.RPCHandlers["getKubeCfg"] = getKubeCfg
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func getGKEService(serviceAccountBytes string) (*container.Service, *oauth2.Token) {
	var (
		containerService *container.Service
		token            *oauth2.Token
	)
	if len(serviceAccountBytes) > 0 {
		containerService = dipper.Must(
			container.NewService(
				context.Background(),
				option.WithCredentialsJSON([]byte(serviceAccountBytes)),
			),
		).(*container.Service)
		conf := dipper.Must(google.JWTConfigFromJSON([]byte(serviceAccountBytes), "https://www.googleapis.com/auth/cloud-platform")).(*jwt.Config)
		token = dipper.Must(conf.TokenSource(context.Background()).Token()).(*oauth2.Token)
	} else {
		containerService = dipper.Must(container.NewService(context.Background())).(*container.Service)
		tokenSource := dipper.Must(
			google.DefaultTokenSource(
				context.Background(),
				"https://www.googleapis.com/auth/cloud-platform",
			),
		).(oauth2.TokenSource)
		token = dipper.Must(tokenSource.Token()).(*oauth2.Token)
	}

	return containerService, token
}

func getKubeCfg(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, _ := dipper.GetMapDataStr(params, "service_account")
	project, ok := dipper.GetMapDataStr(params, "project")
	if !ok {
		panic(ErrMissingProject)
	}
	location, ok := dipper.GetMapDataStr(params, "location")
	if !ok {
		panic(ErrMissingLocation)
	}
	regional := false
	if regionalData, ok := dipper.GetMapData(params, "regional"); ok {
		regional = regionalData.(bool)
	}
	cluster, ok := dipper.GetMapDataStr(params, "cluster")
	if !ok {
		panic(ErrMissingCluster)
	}
	var name string
	if regional {
		name = fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)
	} else {
		name = fmt.Sprintf("projects/%s/zones/%s/clusters/%s", project, location, cluster)
	}

	containerService, token := getGKEService(serviceAccountBytes)

	execContext, cancel := context.WithTimeout(context.Background(), time.Second*driver.APITimeout)
	var (
		clusterObj *container.Cluster
		err        error
	)
	func() {
		defer cancel()
		clusterObj, err = containerService.Projects.Locations.Clusters.Get(name).Context(execContext).Do()
	}()
	if err != nil {
		panic(err)
	}

	useDNS := false
	if cp := clusterObj.ControlPlaneEndpointsConfig; cp != nil {
		if dnsCfg := cp.DnsEndpointConfig; dnsCfg != nil {
			if clusterObj.Endpoint == dnsCfg.Endpoint && dnsCfg.AllowExternalTraffic {
				// GKE DNS based control plane access.
				useDNS = true
			}
		}
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"Host":   clusterObj.Endpoint,
			"Token":  token.AccessToken,
			"CACert": clusterObj.MasterAuth.ClusterCaCertificate,
			"useDNS": useDNS,
		},
	}
}
