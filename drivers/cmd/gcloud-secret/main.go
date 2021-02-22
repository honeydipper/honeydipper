// Copyright 2021 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package gcloud-secret enables Honeydipper to use secrets stored in gcloud secret manager.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/gogf/gf/container/gpool"
	"github.com/googleapis/gax-go/v2"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

// DefaultClientTTLSeconds specifies the TTL for reusable google clients.
const DefaultClientTTLSeconds = 60

// ErrSecretNameMissing means the secret name is not supplied.
var ErrSecretNameMissing = errors.New("secret name not supplied")

// SecretManagerClient is an interface with a subset of method used for mocking.
type SecretManagerClient interface {
	AccessSecretVersion(
		ctx context.Context,
		req *secretmanagerpb.AccessSecretVersionRequest,
		opts ...gax.CallOption,
	) (*secretmanagerpb.AccessSecretVersionResponse, error)
	Close() error
}

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of decrypting with gcloud secret manager")
	}
}

var (
	driver      *dipper.Driver
	_clientPool *gpool.Pool
)

func loadOptions(msg *dipper.Message) {
	var clientTTL time.Duration
	if clientTTLStr, ok := driver.GetOptionStr("data.clientTTL"); ok {
		clientTTL = dipper.Must(time.ParseDuration(clientTTLStr)).(time.Duration)
	} else {
		clientTTL = time.Second * DefaultClientTTLSeconds
	}
	if _clientPool != nil {
		_clientPool.Close()
		dipper.Logger.Infof("[%s] _clientPool re-created", driver.Service)
	}
	_clientPool = gpool.New(
		clientTTL, // TTL
		func() (interface{}, error) { return secretmanager.NewClient(context.Background()) }, // NewFunc
		func(o interface{}) { _ = o.(SecretManagerClient).Close() },                          // ExpireFunc
	)
}

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "google-secret")
	driver.RPCHandlers["decrypt"] = decrypt
	driver.Reload = loadOptions
	driver.Start = loadOptions
	driver.Run()
}

func decrypt(msg *dipper.Message) {
	nameBytes, ok := msg.Payload.([]byte)
	if !ok {
		panic(ErrSecretNameMissing)
	}
	name := string(nameBytes)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*driver.APITimeout)
	defer cancel()
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}
	client, err := _clientPool.Get()
	if err != nil {
		dipper.Logger.Warning("failed to google secret manager client")
		panic(err)
	}
	defer func() { _ = _clientPool.Put(client) }()
	resp, err := client.(SecretManagerClient).AccessSecretVersion(ctx, req)
	if err != nil {
		dipper.Logger.Warning("failed to access the secret version")
		panic(err)
	}

	msg.Reply <- dipper.Message{
		Payload: resp.Payload.Data,
		IsRaw:   true,
	}
}
