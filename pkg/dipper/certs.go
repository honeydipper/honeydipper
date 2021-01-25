// Copyright 2021 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package dipper is a library used for developing drivers for Honeydipper.
package dipper

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
)

// LoadCACerts loads CA certs on top of the system certs from SSL_CERT_FILE or SSL_CERT_DIR.
func LoadCACerts(pemblocks []interface{}, includeSystemCerts bool) *x509.CertPool {
	var (
		cp  *x509.CertPool
		err error
	)

	if includeSystemCerts {
		cp, err = x509.SystemCertPool()
		if err != nil {
			Logger.Warningf("unable to load system cert pool: %+v", err)
		}
	}

	if cp == nil {
		cp = x509.NewCertPool()
	}

	for _, pemblock := range pemblocks {
		rest := []byte(strings.TrimSpace(pemblock.(string)))
		for len(rest) > 0 {
			var block *pem.Block
			block, rest = pem.Decode(rest)

			switch {
			case block == nil:
				Logger.Warningf("skipping non cert block")
			case block.Type != "CERTIFICATE":
				Logger.Warningf("skipping non cert block: %s", block.Type)
			default:
				cert, err := x509.ParseCertificate(block.Bytes)
				if err != nil {
					Logger.Warningf("skipping invalid CA cert due to: %s", err)
				} else {
					cp.AddCert(cert)
				}
			}
		}
	}

	return cp
}
