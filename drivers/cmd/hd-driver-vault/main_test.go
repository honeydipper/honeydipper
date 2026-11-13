// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantServer string
		wantMount  string
		wantPath   string
	}{
		{
			name:       "basic path with mount and secret",
			input:      "secret/data/myapp",
			wantServer: "",
			wantMount:  "secret",
			wantPath:   "myapp",
		},
		{
			name:       "path with nested secret",
			input:      "secret/data/myapp/prod",
			wantServer: "",
			wantMount:  "secret",
			wantPath:   "myapp/prod",
		},
		{
			name:       "path without /data/ defaults to secret mount",
			input:      "myapp",
			wantServer: "",
			wantMount:  "secret",
			wantPath:   "myapp",
		},
		{
			name:       "path with server prefix",
			input:      "vault-prod:secret/data/myapp",
			wantServer: "vault-prod",
			wantMount:  "secret",
			wantPath:   "myapp",
		},
		{
			name:       "path with server prefix and different mount",
			input:      "vault-dev:kv/data/myapp/dev",
			wantServer: "vault-dev",
			wantMount:  "kv",
			wantPath:   "myapp/dev",
		},
		{
			name:       "path with server prefix without /data/",
			input:      "vault-staging:myapp",
			wantServer: "vault-staging",
			wantMount:  "secret",
			wantPath:   "myapp",
		},
		{
			name:       "nested path with multiple components",
			input:      "secret/data/db/prod/password",
			wantServer: "",
			wantMount:  "secret",
			wantPath:   "db/prod/password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotServer, gotMount, gotPath := parsePath(tt.input)
			assert.Equal(t, tt.wantServer, gotServer, "server name mismatch")
			assert.Equal(t, tt.wantMount, gotMount, "mount mismatch")
			assert.Equal(t, tt.wantPath, gotPath, "path mismatch")
		})
	}
}

func TestErrSecretKeyNotFound(t *testing.T) {
	assert.NotNil(t, ErrSecretKeyNotFound)
	assert.Equal(t, "secret key not found in the secret", ErrSecretKeyNotFound.Error())
}

func TestParsePathEdgeCases(t *testing.T) {
	t.Run("empty path defaults to secret mount", func(t *testing.T) {
		server, mount, path := parsePath("")
		assert.Equal(t, "", server)
		assert.Equal(t, "secret", mount)
		assert.Equal(t, "", path)
	})

	t.Run("path with only colon defaults correctly", func(t *testing.T) {
		server, mount, path := parsePath(":")
		assert.Equal(t, "", server)
		assert.Equal(t, "secret", mount)
		assert.Equal(t, "", path)
	})

	t.Run("path with multiple data separators uses first one", func(t *testing.T) {
		server, mount, path := parsePath("secret/data/some/data/path")
		assert.Equal(t, "", server)
		assert.Equal(t, "secret", mount)
		assert.Equal(t, "some/data/path", path)
	})

	t.Run("server with complex path", func(t *testing.T) {
		server, mount, path := parsePath("backup:custom/data/a/b/c")
		assert.Equal(t, "backup", server)
		assert.Equal(t, "custom", mount)
		assert.Equal(t, "a/b/c", path)
	})
}

func TestBuildMetadataListPath(t *testing.T) {
	tests := []struct {
		name     string
		mount    string
		path     string
		wantPath string
	}{
		{
			name:     "empty path",
			mount:    "secret",
			path:     "",
			wantPath: "secret/metadata",
		},
		{
			name:     "simple folder path",
			mount:    "secret",
			path:     "apps",
			wantPath: "secret/metadata/apps",
		},
		{
			name:     "nested folder path",
			mount:    "kv",
			path:     "apps/prod",
			wantPath: "kv/metadata/apps/prod",
		},
		{
			name:     "path with leading slash",
			mount:    "secret",
			path:     "/apps/prod",
			wantPath: "secret/metadata/apps/prod",
		},
		{
			name:     "path with trailing slash",
			mount:    "secret",
			path:     "apps/prod/",
			wantPath: "secret/metadata/apps/prod",
		},
		{
			name:     "path with surrounding slashes",
			mount:    "secret",
			path:     "/apps/prod/",
			wantPath: "secret/metadata/apps/prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath := buildMetadataListPath(tt.mount, tt.path)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}

func TestDeleteSecretKey(t *testing.T) {
	t.Run("delete existing key", func(t *testing.T) {
		data := map[string]interface{}{
			"foo": "bar",
			"baz": "qux",
		}

		updated, err := deleteSecretKey(data, "foo")

		assert.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"baz": "qux"}, updated)
	})

	t.Run("delete only key leaves empty map", func(t *testing.T) {
		data := map[string]interface{}{
			"foo": "bar",
		}

		updated, err := deleteSecretKey(data, "foo")

		assert.NoError(t, err)
		assert.Equal(t, map[string]interface{}{}, updated)
	})

	t.Run("delete missing key returns error", func(t *testing.T) {
		data := map[string]interface{}{
			"baz": "qux",
		}

		updated, err := deleteSecretKey(data, "foo")

		assert.Nil(t, updated)
		assert.ErrorIs(t, err, ErrSecretKeyNotFound)
	})
}

func TestGetScopedPrefixes(t *testing.T) {
	t.Setenv("SECRET_PREFIX_ZZZ", "env/zzz")
	t.Setenv("SECRET_PREFIX_ALPHA", "env/alpha")
	t.Setenv("SECRET_PREFIX_MID", "env/mid")
	t.Setenv("UNRELATED_PREFIX_ALPHA", "ignore-me")

	prefixes := getScopedPrefixes()
	assert.Equal(t, []string{"env/alpha", "env/mid", "env/zzz"}, prefixes)
}

func TestExpandScopedQueries(t *testing.T) {
	t.Setenv("SECRET_PREFIX_BETA", "apps/beta")
	t.Setenv("SECRET_PREFIX_ALPHA", "apps/alpha")

	queries := expandScopedQueries("secret/data/{SCOPED}/db#password;secret/data/common/db#password")
	assert.Equal(t,
		[]string{
			"secret/data/apps/alpha/db#password",
			"secret/data/apps/beta/db#password",
			"secret/data/common/db#password",
		},
		queries,
	)
}
