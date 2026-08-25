// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package redisclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	"github.com/go-redis/redis/v8"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tokenProviderFunc func(context.Context) (*auth.Token, error)

func (f tokenProviderFunc) Token(ctx context.Context) (*auth.Token, error) {
	return f(ctx)
}

func testDriver(connection map[string]interface{}) *dipper.Driver {
	return &dipper.Driver{
		Options: map[string]interface{}{
			"data": map[string]interface{}{
				"connection": connection,
			},
		},
	}
}

func TestGetRedisOptsConfiguresIAMAuth(t *testing.T) {
	original := newIAMTokenProvider
	defer func() { newIAMTokenProvider = original }()

	provider := tokenProviderFunc(func(context.Context) (*auth.Token, error) {
		return &auth.Token{Value: "test-token", Expiry: time.Now().Add(time.Hour)}, nil
	})
	newIAMTokenProvider = func() (auth.TokenProvider, error) { return provider, nil }

	opts := GetRedisOpts(testDriver(map[string]interface{}{
		"Addr": "127.0.0.1:6379",
		"IAM": map[string]interface{}{
			"Enabled": true,
		},
		"TLS": map[string]interface{}{
			"Enabled": true,
		},
	}))

	assert.NotNil(t, opts.TLSConfig)
	assert.NotNil(t, opts.OnConnect)
	assert.Empty(t, opts.Username)
	assert.Empty(t, opts.Password)
	assert.Equal(t, 0, opts.DB)
}

func TestGetRedisOptsWithoutIAMDoesNotLoadCredentials(t *testing.T) {
	original := newIAMTokenProvider
	defer func() { newIAMTokenProvider = original }()

	providerCalls := 0
	newIAMTokenProvider = func() (auth.TokenProvider, error) {
		providerCalls++

		return nil, errors.New("credentials should not be loaded")
	}

	opts := GetRedisOpts(testDriver(map[string]interface{}{
		"Addr":     "127.0.0.1:6379",
		"Password": "static-password",
	}))

	assert.Nil(t, opts.OnConnect)
	assert.Equal(t, 0, providerCalls)
}

func TestGetRedisOptsReportsIAMCredentialInitializationError(t *testing.T) {
	original := newIAMTokenProvider
	defer func() { newIAMTokenProvider = original }()
	newIAMTokenProvider = func() (auth.TokenProvider, error) {
		return nil, errors.New("credentials unavailable")
	}

	assert.PanicsWithError(
		t,
		"initialize application default credentials for Redis IAM authentication: credentials unavailable",
		func() {
			GetRedisOpts(testDriver(map[string]interface{}{
				"IAM": map[string]interface{}{"Enabled": true},
				"TLS": map[string]interface{}{"Enabled": true},
			}))
		},
	)
}

func TestGetRedisOptsRejectsInvalidIAMAuthConfiguration(t *testing.T) {
	original := newIAMTokenProvider
	defer func() { newIAMTokenProvider = original }()
	newIAMTokenProvider = func() (auth.TokenProvider, error) {
		return tokenProviderFunc(func(context.Context) (*auth.Token, error) {
			return &auth.Token{Value: "test-token"}, nil
		}), nil
	}

	tests := []struct {
		name       string
		connection map[string]interface{}
		err        string
	}{
		{
			name: "TLS disabled",
			connection: map[string]interface{}{
				"IAM": map[string]interface{}{"Enabled": true},
			},
			err: "redis IAM authentication requires TLS",
		},
		{
			name: "static username",
			connection: map[string]interface{}{
				"Username": "default",
				"IAM":      map[string]interface{}{"Enabled": true},
				"TLS":      map[string]interface{}{"Enabled": true},
			},
			err: "redis IAM authentication cannot be combined with a static username or password",
		},
		{
			name: "static password",
			connection: map[string]interface{}{
				"Password": "static-password",
				"IAM":      map[string]interface{}{"Enabled": true},
				"TLS":      map[string]interface{}{"Enabled": true},
			},
			err: "redis IAM authentication cannot be combined with a static username or password",
		},
		{
			name: "non-default DB",
			connection: map[string]interface{}{
				"DB":  "1",
				"IAM": map[string]interface{}{"Enabled": true},
				"TLS": map[string]interface{}{"Enabled": true},
			},
			err: "redis IAM authentication requires DB 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.PanicsWithError(t, tt.err, func() {
				GetRedisOpts(testDriver(tt.connection))
			})
		})
	}
}

func TestIAMAuthRejectsUnusableTokens(t *testing.T) {
	tests := []struct {
		name     string
		provider auth.TokenProvider
		err      string
	}{
		{
			name: "provider error",
			provider: tokenProviderFunc(func(context.Context) (*auth.Token, error) {
				return nil, errors.New("metadata unavailable")
			}),
			err: "retrieve IAM access token for Redis connection: metadata unavailable",
		},
		{
			name: "empty token",
			provider: tokenProviderFunc(func(context.Context) (*auth.Token, error) {
				return &auth.Token{}, nil
			}),
			err: "retrieve IAM access token for Redis connection: token is empty",
		},
		{
			name: "expired token",
			provider: tokenProviderFunc(func(context.Context) (*auth.Token, error) {
				return &auth.Token{Value: "expired", Expiry: time.Now().Add(-time.Minute)}, nil
			}),
			err: "retrieve IAM access token for Redis connection: token is expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := testDriver(map[string]interface{}{
				"IAM": map[string]interface{}{"Enabled": true},
				"TLS": map[string]interface{}{"Enabled": true},
			})
			opts := &redis.Options{TLSConfig: setupTLSConfig(driver)}
			original := newIAMTokenProvider
			newIAMTokenProvider = func() (auth.TokenProvider, error) { return tt.provider, nil }
			defer func() { newIAMTokenProvider = original }()

			require.NoError(t, setupIAMAuth(driver, opts))
			err := opts.OnConnect(context.Background(), nil)
			assert.EqualError(t, err, tt.err)
		})
	}
}

func TestIAMAuthRetrievesCredentialsForEachConnection(t *testing.T) {
	var mu sync.Mutex
	tokens := []string{"first-token", "second-token"}
	nextToken := 0
	provider := tokenProviderFunc(func(context.Context) (*auth.Token, error) {
		mu.Lock()
		defer mu.Unlock()
		token := tokens[nextToken]
		nextToken++

		return &auth.Token{Value: token, Expiry: time.Now().Add(time.Hour)}, nil
	})

	driver := testDriver(map[string]interface{}{
		"IAM": map[string]interface{}{"Enabled": true},
		"TLS": map[string]interface{}{"Enabled": true},
	})
	opts := &redis.Options{TLSConfig: setupTLSConfig(driver)}
	original := newIAMTokenProvider
	newIAMTokenProvider = func() (auth.TokenProvider, error) { return provider, nil }
	defer func() { newIAMTokenProvider = original }()
	require.NoError(t, setupIAMAuth(driver, opts))

	authCommands := make(chan []string, len(tokens))
	opts.Dialer = func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		go serveTestRedisConnection(t, server, authCommands)

		return client, nil
	}

	for range tokens {
		clientOpts := *opts
		client := redis.NewClient(&clientOpts)
		require.NoError(t, client.Ping(context.Background()).Err())
		require.NoError(t, client.Close())
	}

	for _, token := range tokens {
		command := <-authCommands
		require.Len(t, command, 3)
		assert.Equal(t, "AUTH", strings.ToUpper(command[0]))
		assert.Equal(t, valkeyIAMUsername, command[1])
		assert.Equal(t, token, command[2])
	}
	assert.Equal(t, len(tokens), nextToken)
}

func serveTestRedisConnection(t *testing.T, conn net.Conn, authCommands chan<- []string) {
	t.Helper()
	defer conn.Close()
	reader := bufio.NewReader(conn)

	authCommand, err := readRESPCommand(reader)
	if !assert.NoError(t, err) {
		return
	}
	authCommands <- authCommand
	if _, err := fmt.Fprint(conn, "+OK\r\n"); !assert.NoError(t, err) {
		return
	}

	pingCommand, err := readRESPCommand(reader)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, []string{"ping"}, pingCommand) {
		return
	}
	_, err = fmt.Fprint(conn, "+PONG\r\n")
	assert.NoError(t, err)
}

func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read RESP array header: %w", err)
	}
	if len(header) < 4 || header[0] != '*' {
		return nil, fmt.Errorf("invalid RESP array header %q", strings.TrimSpace(header))
	}
	count, err := strconv.Atoi(strings.TrimSpace(header[1:]))
	if err != nil {
		return nil, fmt.Errorf("parse RESP array length: %w", err)
	}

	command := make([]string, count)
	for i := range command {
		lengthLine, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read RESP bulk string header: %w", err)
		}
		if len(lengthLine) < 4 || lengthLine[0] != '$' {
			return nil, fmt.Errorf("invalid RESP bulk string header %q", strings.TrimSpace(lengthLine))
		}
		length, err := strconv.Atoi(strings.TrimSpace(lengthLine[1:]))
		if err != nil {
			return nil, fmt.Errorf("parse RESP bulk string length: %w", err)
		}
		value := make([]byte, length+2)
		if _, err := io.ReadFull(reader, value); err != nil {
			return nil, fmt.Errorf("read RESP bulk string value: %w", err)
		}
		command[i] = string(value[:length])
	}

	return command, nil
}
