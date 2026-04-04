// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildGHSecretsPath(t *testing.T) {
	tests := []struct {
		name   string
		slug   string
		path   string
		hasErr bool
	}{
		{name: "org only", slug: "paypal", path: "secrets/data/hdci/gh/paypal/_org"},
		{name: "repo slug", slug: "paypal/honeydipper", path: "secrets/data/hdci/gh/paypal/honeydipper"},
		{name: "repo slug with wrapped slashes", slug: "/paypal/honeydipper/", path: "secrets/data/hdci/gh/paypal/honeydipper"},
		{name: "empty slug", slug: "", hasErr: true},
		{name: "invalid deep slug", slug: "a/b/c", hasErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, err := buildGHSecretsPath(tc.slug)
			if tc.hasErr {
				assert.ErrorIs(t, err, ErrInvalidGHSlug)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.path, path)
		})
	}
}

func TestGetSecretDriver(t *testing.T) {
	old, oldExists := os.LookupEnv("HD_SECRET_DRIVER")
	defer func() {
		if oldExists {
			_ = os.Setenv("HD_SECRET_DRIVER", old)
		} else {
			_ = os.Unsetenv("HD_SECRET_DRIVER")
		}
	}()

	_ = os.Unsetenv("HD_SECRET_DRIVER")
	assert.Equal(t, defaultSecretDriver, getSecretDriver())
	assert.Equal(t, "driver:"+defaultSecretDriver, getSecretDriverFeature())

	_ = os.Setenv("HD_SECRET_DRIVER", "custom-secret-driver")
	assert.Equal(t, "custom-secret-driver", getSecretDriver())
	assert.Equal(t, "driver:custom-secret-driver", getSecretDriverFeature())
}

func TestOperatorFeatures(t *testing.T) {
	old, oldExists := os.LookupEnv("HD_SECRET_DRIVER")
	defer func() {
		if oldExists {
			_ = os.Setenv("HD_SECRET_DRIVER", old)
		} else {
			_ = os.Unsetenv("HD_SECRET_DRIVER")
		}
	}()

	_ = os.Setenv("HD_SECRET_DRIVER", "my-secret-driver")
	features := OperatorFeatures(nil)
	_, found := features["driver:my-secret-driver"]
	assert.True(t, found)
}

func TestIsSecretNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "secret not found exact", err: errors.New("secret not found"), want: true},
		{name: "secret not found mixed case", err: errors.New("Secret Not Found at path"), want: true},
		{name: "different error", err: errors.New("permission denied"), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSecretNotFoundError(tc.err))
		})
	}
}

func TestIsSecretKeyNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "secret key not found exact", err: errors.New("secret key not found"), want: true},
		{name: "secret key not found mixed case", err: errors.New("Secret Key Not Found in secret"), want: true},
		{name: "different error", err: errors.New("permission denied"), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSecretKeyNotFoundError(tc.err))
		})
	}
}

func TestGetStringFromPayload(t *testing.T) {
	t.Run("direct field wins", func(t *testing.T) {
		payload := map[string]interface{}{
			"key":  "  API_KEY  ",
			"body": `{"key":"BODY_KEY"}`,
		}
		assert.Equal(t, "API_KEY", getStringFromPayload(payload, "key"))
	})

	t.Run("fallback to json body", func(t *testing.T) {
		payload := map[string]interface{}{
			"body": `{"key":"BODY_KEY","value":"BODY_VALUE"}`,
		}
		assert.Equal(t, "BODY_KEY", getStringFromPayload(payload, "key"))
		assert.Equal(t, "BODY_VALUE", getStringFromPayload(payload, "value"))
	})

	t.Run("invalid body returns empty", func(t *testing.T) {
		payload := map[string]interface{}{"body": `{`}
		assert.Equal(t, "", getStringFromPayload(payload, "key"))
	})
}

func TestExtractSecretKeyValue(t *testing.T) {
	t.Run("valid key and value", func(t *testing.T) {
		k, v, err := extractSecretKeyValue(map[string]interface{}{"key": "k", "value": "v"})
		assert.NoError(t, err)
		assert.Equal(t, "k", k)
		assert.Equal(t, "v", v)
	})

	t.Run("missing key", func(t *testing.T) {
		_, _, err := extractSecretKeyValue(map[string]interface{}{"value": "v"})
		assert.ErrorIs(t, err, ErrMissingSecretKey)
	})

	t.Run("missing value", func(t *testing.T) {
		_, _, err := extractSecretKeyValue(map[string]interface{}{"key": "k"})
		assert.ErrorIs(t, err, ErrMissingSecretValue)
	})
}

func TestExtractSecretKey(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		k, err := extractSecretKey(map[string]interface{}{"key": "k"})
		assert.NoError(t, err)
		assert.Equal(t, "k", k)
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := extractSecretKey(map[string]interface{}{})
		assert.ErrorIs(t, err, ErrMissingSecretKey)
	})
}
