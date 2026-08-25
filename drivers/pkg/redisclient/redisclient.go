// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redisclient is shared by various redis drivers.
package redisclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"github.com/go-redis/redis/v8"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

const (
	cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
	valkeyIAMUsername  = "default"
)

var (
	errIAMRequiresTLS      = errors.New("redis IAM authentication requires TLS")
	errIAMWithStaticAuth   = errors.New("redis IAM authentication cannot be combined with a static username or password")
	errIAMWithNonDefaultDB = errors.New("redis IAM authentication requires DB 0")
	errIAMTokenEmpty       = errors.New("retrieve IAM access token for Redis connection: token is empty")
	errIAMTokenExpired     = errors.New("retrieve IAM access token for Redis connection: token is expired")
	newIAMTokenProvider    = defaultIAMTokenProvider
)

func defaultIAMTokenProvider() (auth.TokenProvider, error) {
	tokenProvider, err := credentials.DetectDefault(&credentials.DetectOptions{
		Scopes: []string{cloudPlatformScope},
	})
	if err != nil {
		return nil, fmt.Errorf("detect application default credentials: %w", err)
	}

	return tokenProvider, nil
}

// Options wraps redis.Options and provide a persisted redis.Client.
type Options struct {
	*redis.Client
	*redis.Options
}

// Close method hides redis.Client Close method so it can be reused.
func (o Options) Close() error {
	return nil
}

func verifyPeerCertificate(config *tls.Config, rawCerts [][]byte, _ [][]*x509.Certificate) error {
	// the function does the samething as the part this is skipped due to
	// InsecureSkipVerify in the verifyServerCertificate function from tls
	// handshake_client.go except that it doesn't do the name and SANs checking.

	var err error
	certs := make([]*x509.Certificate, len(rawCerts))
	for i, asn1Data := range rawCerts {
		certs[i], err = x509.ParseCertificate(asn1Data)
		if err != nil {
			//nolint:wrapcheck
			return err
		}
	}
	vOpts := x509.VerifyOptions{
		Roots:         config.RootCAs,
		CurrentTime:   time.Now(),
		DNSName:       "", // this has be blank to skip verifying names against CN or SANs
		Intermediates: x509.NewCertPool(),
	}
	for _, cert := range certs[1:] {
		vOpts.Intermediates.AddCert(cert)
	}
	_, err = certs[0].Verify(vOpts)

	//nolint:wrapcheck
	return err
}

func setupTLSConfig(driver *dipper.Driver) *tls.Config {
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if driver.CheckOption("data.connection.TLS.InsecureSkipVerify") {
		config.InsecureSkipVerify = true
	} else {
		serverName, ok := driver.GetOptionStr("data.connection.TLS.VerifyServerName")
		if ok && strings.TrimSpace(serverName) != "*" {
			config.ServerName = serverName
		} else if ok {
			// golang tls doesn't support verify certificate without any SANs, using
			// InsecureSkipVerify and a custom VerifyPeerCertificate to support this,
			// so we can use google memorystore redis with TLS.

			config.InsecureSkipVerify = true
			config.VerifyPeerCertificate = func(rawCerts [][]byte, chains [][]*x509.Certificate) error {
				return verifyPeerCertificate(config, rawCerts, chains)
			}
		}

		var pemblocks []interface{}
		if caCerts, ok := driver.GetOption("data.connection.TLS.CACerts"); ok {
			pemblocks = caCerts.([]interface{})
		}
		config.RootCAs = dipper.LoadCACerts(pemblocks, true)
	}

	return config
}

func setupIAMAuth(driver *dipper.Driver, opts *redis.Options) error {
	if !driver.CheckOption("data.connection.IAM.Enabled") {
		return nil
	}
	if opts.TLSConfig == nil {
		return errIAMRequiresTLS
	}
	if opts.Username != "" || opts.Password != "" {
		return errIAMWithStaticAuth
	}
	if opts.DB != 0 {
		return errIAMWithNonDefaultDB
	}

	tokenProvider, err := newIAMTokenProvider()
	if err != nil {
		return fmt.Errorf("initialize application default credentials for Redis IAM authentication: %w", err)
	}

	opts.OnConnect = func(ctx context.Context, conn *redis.Conn) error {
		token, err := tokenProvider.Token(ctx)
		if err != nil {
			return fmt.Errorf("retrieve IAM access token for Redis connection: %w", err)
		}
		if token == nil || token.Value == "" {
			return errIAMTokenEmpty
		}
		if !token.Expiry.IsZero() && !token.Expiry.After(time.Now()) {
			return errIAMTokenExpired
		}
		if err := conn.AuthACL(ctx, valkeyIAMUsername, token.Value).Err(); err != nil {
			return fmt.Errorf("authenticate Redis connection with IAM: %w", err)
		}

		return nil
	}

	return nil
}

// GetRedisOpts configures driver to talk to Redis.
func GetRedisOpts(driver *dipper.Driver) *Options {
	if conn, ok := dipper.GetMapData(driver.Options, "data.connection"); ok {
		defer delete(conn.(map[string]interface{}), "Password")
	}
	if tls, ok := dipper.GetMapData(driver.Options, "data.connection.TLS"); ok {
		defer delete(tls.(map[string]interface{}), "CACerts")
	}

	if localRedis, ok := os.LookupEnv("LOCALREDIS"); ok && localRedis != "" {
		if opts, e := redis.ParseURL(localRedis); e == nil {
			return &Options{
				Options: opts,
			}
		}

		return &Options{
			Options: &redis.Options{
				Addr: "127.0.0.1:6379",
				DB:   0,
			},
		}
	}

	opts := &redis.Options{}
	if value, ok := driver.GetOptionStr("data.connection.Addr"); ok {
		opts.Addr = value
	}
	if value, ok := driver.GetOptionStr("data.connection.Username"); ok {
		opts.Username = value
	}
	if value, ok := driver.GetOptionStr("data.connection.Password"); ok {
		opts.Password = value
	}
	if DB, ok := driver.GetOptionStr("data.connection.DB"); ok {
		opts.DB = dipper.Must(strconv.Atoi(DB)).(int)
	}
	if driver.CheckOption("data.connection.TLS.Enabled") {
		opts.TLSConfig = setupTLSConfig(driver)
	}
	dipper.Must(setupIAMAuth(driver, opts))

	return &Options{
		Options: opts,
	}
}

// NewClient wraps around redis.NewClient method so we can inject a wrapper for redis.Client.
func NewClient(c *Options) Options {
	if c.Client == nil {
		c.Client = redis.NewClient(c.Options)
	}

	return *c
}
