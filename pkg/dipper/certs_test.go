// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package dipper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadCACerts(t *testing.T) {
	certs := []interface{}{
		`
-----BEGIN CERTIFICATE-----
MIIBuDCCAV4CCQDX6zn1LwUpWDAKBggqhkjOPQQDAjBkMQswCQYDVQQGEwJVUzEL
MAkGA1UECAwCQ0ExFDASBgNVBAcMC0xvcyBBbmdlbGVzMRAwDgYDVQQKDAdDb21w
YW55MRAwDgYDVQQLDAdTZWN0aW9uMQ4wDAYDVQQDDAVUZXN0MTAeFw0yMTAxMjQw
MTIwNDFaFw0yMjAxMjQwMTIwNDFaMGQxCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJD
QTEUMBIGA1UEBwwLTG9zIEFuZ2VsZXMxEDAOBgNVBAoMB0NvbXBhbnkxEDAOBgNV
BAsMB1NlY3Rpb24xDjAMBgNVBAMMBVRlc3QxMFkwEwYHKoZIzj0CAQYIKoZIzj0D
AQcDQgAELv64EsaVD0o3JJG/UMuVPmo570VN3hxtKB3nkJ3tNXYaurSgpe1AohMX
/0avIuqnzlxM2JKkevYgW/HJyY+oVzAKBggqhkjOPQQDAgNIADBFAiBcDGOA7sy8
MPZ+cqSWEXI4VAanhKCFxpN3urEb+mLtbAIhANjpn1Hjfbx5XSwWrHPfoCAoRzPd
KiCFvgTSZQojAG8t
-----END CERTIFICATE-----
		`,
		`
-----BEGIN CERTIFICATE-----
MIIBszCCAVgCCQCb9f3CUhy1wzAKBggqhkjOPQQDAjBhMQswCQYDVQQGEwJVUzEL
MAkGA1UECAwCQ0ExETAPBgNVBAcMCFBhc2FkZW5hMRAwDgYDVQQKDAdDb21wYW55
MRAwDgYDVQQLDAdTZWN0aW9uMQ4wDAYDVQQDDAVUZXN0MjAeFw0yMTAxMjQwMTIx
NDBaFw0yMjAxMjQwMTIxNDBaMGExCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJDQTER
MA8GA1UEBwwIUGFzYWRlbmExEDAOBgNVBAoMB0NvbXBhbnkxEDAOBgNVBAsMB1Nl
Y3Rpb24xDjAMBgNVBAMMBVRlc3QyMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE
Lv64EsaVD0o3JJG/UMuVPmo570VN3hxtKB3nkJ3tNXYaurSgpe1AohMX/0avIuqn
zlxM2JKkevYgW/HJyY+oVzAKBggqhkjOPQQDAgNJADBGAiEAwXclv5JcBBOAFcJ1
mTh7GDlF5m4UBEQUAx4Sm4nVEAoCIQDoenOGwxdEvw5xbeZizct50bOZp1dU4u2c
YYaBtD/3UQ==
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIBrTCCAVICCQC6ufl87XwWCjAKBggqhkjOPQQDAjBeMQswCQYDVQQGEwJVUzEL
MAkGA1UECAwCQ0ExDjAMBgNVBAcMBUF6dXNhMRAwDgYDVQQKDAdDb21wYW55MRAw
DgYDVQQLDAdTZWN0aW9uMQ4wDAYDVQQDDAVUZXN0MzAeFw0yMTAxMjQwMTIyMTha
Fw0yMjAxMjQwMTIyMThaMF4xCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJDQTEOMAwG
A1UEBwwFQXp1c2ExEDAOBgNVBAoMB0NvbXBhbnkxEDAOBgNVBAsMB1NlY3Rpb24x
DjAMBgNVBAMMBVRlc3QzMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAELv64EsaV
D0o3JJG/UMuVPmo570VN3hxtKB3nkJ3tNXYaurSgpe1AohMX/0avIuqnzlxM2JKk
evYgW/HJyY+oVzAKBggqhkjOPQQDAgNJADBGAiEA3wxNoM814xT/QVpUoQrmhurE
s99PXe7pSqqKPGBq5Y0CIQCK367N+dWphmnMV4z3RO2loTPDlJaGh2Jz0VhQ4AE/
Ow==
-----END CERTIFICATE-----
		`,
	}
	assert.NotPanics(t, func() { LoadCACerts(nil, true) }, "loading system cert pool should not panic")
	assert.NotPanics(t, func() { LoadCACerts(nil, false) }, "loading an empty pool should not panic")
	assert.NotPanics(t, func() { LoadCACerts(certs, false) }, "loading user specified root CA should not panic")
	assert.NotPanics(t, func() { LoadCACerts(certs, true) }, "loading user specified root CA on top of system certs should not panic")
}
