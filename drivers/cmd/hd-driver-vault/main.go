// Copyright 2026 PayPal Inc.

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
	"sort"
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

const (
	scopedPlaceholder = "{SCOPED}"
	scopedPrefixEnv   = "SECRET_PREFIX_"
)

// vaultConfig holds vault connection configuration.
type vaultConfig struct {
	addr        string
	token       string
	k8sRole     string
	appRoleID   string
	appSecretID string
}

// getVaultConfig retrieves vault configuration from driver options or environment variables.
// If serverName is provided, it looks for "data.<serverName>.*" configs, otherwise uses "data.*".
func getVaultConfig(serverName string) vaultConfig {
	cfg := vaultConfig{}
	keyPrefix := "data."
	if serverName != "" {
		keyPrefix = "data." + serverName + "."
	}

	cfg.addr, _ = dipper.GetMapDataStr(driver.Options, keyPrefix+"addr")
	if cfg.addr == "" {
		cfg.addr = os.Getenv("VAULT_ADDR")
	}

	cfg.token, _ = dipper.GetMapDataStr(driver.Options, keyPrefix+"token")
	if cfg.token == "" {
		cfg.token = os.Getenv("VAULT_TOKEN")
	}

	cfg.k8sRole, _ = dipper.GetMapDataStr(driver.Options, keyPrefix+"k8sRole")
	if cfg.k8sRole == "" {
		cfg.k8sRole = os.Getenv("VAULT_K8S_ROLE")
	}

	cfg.appRoleID, _ = dipper.GetMapDataStr(driver.Options, keyPrefix+"approle.role_id")
	if cfg.appRoleID == "" {
		cfg.appRoleID = os.Getenv("VAULT_ROLE_ID")
	}

	cfg.appSecretID, _ = dipper.GetMapDataStr(driver.Options, keyPrefix+"approle.secret_id")
	if cfg.appSecretID == "" {
		cfg.appSecretID = os.Getenv("VAULT_SECRET_ID")
	}

	return cfg
}

// createVaultClient creates and authenticates a vault client using the provided configuration.
func createVaultClient(ctx context.Context, cfg vaultConfig) *vault.Client {
	c := vault.DefaultConfig()
	c.Address = cfg.addr
	client := dipper.Must(vault.NewClient(c)).(*vault.Client)

	switch {
	case cfg.k8sRole != "":
		k8sAuth := dipper.Must(auth.NewKubernetesAuth(cfg.k8sRole)).(*auth.KubernetesAuth)
		_ = dipper.Must(client.Auth().Login(ctx, k8sAuth))
	case cfg.appRoleID != "":
		appRoleAuth := dipper.Must(approle.NewAppRoleAuth(cfg.appRoleID, &approle.SecretID{FromString: cfg.appSecretID})).(*approle.AppRoleAuth)
		_ = dipper.Must(client.Auth().Login(ctx, appRoleAuth))
	default:
		client.SetToken(cfg.token)
	}

	return client
}

// parsePath splits a vault path into server name, mount point, and secret path.
// Format: [serverName:]mount/data/path
// Returns: serverName (or ""), mount (or "secret" default), path.
func parsePath(pathStr string) (string, string, string) {
	// Check for server prefix
	parts := strings.SplitN(pathStr, ":", 2)
	serverName := ""
	actualPath := pathStr

	if len(parts) > 1 {
		serverName = parts[0]
		actualPath = parts[1]
	}

	// Parse mount and path
	mountParts := strings.SplitN(actualPath, "/data/", 2)
	var mount, path string

	if len(mountParts) > 1 {
		mount = mountParts[0]
		path = mountParts[1]
	} else {
		mount = "secret"
		path = actualPath
	}

	return serverName, mount, path
}

// buildMetadataListPath builds KV v2 metadata path for listing child secrets.
func buildMetadataListPath(mount, path string) string {
	cleanPath := strings.Trim(path, "/")
	if cleanPath == "" {
		return mount + "/metadata"
	}

	return mount + "/metadata/" + cleanPath
}

func deleteSecretKey(data map[string]interface{}, key string) (map[string]interface{}, error) {
	if _, found := data[key]; !found {
		return nil, ErrSecretKeyNotFound
	}

	delete(data, key)

	return data, nil
}

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
	driver.RPCHandlers["set"] = set
	driver.RPCHandlers["list_keys"] = listKeys
	driver.RPCHandlers["list_secrets"] = listSecrets
	driver.RPCHandlers["delete_key"] = deleteKey
	driver.RPCHandlers["delete_secret"] = deleteSecret
	driver.Run()
}

// lookupSinglePath attempts to lookup a single secret path and returns the value if found.
// Returns the value string and a boolean indicating if it was found (nil error).
func lookupSinglePath(ctx context.Context, client *vault.Client, singleQuery string) (string, error) {
	_, mount, path := parsePath(singleQuery)

	// Extract version and key from path
	pathParts := strings.SplitN(path, "@", 2)
	version := -1
	if len(pathParts) > 1 {
		version = dipper.Must(strconv.Atoi(pathParts[1])).(int)
	}

	keyParts := strings.SplitN(pathParts[0], "#", 2)
	path = keyParts[0]
	key := keyParts[1]

	var (
		secret *vault.KVSecret
		err    error
	)
	if version >= 0 {
		secret, err = client.KVv2(mount).GetVersion(ctx, path, version)
	} else {
		secret, err = client.KVv2(mount).Get(ctx, path)
	}

	if err != nil {
		return "", fmt.Errorf("failed to read vault secret %s/%s: %w", mount, path, err)
	}

	value, found := secret.Data[key]
	if !found {
		return "", ErrSecretKeyNotFound
	}

	return value.(string), nil
}

func getScopedPrefixes() []string {
	type scopedEntry struct {
		key   string
		value string
	}

	entries := []scopedEntry{}
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if !strings.HasPrefix(parts[0], scopedPrefixEnv) {
			continue
		}

		scopeKey := strings.TrimPrefix(parts[0], scopedPrefixEnv)
		if scopeKey == "" || parts[1] == "" {
			continue
		}

		entries = append(entries, scopedEntry{key: strings.ToLower(scopeKey), value: parts[1]})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})

	prefixes := make([]string, 0, len(entries))
	for _, entry := range entries {
		prefixes = append(prefixes, entry.value)
	}

	return prefixes
}

func expandScopedQueries(queryStr string) []string {
	queries := strings.Split(queryStr, ";")
	prefixes := getScopedPrefixes()

	expanded := []string{}
	for _, rawQuery := range queries {
		singleQuery := strings.TrimSpace(rawQuery)
		if singleQuery == "" {
			continue
		}

		if strings.Contains(singleQuery, scopedPlaceholder) {
			if len(prefixes) == 0 {
				expanded = append(expanded, singleQuery)

				continue
			}

			for _, prefix := range prefixes {
				expanded = append(expanded, strings.ReplaceAll(singleQuery, scopedPlaceholder, prefix))
			}

			continue
		}

		expanded = append(expanded, singleQuery)
	}

	return expanded
}

func lookup(msg *dipper.Message) {
	queryStr := string(msg.Payload.([]byte))
	queries := expandScopedQueries(queryStr)

	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	clients := map[string]*vault.Client{}

	// Try each query path in order, return the first one found
	for _, singleQuery := range queries {
		serverName, _, _ := parsePath(singleQuery)
		client, found := clients[serverName]
		if !found {
			cfg := getVaultConfig(serverName)
			client = createVaultClient(ctx, cfg)
			clients[serverName] = client
		}

		value, err := lookupSinglePath(ctx, client, singleQuery)
		if err == nil {
			// Found a valid secret, return it
			msg.Reply <- dipper.Message{
				Payload: []byte(value),
				IsRaw:   true,
			}

			return
		}
	}

	// No valid secret found in any of the paths
	panic(fmt.Errorf("%w: %s", ErrSecretKeyNotFound, queryStr))
}

func set(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	payload := msg.Payload.(map[string]interface{})

	pathStr := dipper.MustGetMapDataStr(payload, "path")
	key := dipper.MustGetMapDataStr(payload, "key")
	value := dipper.MustGetMapDataStr(payload, "value")

	serverName, mount, path := parsePath(pathStr)

	cfg := getVaultConfig(serverName)
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	client := createVaultClient(ctx, cfg)

	// Get existing secret data and merge with new key-value
	var data map[string]interface{}
	existingSecret, err := client.KVv2(mount).Get(ctx, path)
	if err == nil && existingSecret != nil {
		// Existing secret found, use its data and merge
		data = existingSecret.Data
	} else {
		// New secret or error, create fresh map
		data = make(map[string]interface{})
	}

	// Set the new key-value pair
	data[key] = value

	// Put the updated secret
	_ = dipper.Must(client.KVv2(mount).Put(ctx, path, data))

	msg.Reply <- dipper.Message{
		Payload: []byte("OK"),
		IsRaw:   true,
	}
}

func listKeys(msg *dipper.Message) {
	query := string(msg.Payload.([]byte))
	serverName, mount, path := parsePath(query)

	cfg := getVaultConfig(serverName)
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	client := createVaultClient(ctx, cfg)

	secret := dipper.Must(client.KVv2(mount).Get(ctx, path)).(*vault.KVSecret)

	// Extract keys from the secret data
	keys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		keys = append(keys, k)
	}

	msg.Reply <- dipper.Message{
		Payload: keys,
	}
}

func listSecrets(msg *dipper.Message) {
	query := string(msg.Payload.([]byte))
	serverName, mount, path := parsePath(query)

	cfg := getVaultConfig(serverName)
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	client := createVaultClient(ctx, cfg)

	listPath := buildMetadataListPath(mount, path)
	listed := dipper.Must(client.Logical().ListWithContext(ctx, listPath)).(*vault.Secret)

	secrets := []string{}
	if listed != nil {
		if keysRaw, found := listed.Data["keys"]; found {
			switch keys := keysRaw.(type) {
			case []interface{}:
				for _, key := range keys {
					if keyStr, ok := key.(string); ok {
						secrets = append(secrets, keyStr)
					}
				}
			case []string:
				secrets = append(secrets, keys...)
			}
		}
	}

	secretsJSON := dipper.Must(dipper.SerializeContent(secrets)).([]byte)

	msg.Reply <- dipper.Message{
		Payload: secretsJSON,
		IsRaw:   true,
	}
}

func deleteKey(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	payload := msg.Payload.(map[string]interface{})

	pathStr := dipper.MustGetMapDataStr(payload, "path")
	key := dipper.MustGetMapDataStr(payload, "key")

	serverName, mount, path := parsePath(pathStr)

	cfg := getVaultConfig(serverName)
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	client := createVaultClient(ctx, cfg)
	secret := dipper.Must(client.KVv2(mount).Get(ctx, path)).(*vault.KVSecret)

	updatedData, err := deleteSecretKey(secret.Data, key)
	if err != nil {
		panic(err)
	}

	if len(updatedData) == 0 {
		_ = dipper.Must(client.KVv2(mount).DeleteMetadata(ctx, path))
	} else {
		_ = dipper.Must(client.KVv2(mount).Put(ctx, path, updatedData))
	}

	msg.Reply <- dipper.Message{
		Payload: []byte("OK"),
		IsRaw:   true,
	}
}

func deleteSecret(msg *dipper.Message) {
	query := string(msg.Payload.([]byte))
	serverName, mount, path := parsePath(query)

	cfg := getVaultConfig(serverName)
	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	client := createVaultClient(ctx, cfg)
	_ = dipper.Must(client.KVv2(mount).DeleteMetadata(ctx, path))

	msg.Reply <- dipper.Message{
		Payload: []byte("OK"),
		IsRaw:   true,
	}
}
