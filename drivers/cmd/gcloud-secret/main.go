// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package gcloud-secret enables Honeydipper to use secrets stored in gcloud secret manager.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/gogf/gf/container/gpool"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	secureexec "github.com/honeydipper/honeydipper/v4/pkg/secure-exec"
)

// DefaultClientTTLSeconds specifies the TTL for reusable google clients.
const DefaultClientTTLSeconds = 60

// ErrSecretNameMissing means the secret name is not supplied.
var ErrSecretNameMissing = errors.New("secret name not supplied")

// ErrSecretNameInvalid means the secret name is not valid.
var ErrSecretNameInvalid = errors.New("secret name not valid")

const (
	scopedPlaceholder = "{SCOPED}"
	scopedPrefixEnv   = "SECRET_PREFIX_"
)

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
	driver      *secureexec.SecureExec
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

	driver = secureexec.NewDriver(os.Args[1], "google-secret")
	driver.RPCHandlers["lookup"] = lookup
	driver.Reload = loadOptions
	driver.Start = loadOptions
	driver.Run()
}

// lookupSingleSecret attempts to lookup a single secret and returns the data if found.
// Returns the secret data []byte and a boolean indicating if it was found (nil error).
func lookupSingleSecret(ctx context.Context, client SecretManagerClient, name string) ([]byte, error) {
	parts := strings.Split(name, "/")
	switch {
	case len(parts) == 6:
		if parts[0] != "projects" || parts[2] != "secrets" || parts[4] != "versions" {
			dipper.Logger.Warningf("incorrect secret key format %s", name)

			return nil, ErrSecretNameInvalid
		}
	case len(parts) == 2 || len(parts) == 3:
		version := "latest"
		if len(parts) == 3 {
			version = parts[2]
		}
		name = fmt.Sprintf("projects/%s/secrets/%s/versions/%s", parts[0], parts[1], version)
	default:
		dipper.Logger.Warningf("incorrect secret key format %s", name)

		return nil, ErrSecretNameInvalid
	}

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	resp, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		dipper.Logger.Debugf("failed to access secret %s: %v", name, err)

		return nil, fmt.Errorf("access secret version: %w", err)
	}

	return resp.Payload.Data, nil
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

func expandScopedNames(nameStr string) []string {
	names := strings.Split(nameStr, ";")
	prefixes := getScopedPrefixes()

	expanded := []string{}
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}

		if strings.Contains(name, scopedPlaceholder) {
			if len(prefixes) == 0 {
				expanded = append(expanded, name)

				continue
			}

			for _, prefix := range prefixes {
				expanded = append(expanded, strings.ReplaceAll(name, scopedPlaceholder, prefix))
			}

			continue
		}

		expanded = append(expanded, name)
	}

	return expanded
}

func lookup(msg *dipper.Message) {
	nameBytes, ok := msg.Payload.([]byte)
	if !ok {
		panic(ErrSecretNameMissing)
	}
	nameStr := string(nameBytes)
	names := expandScopedNames(nameStr)

	ctx, cancel := driver.GetContext(msg)
	defer cancel()

	var client interface{}
	var err error

	// Initialize client pool if not already done (handles secure loader case)
	if _clientPool == nil {
		loadOptions(msg)
	}

	// Try each secret name in order, return the first one found
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		// Get client only once
		if client == nil {
			client, err = _clientPool.Get()
			if err != nil {
				dipper.Logger.Warning("failed to get google secret manager client")
				panic(err)
			}
			defer func() { _ = _clientPool.Put(client) }()
		}

		data, err := lookupSingleSecret(ctx, client.(SecretManagerClient), name)
		if err == nil {
			// Found a valid secret, return it
			msg.Reply <- dipper.Message{
				Payload: data,
				IsRaw:   true,
			}

			return
		}
	}

	// No valid secret found in any of the names
	dipper.Logger.Warning("failed to access any of the secrets")
	panic(ErrSecretNameMissing)
}
