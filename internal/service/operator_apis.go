// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/honeydipper/honeydipper/v4/internal/api"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const defaultSecretDriver = "vault"

var ErrInvalidGHSlug = errors.New("invalid gh slug")
var ErrMissingSecretKey = errors.New("missing secret key")
var ErrMissingSecretValue = errors.New("missing secret value")

func setupOperatorAPIs() {
	operator.APIs["ghSecretList"] = handleGHSecretList
	operator.APIs["ghSecretSet"] = handleGHSecretSet
	operator.APIs["ghSecretDelete"] = handleGHSecretDelete
}

func isSecretNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(strings.ToLower(err.Error()), "secret not found")
}

func isSecretKeyNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(strings.ToLower(err.Error()), "secret key not found")
}

func getSecretDriver() string {
	if driverName := os.Getenv("HD_SECRET_DRIVER"); driverName != "" {
		return driverName
	}

	return defaultSecretDriver
}

func getSecretDriverFeature() string {
	return "driver:" + getSecretDriver()
}

func buildGHSecretsPath(ghSlug string) (string, error) {
	normalized := strings.Trim(strings.TrimSpace(ghSlug), "/")
	if normalized == "" {
		return "", ErrInvalidGHSlug
	}

	parts := strings.Split(normalized, "/")
	if len(parts) == 1 {
		return "secrets/data/hdci/gh/" + parts[0] + "/org", nil
	}
	if len(parts) == 2 {
		return "secrets/data/hdci/gh/" + parts[0] + "/" + parts[1], nil
	}

	return "", ErrInvalidGHSlug
}

func getStringFromPayload(payload map[string]interface{}, key string) string {
	if raw, found := payload[key]; found {
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s)
		}
	}

	bodyRaw, hasBody := payload["body"]
	if !hasBody {
		return ""
	}
	body, ok := bodyRaw.(string)
	if !ok || strings.TrimSpace(body) == "" {
		return ""
	}

	parsed := map[string]interface{}{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return ""
	}

	if raw, found := parsed[key]; found {
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s)
		}
	}

	return ""
}

func extractSecretKeyValue(payload map[string]interface{}) (string, string, error) {
	key := getStringFromPayload(payload, "key")
	if key == "" {
		return "", "", ErrMissingSecretKey
	}

	value := getStringFromPayload(payload, "value")
	if value == "" {
		return "", "", ErrMissingSecretValue
	}

	return key, value, nil
}

func extractSecretKey(payload map[string]interface{}) (string, error) {
	key := getStringFromPayload(payload, "key")
	if key == "" {
		return "", ErrMissingSecretKey
	}

	return key, nil
}

func handleGHSecretList(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	ghSlug := dipper.MustGetMapDataStr(resp.Request.Payload, "gh_slug")

	secretPath, err := buildGHSecretsPath(ghSlug)
	if err != nil {
		panic(err)
	}

	raw, err := operator.CallRaw(getSecretDriverFeature(), "list_keys", []byte(secretPath))
	if err != nil {
		if isSecretNotFoundError(err) {
			resp.Return(map[string]interface{}{"keys": []string{}})

			return
		}

		panic(fmt.Errorf("ghSecretList failed: %w", err))
	}

	keys := []string{}
	if len(raw) > 0 {
		dipper.Must(json.Unmarshal(raw, &keys))
	}

	resp.Return(map[string]interface{}{
		"keys": keys,
	})
}

func handleGHSecretSet(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	payload := resp.Request.Payload.(map[string]interface{})
	ghSlug := dipper.MustGetMapDataStr(payload, "gh_slug")

	secretPath, err := buildGHSecretsPath(ghSlug)
	if err != nil {
		panic(err)
	}

	key, value, err := extractSecretKeyValue(payload)
	if err != nil {
		panic(err)
	}

	_, err = operator.Call(getSecretDriverFeature(), "set", map[string]interface{}{
		"path":  secretPath,
		"key":   key,
		"value": value,
	})
	if err != nil {
		panic(fmt.Errorf("ghSecretSet failed: %w", err))
	}

	resp.Return(map[string]interface{}{
		"key": key,
	})
}

func handleGHSecretDelete(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	payload := resp.Request.Payload.(map[string]interface{})
	ghSlug := dipper.MustGetMapDataStr(payload, "gh_slug")

	secretPath, err := buildGHSecretsPath(ghSlug)
	if err != nil {
		panic(err)
	}

	key, err := extractSecretKey(payload)
	if err != nil {
		panic(err)
	}

	_, err = operator.Call(getSecretDriverFeature(), "delete_key", map[string]interface{}{
		"path": secretPath,
		"key":  key,
	})
	if err != nil {
		if isSecretNotFoundError(err) || isSecretKeyNotFoundError(err) {
			resp.Return(map[string]interface{}{"key": key, "deleted": false})

			return
		}

		panic(fmt.Errorf("ghSecretDelete failed: %w", err))
	}

	resp.Return(map[string]interface{}{
		"key":     key,
		"deleted": true,
	})
}
