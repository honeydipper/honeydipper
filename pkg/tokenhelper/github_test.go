// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package tokenhelper

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		logFile, err := os.Create(os.DevNull)
		if err != nil {
			panic(err)
		}
		defer logFile.Close()
		dipper.GetLogger("test", "INFO", logFile, logFile)
	}
	os.Exit(m.Run())
}

func generateTestRSAKey() string {
	privateKey := dipper.Must(rsa.GenerateKey(rand.Reader, 2048)).(*rsa.PrivateKey)
	privateKeyBytes := dipper.Must(x509.MarshalPKCS8PrivateKey(privateKey)).([]byte)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return string(privateKeyPEM)
}

func TestGetGitHubToken(t *testing.T) {
	// Setup test RSA key
	var privateKeyPEM string
	assert.NotPanics(t, func() { privateKeyPEM = generateTestRSAKey() })

	// Test successful token retrieval
	t.Run("successful token retrieval", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/app/installations/12345/access_tokens", r.URL.Path)
			assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      "ghu_test_token_123",
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
		}))
		defer server.Close()

		s := map[string]interface{}{
			"app_id":          "123456",
			"key":             privateKeyPEM,
			"permissions":     map[string]interface{}{"contents": "read"},
			"installation_id": "12345",
			"github_url":      server.URL,
		}

		token := GetGitHubToken(s)
		assert.Equal(t, "ghu_test_token_123", token)
		assert.NotNil(t, s["_saved"])
		assert.NotNil(t, s["_expiresAt"])
	})

	// Test token caching - returns saved token if not expired
	t.Run("token caching when not expired", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      "ghu_test_token_first",
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
		}))
		defer server.Close()

		s := map[string]interface{}{
			"app_id":          "123456",
			"key":             privateKeyPEM,
			"permissions":     map[string]interface{}{"contents": "read"},
			"installation_id": "12345",
			"github_url":      server.URL,
		}

		// First call
		token1 := GetGitHubToken(s)
		assert.Equal(t, "ghu_test_token_first", token1)

		// Second call should return cached token
		token2 := GetGitHubToken(s)
		assert.Equal(t, token1, token2)
	})

	// Test token refresh when expired
	t.Run("token refresh when expired", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			tokenValue := "ghu_test_token_first"
			if callCount > 1 {
				tokenValue = "ghu_test_token_refreshed"
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      tokenValue,
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
		}))
		defer server.Close()

		s := map[string]interface{}{
			"app_id":          "123456",
			"key":             privateKeyPEM,
			"permissions":     map[string]interface{}{"contents": "read"},
			"installation_id": "12345",
			"github_url":      server.URL,
		}

		// First call
		token1 := GetGitHubToken(s)
		assert.Equal(t, "ghu_test_token_first", token1)

		// Manually expire the token by setting expiration to past
		s["_expiresAt"] = time.Now().Add(-time.Hour)

		// Second call should fetch a new token
		token2 := GetGitHubToken(s)
		assert.Equal(t, "ghu_test_token_refreshed", token2)
		assert.NotEqual(t, token1, token2)
	})

	// Test using custom github_url
	t.Run("custom github url", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      "ghu_test_token",
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
		}))
		defer server.Close()

		s := map[string]interface{}{
			"app_id":          "123456",
			"key":             privateKeyPEM,
			"permissions":     map[string]interface{}{"contents": "read"},
			"installation_id": "12345",
			"github_url":      server.URL,
		}

		token := GetGitHubToken(s)
		assert.NotEmpty(t, token)
	})

	// Test permissions passed correctly
	t.Run("permissions passed in request", func(t *testing.T) {
		var requestBody map[string]interface{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &requestBody)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      "ghu_test_token",
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
		}))
		defer server.Close()

		permissions := map[string]interface{}{
			"contents": "read",
			"issues":   "write",
		}
		s := map[string]interface{}{
			"app_id":          "123456",
			"key":             privateKeyPEM,
			"permissions":     permissions,
			"installation_id": "12345",
			"github_url":      server.URL,
		}

		GetGitHubToken(s)
		assert.Equal(t, permissions, requestBody["permissions"])
	})

	// Test HTTP error handling
	t.Run("http error handling", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Invalid authentication"))
		}))
		defer server.Close()

		s := map[string]interface{}{
			"app_id":          "123456",
			"key":             privateKeyPEM,
			"permissions":     map[string]interface{}{"contents": "read"},
			"installation_id": "12345",
			"github_url":      server.URL,
		}

		assert.Panics(t, func() { GetGitHubToken(s) })
	})

	// Test JWT contains required claims
	t.Run("jwt contains required claims", func(t *testing.T) {
		var capturedJWT string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			capturedJWT = authHeader[7:] // Remove "Bearer " prefix
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      "ghu_test_token",
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
		}))
		defer server.Close()

		s := map[string]interface{}{
			"app_id":          "123456",
			"key":             privateKeyPEM,
			"permissions":     map[string]interface{}{"contents": "read"},
			"installation_id": "12345",
			"github_url":      server.URL,
		}

		GetGitHubToken(s)

		// Parse and verify JWT
		privateKey, _ := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
		claims := &jwt.RegisteredClaims{}
		jwt.ParseWithClaims(capturedJWT, claims, func(token *jwt.Token) (interface{}, error) {
			return &privateKey.PublicKey, nil
		})

		assert.Equal(t, "123456", claims.Issuer)
		assert.NotNil(t, claims.IssuedAt)
		assert.NotNil(t, claims.ExpiresAt)
	})

	// Test cached key not reparsed
	t.Run("cached private key reuse", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      "ghu_test_token",
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
		}))
		defer server.Close()

		s := map[string]interface{}{
			"app_id":          "123456",
			"key":             privateKeyPEM,
			"permissions":     map[string]interface{}{"contents": "read"},
			"installation_id": "12345",
			"github_url":      server.URL,
		}

		// First call
		GetGitHubToken(s)
		parsedKey1 := s["_parsed_key"]

		// Force token expiration to get a new one
		s["_expiresAt"] = time.Now().Add(-time.Hour)

		// Second call - should reuse cached key
		GetGitHubToken(s)
		parsedKey2 := s["_parsed_key"]

		assert.Equal(t, parsedKey1, parsedKey2)
		assert.Equal(t, 2, callCount)
	})
}

func TestSetupGitHubSourceDefault(t *testing.T) {
	t.Run("fills defaults from env", func(t *testing.T) {
		key := generateTestRSAKey()
		t.Setenv("GH_APP_ID", "999")
		t.Setenv("GH_APP_INSTALLATION_ID", "888")
		t.Setenv("GH_APP_PRIVATE_KEY", key)

		s := map[string]interface{}{}
		SetupGitHubSourceDefault(s)

		assert.Equal(t, "999", s["app_id"])
		assert.Equal(t, "888", s["installation_id"])
		assert.Equal(t, key, s["key"])
		assert.Equal(t, _globalGitHubURL, s["github_url"])
		permissions, ok := s["permissions"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "read", permissions["contents"])
	})

	t.Run("keeps explicit values", func(t *testing.T) {
		s := map[string]interface{}{
			"app_id":          "explicit-app",
			"installation_id": "explicit-install",
			"key":             "explicit-key",
			"github_url":      "https://example.com",
			"permissions":     map[string]interface{}{"contents": "write"},
		}

		SetupGitHubSourceDefault(s)

		assert.Equal(t, "explicit-app", s["app_id"])
		assert.Equal(t, "explicit-install", s["installation_id"])
		assert.Equal(t, "explicit-key", s["key"])
		assert.Equal(t, "https://example.com", s["github_url"])
		permissions := s["permissions"].(map[string]interface{})
		assert.Equal(t, "write", permissions["contents"])
	})

	t.Run("panics when required env is missing", func(t *testing.T) {
		t.Setenv("GH_APP_INSTALLATION_ID", "")
		t.Setenv("GH_APP_ID", "")
		t.Setenv("GH_APP_PRIVATE_KEY", "")

		assert.PanicsWithError(t, fmt.Errorf("%w: installation_id missing", ErrRetrieveToken).Error(), func() {
			SetupGitHubSourceDefault(map[string]interface{}{})
		})
	})
}

func TestGetGitHubTokenConcurrentAccess(t *testing.T) {
	key := generateTestRSAKey()
	t.Setenv("GH_APP_ID", "123456")
	t.Setenv("GH_APP_INSTALLATION_ID", "12345")
	t.Setenv("GH_APP_PRIVATE_KEY", key)

	s := map[string]interface{}{
		"type": "github",
		"mock": map[string]interface{}{
			"token":     "mock-token",
			"expiresAt": time.Now().Add(time.Minute),
		},
	}

	const workers = 32
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := GetGitHubToken(s); got != "mock-token" {
				errCh <- fmt.Errorf("unexpected token: %s", got)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		assert.NoError(t, err)
	}

	assert.Equal(t, "mock-token", s["_saved"])
}
