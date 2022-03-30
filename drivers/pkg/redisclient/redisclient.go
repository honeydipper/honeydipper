// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package redisclient is shared by various redis drivers.
package redisclient

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

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

// GetRedisOpts configures driver to talk to Redis.
func GetRedisOpts(driver *dipper.Driver) *redis.Options {
	if conn, ok := dipper.GetMapData(driver.Options, "data.connection"); ok {
		defer delete(conn.(map[string]interface{}), "Password")
	}
	if tls, ok := dipper.GetMapData(driver.Options, "data.connection.TLS"); ok {
		defer delete(tls.(map[string]interface{}), "CACerts")
	}

	if localRedis, ok := os.LookupEnv("LOCALREDIS"); ok && localRedis != "" {
		if opts, e := redis.ParseURL(localRedis); e == nil {
			return opts
		}

		return &redis.Options{
			Addr: "127.0.0.1:6379",
			DB:   0,
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

	return opts
}
