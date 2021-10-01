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
	"strings"
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

// ErrSecretNameInvalid means the secret name is not valid.
var ErrSecretNameInvalid = errors.New("secret name not valid")

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
		func() (interface{}, error) {
			i, e := secretmanager.NewClient(context.Background())
			if e != nil {
				return i, fmt.Errorf("new secret manager client error: %w", e)
			}

			return i, nil
		}, // NewFunc
		func(o interface{}) { _ = o.(SecretManagerClient).Close() }, // ExpireFunc
	)
}

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "google-secret")
	driver.RPCHandlers["lookup"] = lookup
	driver.Reload = loadOptions
	driver.Start = loadOptions
	driver.Run()
}

func lookup(msg *dipper.Message) {
	nameBytes, ok := msg.Payload.([]byte)
	if !ok {
		panic(ErrSecretNameMissing)
	}
	name := string(nameBytes)

	parts := strings.Split(name, "/")
	switch {
	case len(parts) == 6: //nolint:gomnd
		if parts[0] != "projects" || parts[2] != "secrets" || parts[4] != "versions" {
			dipper.Logger.Warningf("incorrect secret key format %s", name)
			panic(ErrSecretNameInvalid)
		}
	case len(parts) == 2 || len(parts) == 3:
		version := "latest"
		if len(parts) == 3 { //nolint:gomnd
			version = parts[2]
		}
		name = fmt.Sprintf("projects/%s/secrets/%s/versions/%s", parts[0], parts[1], version)
	default:
		dipper.Logger.Warningf("incorrect secret key format %s", name)
		panic(ErrSecretNameInvalid)
	}

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
