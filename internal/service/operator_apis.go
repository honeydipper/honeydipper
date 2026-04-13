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
	"strconv"
	"strings"

	"github.com/honeydipper/honeydipper/v4/internal/api"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const defaultSecretDriver = "vault"

var (
	ErrInvalidGHSlug            = errors.New("invalid gh slug")
	ErrMissingSecretKey         = errors.New("missing secret key")
	ErrMissingSecretValue       = errors.New("missing secret value")
	ErrInvalidPodLogToken       = errors.New("invalid pod log stream token")
	ErrPodLogChunkPodIDRequired = errors.New("podLogChunk failed: pod_id is required")
)

func setupOperatorAPIs() {
	operator.APIs["ghSecretList"] = handleGHSecretList
	operator.APIs["ghSecretSet"] = handleGHSecretSet
	operator.APIs["ghSecretDelete"] = handleGHSecretDelete
	operator.APIs["podLogChunk"] = handlePodLogChunk
	operator.APIs["ghPodLogChunk"] = handlePodLogChunk
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
		return "secrets/data/hdci/gh/" + parts[0] + "/_org", nil
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

func getIntFromPayload(payload map[string]interface{}, key string, fallback int) int {
	if raw, found := payload[key]; found {
		switch t := raw.(type) {
		case int:
			return t
		case int32:
			return int(t)
		case int64:
			return int(t)
		case float64:
			return int(t)
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
				return n
			}
		}
	}

	return fallback
}

func getLogProvider(payload map[string]interface{}) string {
	provider := strings.ToLower(strings.TrimSpace(getStringFromPayload(payload, "provider")))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(getStringFromPayload(payload, "runtime")))
	}

	switch provider {
	case "", "podman", "container", "containers":
		return "podman"
	case "k8s":
		return "kubernetes"
	case "kubernetes":
		return "kubernetes"
	default:
		return "podman"
	}
}

func getCursorPayload(payload map[string]interface{}) interface{} {
	raw, ok := payload["cursor"]
	if !ok || raw == nil {
		return nil
	}

	if asStr, isString := raw.(string); isString {
		asStr = strings.TrimSpace(asStr)
		if asStr == "" {
			return nil
		}

		var parsed interface{}
		if err := json.Unmarshal([]byte(asStr), &parsed); err == nil {
			return parsed
		}

		return nil
	}

	return raw
}

func getPodIDSigningSecrets() (string, string) {
	if operator == nil || operator.config == nil || operator.config.DataSet == nil || operator.config.DataSet.Drivers == nil {
		return "", ""
	}

	current, _ := dipper.GetMapDataStr(operator.config.DataSet.Drivers, "daemon.workflow.pod_id_signing_secret")
	previous, _ := dipper.GetMapDataStr(operator.config.DataSet.Drivers, "daemon.workflow.pod_id_signing_secret_previous")
	if strings.TrimSpace(previous) == "" {
		previous, _ = dipper.GetMapDataStr(operator.config.DataSet.Drivers, "daemon.workflow.pod_id_signing_secret_prev")
	}

	return strings.TrimSpace(current), strings.TrimSpace(previous)
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

func handlePodLogChunk(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	payload := resp.Request.Payload.(map[string]interface{})
	podID := getStringFromPayload(payload, "pod_id")
	if podID == "" {
		panic(ErrPodLogChunkPodIDRequired)
	}
	ghSlug := strings.Trim(strings.TrimSpace(getStringFromPayload(payload, "gh_slug")), "/")
	if ghSlug != "" {
		token := getStringFromPayload(payload, "stream_token")
		currentSecret, previousSecret := getPodIDSigningSecrets()
		sigPayload := dipper.PodIDSignaturePayload(podID, ghSlug)
		if !dipper.VerifyPayloadWithSecrets(sigPayload, token, currentSecret, previousSecret) {
			panic(fmt.Errorf("podLogChunk failed: %w", ErrInvalidPodLogToken))
		}
	}

	provider := getLogProvider(payload)
	rpcPayload := map[string]interface{}{
		"pod_id":         podID,
		"wait_seconds":   getIntFromPayload(payload, "wait_seconds", 3),
		"max_lines":      getIntFromPayload(payload, "max_lines", 200),
		"done_max_lines": getIntFromPayload(payload, "done_max_lines", 5000),
	}
	if cursor := getCursorPayload(payload); cursor != nil {
		rpcPayload["cursor"] = cursor
	}
	if include, ok := payload["include_containers"]; ok {
		rpcPayload["include_containers"] = include
	}

	raw, err := operator.Call("driver:"+provider, "get_pod_log_tail", rpcPayload)
	if err != nil {
		panic(fmt.Errorf("podLogChunk failed via %s: %w", provider, err))
	}

	resp.Return(raw)
}
