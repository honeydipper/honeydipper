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

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
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

func TestGetPodIDSigningSecrets(t *testing.T) {
	originalOperator := operator
	t.Cleanup(func() {
		operator = originalOperator
	})

	operator = &Service{config: &config.Config{}}
	operator.config.DataSet = &config.DataSet{
		Drivers: map[string]interface{}{
			"daemon": map[string]interface{}{
				"workflow": map[string]interface{}{
					"pod_id_signing_secret":          "current",
					"pod_id_signing_secret_previous": "previous",
				},
			},
		},
	}

	current, previous := getPodIDSigningSecrets()
	assert.Equal(t, "current", current)
	assert.Equal(t, "previous", previous)
}

func TestGetPodIDSigningSecretsWithPrevAlias(t *testing.T) {
	originalOperator := operator
	t.Cleanup(func() {
		operator = originalOperator
	})

	operator = &Service{config: &config.Config{}}
	operator.config.DataSet = &config.DataSet{
		Drivers: map[string]interface{}{
			"daemon": map[string]interface{}{
				"workflow": map[string]interface{}{
					"pod_id_signing_secret":      "current",
					"pod_id_signing_secret_prev": "legacy-prev",
				},
			},
		},
	}

	current, previous := getPodIDSigningSecrets()
	assert.Equal(t, "current", current)
	assert.Equal(t, "legacy-prev", previous)
}

func TestPodIDSignatureVerificationWithDualSecrets(t *testing.T) {
	payload := dipper.PodIDSignaturePayload("pod-1", "org/repo")
	oldSignature := dipper.SignPayload("old-secret", payload)

	assert.True(t, dipper.VerifyPayloadWithSecrets(payload, oldSignature, "new-secret", "old-secret"))
	assert.False(t, dipper.VerifyPayloadWithSecrets(payload, oldSignature, "new-secret"))
}

func TestGetLogProviderMapsKubernetes(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]interface{}
		expected string
	}{
		{
			name:     "k8s maps to kubernetes",
			payload:  map[string]interface{}{"provider": "k8s"},
			expected: "kubernetes",
		},
		{
			name:     "kubernetes maps to kubernetes",
			payload:  map[string]interface{}{"provider": "kubernetes"},
			expected: "kubernetes",
		},
		{
			name:     "podman maps to podman",
			payload:  map[string]interface{}{"provider": "podman"},
			expected: "podman",
		},
		{
			name:     "container maps to podman",
			payload:  map[string]interface{}{"provider": "container"},
			expected: "podman",
		},
		{
			name:     "unknown defaults to podman",
			payload:  map[string]interface{}{"provider": "unknown"},
			expected: "podman",
		},
		{
			name:     "empty provider defaults to podman",
			payload:  map[string]interface{}{"provider": ""},
			expected: "podman",
		},
		{
			name:     "no provider uses runtime field",
			payload:  map[string]interface{}{"runtime": "k8s"},
			expected: "kubernetes",
		},
		{
			name:     "no provider no runtime defaults to podman",
			payload:  map[string]interface{}{},
			expected: "podman",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getLogProvider(tc.payload)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetCursorPayloadExtractsData(t *testing.T) {
	tests := []struct {
		name      string
		payload   map[string]interface{}
		expected  interface{}
		expectNil bool
	}{
		{
			name: "valid cursor payload with nested map",
			payload: map[string]interface{}{
				"cursor": map[string]interface{}{
					"pod1": map[string]interface{}{
						"container1": map[string]interface{}{
							"timestamp": "2024-01-01T00:00:00Z",
							"skip":      5,
						},
					},
				},
			},
			expected: map[string]interface{}{
				"pod1": map[string]interface{}{
					"container1": map[string]interface{}{
						"timestamp": "2024-01-01T00:00:00Z",
						"skip":      5,
					},
				},
			},
			expectNil: false,
		},
		{
			name:      "nil cursor returns nil",
			payload:   map[string]interface{}{},
			expectNil: true,
		},
		{
			name: "cursor string with valid JSON is parsed",
			payload: map[string]interface{}{
				"cursor": `{"pod1":{"container1":{"timestamp":"2024-01-01T00:00:00Z","skip":5}}}`,
			},
			expected: map[string]interface{}{
				"pod1": map[string]interface{}{
					"container1": map[string]interface{}{
						"timestamp": "2024-01-01T00:00:00Z",
						"skip":      float64(5),
					},
				},
			},
			expectNil: false,
		},
		{
			name: "cursor string with invalid JSON returns nil",
			payload: map[string]interface{}{
				"cursor": "not-valid-json",
			},
			expectNil: true,
		},
		{
			name: "empty string cursor returns nil",
			payload: map[string]interface{}{
				"cursor": "   ",
			},
			expectNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getCursorPayload(tc.payload)
			if tc.expectNil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}
