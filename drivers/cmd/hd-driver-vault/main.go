// Copyright 2024 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package hd-driver-vault enables Honeydipper to use secrets stored in Hashicorp Vault.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	vault "github.com/hashicorp/vault/api"
	auth "github.com/hashicorp/vault/api/auth/kubernetes"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// ErrSecretKeyNotFound means the secret is found but the key is not found.
var ErrSecretKeyNotFound = errors.New("secret key not found in the secret")

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of decrypting with vault")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "vault")
	driver.RPCHandlers["lookup"] = lookup
	driver.Run()
}

func lookup(msg *dipper.Message) {
	query := string(msg.Payload.([]byte))
	parts := strings.SplitN(query, ":", 2)

	var addr, token, k8sRole string
	if len(parts) > 1 {
		query = parts[1]
		server := parts[0]
		addr = dipper.MustGetMapDataStr(driver.Options, "data."+server+".addr")
		token, _ = dipper.GetMapDataStr(driver.Options, "data."+server+".token")
		k8sRole, _ = dipper.GetMapDataStr(driver.Options, "data."+server+".k8sRole")
	} else {
		addr = dipper.MustGetMapDataStr(driver.Options, "data.addr")
		token, _ = dipper.GetMapDataStr(driver.Options, "data.token")
		k8sRole, _ = dipper.GetMapDataStr(driver.Options, "data.k8sRole")
	}

	version := -1
	parts = strings.SplitN(query, "@", 2)
	if len(parts) > 1 {
		version = dipper.Must(strconv.Atoi(parts[1])).(int)
	}

	parts = strings.SplitN(parts[0], "#", 2)
	key := parts[1]

	parts = strings.SplitN(parts[0], "/data/", 2)
	path := parts[1]
	mount := parts[0]

	cfg := vault.DefaultConfig()
	cfg.Address = addr
	client := dipper.Must(vault.NewClient(cfg)).(*vault.Client)
	if k8sRole != "" {
		k8sAuth := dipper.Must(auth.NewKubernetesAuth(k8sRole)).(*auth.KubernetesAuth)
		_ = dipper.Must(client.Auth().Login(context.Background(), k8sAuth))
	} else {
		client.SetToken(token)
	}

	var secret *vault.KVSecret
	if version >= 0 {
		secret = dipper.Must(client.KVv2(mount).GetVersion(context.Background(), path, version)).(*vault.KVSecret)
	} else {
		secret = dipper.Must(client.KVv2(mount).Get(context.Background(), path)).(*vault.KVSecret)
	}

	value, found := secret.Data[key]
	if !found {
		panic(ErrSecretKeyNotFound)
	}

	msg.Reply <- dipper.Message{
		Payload: []byte(value.(string)),
		IsRaw:   true,
	}
}
