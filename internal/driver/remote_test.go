// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package driver

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRemoteDriver(t *testing.T) {
	m := &Meta{
		Name: "test",
		Type: "remote",
	}

	dh := NewRemoteDriver(m)
	assert.Equal(t, m, dh.meta, "new remote driver should have meta")
}

func TestRemoteAcquire(t *testing.T) {
	cacheDir := t.TempDir()
	RemotePath = cacheDir
	t.Cleanup(func() {
		RemotePath = ""
	})

	payload := []byte("#!/bin/sh\necho test\n")
	sha := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sha[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	testCases := map[string]interface{}{
		"panic when url is missing": []interface{}{
			&RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: &Meta{Name: "test", HandlerData: map[string]interface{}{}}}},
			"driver error: url is missing for remote driver: ",
		},
		"panic when sha256 is missing": []interface{}{
			&RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: &Meta{Name: "test", HandlerData: map[string]interface{}{"url": srv.URL + "/driver"}}}},
			"driver error: sha256 is missing for remote driver: ",
		},
		"panic when sha256 is malformed": []interface{}{
			&RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: &Meta{Name: "test", HandlerData: map[string]interface{}{"url": srv.URL + "/driver", "sha256": "not-a-sha"}}}},
			"driver error: invalid sha256 for remote driver: ",
		},
		"download and cache driver": []interface{}{
			&RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: &Meta{Name: "test", HandlerData: map[string]interface{}{"url": srv.URL + "/driver", "sha256": shaHex, "fileName": "hd-driver-test"}}}},
			"",
			filepath.Join(cacheDir, "sha256", shaHex, "hd-driver-test"),
		},
	}

	for msg, tc := range testCases {
		func(c []interface{}) {
			defer func() {
				if r := recover(); r != nil {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Equal(t, c[1], r.(error).Error()[:len(c[1].(string))], msg)
					} else {
						assert.Fail(t, "should "+msg)
					}
				} else {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Fail(t, "should "+msg)
					} else {
						assert.Equal(t, c[2].(string), c[0].(*RemoteDriver).meta.Executable, "should "+msg)
						info, err := os.Stat(c[2].(string))
						assert.Nil(t, err, "should "+msg)
						assert.True(t, info.Mode().Perm()&0o100 > 0, "should be executable")
					}
				}
			}()
			c[0].(*RemoteDriver).Acquire()
		}(tc.([]interface{}))
	}
}

func TestRemoteAcquireUsesCache(t *testing.T) {
	cacheDir := t.TempDir()
	RemotePath = cacheDir
	t.Cleanup(func() {
		RemotePath = ""
	})

	payload := []byte("#!/bin/sh\necho cached\n")
	sha := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sha[:])

	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	d := &RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: &Meta{
		Name: "cached-driver",
		Type: "remote",
		HandlerData: map[string]interface{}{
			"url":      srv.URL + "/driver",
			"sha256":   shaHex,
			"fileName": "cached-driver",
		},
	}}}

	d.Acquire()
	d.Acquire()

	assert.Equal(t, 1, requestCount, "second acquire should hit cache")
}

func TestRemoteAcquireSignature(t *testing.T) {
	cacheDir := t.TempDir()
	RemotePath = cacheDir
	t.Cleanup(func() {
		RemotePath = ""
	})

	payload := []byte("#!/bin/sh\necho signed\n")
	sha := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sha[:])

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	assert.Nil(t, err)
	signature := ed25519.Sign(privateKey, sha[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	testCases := map[string]interface{}{
		"panic when signature required but missing": []interface{}{
			&RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: &Meta{Name: "signed", HandlerData: map[string]interface{}{
				"url":              srv.URL + "/driver",
				"sha256":           shaHex,
				"requireSignature": true,
			}}}},
			"driver error: publicKey is missing for remote driver",
		},
		"download and verify signed driver": []interface{}{
			&RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: &Meta{Name: "signed", HandlerData: map[string]interface{}{
				"url":              srv.URL + "/driver",
				"sha256":           shaHex,
				"fileName":         "hd-driver-signed",
				"requireSignature": true,
				"publicKey":        base64.StdEncoding.EncodeToString(publicKey),
				"signature":        base64.StdEncoding.EncodeToString(signature),
			}}}},
			"",
			filepath.Join(cacheDir, "sha256", shaHex, "hd-driver-signed"),
		},
	}

	for msg, tc := range testCases {
		func(c []interface{}) {
			defer func() {
				if r := recover(); r != nil {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Equal(t, c[1], r.(error).Error()[:len(c[1].(string))], msg)
					} else {
						assert.Fail(t, "should "+msg)
					}
				} else {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Fail(t, "should "+msg)
					} else {
						assert.Equal(t, c[2].(string), c[0].(*RemoteDriver).meta.Executable, "should "+msg)
					}
				}
			}()
			c[0].(*RemoteDriver).Acquire()
		}(tc.([]interface{}))
	}
}

func TestRemoteAcquireRegistry(t *testing.T) {
	cacheDir := t.TempDir()
	RemotePath = cacheDir
	t.Cleanup(func() {
		RemotePath = ""
	})

	payload := []byte("#!/bin/sh\necho registry\n")
	sha := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sha[:])

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	assert.Nil(t, err)
	signature := ed25519.Sign(privateKey, sha[:])

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry/registry-driver.json":
			_, _ = fmt.Fprintf(w, `{
				"driver": "registry-driver",
				"channels": {"stable": "1.0.0"},
				"versions": {
					"1.0.0": {
						"artifacts": [{
							"os": %q,
							"arch": %q,
							"url": %q,
							"sha256": %q,
							"fileName": "hd-driver-registry",
							"publicKey": %q,
							"signature": %q
						}]
					}
				}
			}`,
				runtime.GOOS,
				runtime.GOARCH,
				srv.URL+"/artifact/registry-driver",
				shaHex,
				base64.StdEncoding.EncodeToString(publicKey),
				base64.StdEncoding.EncodeToString(signature),
			)
		case "/artifact/registry-driver":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	testCases := map[string]interface{}{
		"download from registry channel": []interface{}{
			&RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: &Meta{Name: "registry-driver", HandlerData: map[string]interface{}{
				"registryURL":      srv.URL + "/registry",
				"channel":          "stable",
				"requireSignature": true,
			}}}},
			"",
			filepath.Join(cacheDir, "sha256", shaHex, "hd-driver-registry"),
		},
		"panic when registry version is missing": []interface{}{
			&RemoteDriver{BuiltinDriver: &BuiltinDriver{meta: &Meta{Name: "registry-driver", HandlerData: map[string]interface{}{
				"registryURL": srv.URL + "/registry",
				"version":     "9.9.9",
			}}}},
			"driver error: failed resolving remote driver version from registry",
		},
	}

	for msg, tc := range testCases {
		func(c []interface{}) {
			defer func() {
				if r := recover(); r != nil {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Equal(t, c[1], r.(error).Error()[:len(c[1].(string))], msg)
					} else {
						assert.Fail(t, "should "+msg)
					}
				} else {
					if len(c) > 1 && len(c[1].(string)) > 0 {
						assert.Fail(t, "should "+msg)
					} else {
						assert.Equal(t, c[2].(string), c[0].(*RemoteDriver).meta.Executable, "should "+msg)
					}
				}
			}()
			c[0].(*RemoteDriver).Acquire()
		}(tc.([]interface{}))
	}
}
