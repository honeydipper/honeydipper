// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package driver

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
