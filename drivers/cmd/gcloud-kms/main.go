// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package gcloud-kms enables Honeydipper to use secrets encrypted using gcloud KMS.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// ErrKeyNameMissing means the key used for decrypting is not configured.
var ErrKeyNameMissing = errors.New("key name not configured")

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of decrypting with gcloud kms")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "kms")
	driver.RPCHandlers["decrypt"] = decrypt
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func decrypt(msg *dipper.Message) {
	name, ok := driver.GetOptionStr("data.keyname")
	if !ok {
		panic(ErrKeyNameMissing)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*driver.APITimeout)
	defer cancel()
	req := &kmspb.DecryptRequest{
		Name:       name,
		Ciphertext: msg.Payload.([]byte),
	}
	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		dipper.Logger.Warning("failed to create kms client")
		panic(err)
	}
	resp, err := client.Decrypt(ctx, req)
	if err != nil {
		dipper.Logger.Warning("failed to decrypt")
		panic(err)
	}

	msg.Reply <- dipper.Message{
		Payload: resp.Plaintext,
		IsRaw:   true,
	}
}
