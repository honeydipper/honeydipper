// Copyright 2024 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package hd-driver-vault enables Honeydipper to use secrets stored in Hashicorp Vault.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	vault "github.com/hashicorp/vault/api"
	approle "github.com/hashicorp/vault/api/auth/approle"
	auth "github.com/hashicorp/vault/api/auth/kubernetes"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	secureexec "github.com/honeydipper/honeydipper/v4/pkg/secure-exec"
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

var driver *secureexec.SecureExec

func main() {
	initFlags()
	flag.Parse()

	driver = secureexec.NewDriver(os.Args[1], "vault")
	driver.RPCHandlers["lookup"] = lookup
	driver.Run()
}

func lookup(msg *dipper.Message) {
	query := string(msg.Payload.([]byte))
	parts := strings.SplitN(query, ":", 2)

	var addr, token, k8sRole, appRoleID, appSecretID string
	// Go away you freaking linter! This is easy enough to understand!
	//nolint:nestif
	if len(parts) > 1 {
		query = parts[1]
		server := parts[0]
		addr = dipper.MustGetMapDataStr(driver.Options, "data."+server+".addr")
		token, _ = dipper.GetMapDataStr(driver.Options, "data."+server+".token")
		k8sRole, _ = dipper.GetMapDataStr(driver.Options, "data."+server+".k8sRole")
		appRoleID, _ = dipper.GetMapDataStr(driver.Options, "data."+server+".approle.role_id")
		appSecretID, _ = dipper.GetMapDataStr(driver.Options, "data."+server+".approle.secret_id")
	} else {
		addr, _ = dipper.GetMapDataStr(driver.Options, "data.addr")
		if addr == "" {
			addr = os.Getenv("VAULT_ADDR")
		}
		token, _ = dipper.GetMapDataStr(driver.Options, "data.token")
		if token == "" {
			token = os.Getenv("VAULT_TOKEN")
		}
		k8sRole, _ = dipper.GetMapDataStr(driver.Options, "data.k8sRole")
		appRoleID, _ = dipper.GetMapDataStr(driver.Options, "data.approle.role_id")
		if appRoleID == "" {
			appRoleID = os.Getenv("VAULT_ROLE_ID")
		}
		appSecretID, _ = dipper.GetMapDataStr(driver.Options, "data.approle.secret_id")
		if appSecretID == "" {
			appSecretID = os.Getenv("VAULT_SECRET_ID")
		}
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

	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	cfg := vault.DefaultConfig()
	cfg.Address = addr
	client := dipper.Must(vault.NewClient(cfg)).(*vault.Client)
	switch {
	case k8sRole != "":
		k8sAuth := dipper.Must(auth.NewKubernetesAuth(k8sRole)).(*auth.KubernetesAuth)
		_ = dipper.Must(client.Auth().Login(ctx, k8sAuth))
	case appRoleID != "":
		appRoleAuth := dipper.Must(approle.NewAppRoleAuth(appRoleID, &approle.SecretID{FromString: appSecretID})).(*approle.AppRoleAuth)
		_ = dipper.Must(client.Auth().Login(ctx, appRoleAuth))
	default:
		client.SetToken(token)
	}

	var secret *vault.KVSecret
	if version >= 0 {
		secret = dipper.Must(client.KVv2(mount).GetVersion(ctx, path, version)).(*vault.KVSecret)
	} else {
		secret = dipper.Must(client.KVv2(mount).Get(ctx, path)).(*vault.KVSecret)
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
