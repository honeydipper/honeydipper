// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// SignPayload signs a payload with a secret using HMAC-SHA256.
func SignPayload(secret, payload string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))

	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyPayload verifies a payload signature with a secret.
func VerifyPayload(secret, payload, signature string) bool {
	expected := SignPayload(secret, payload)
	if expected == "" {
		return false
	}

	normalizedSig := strings.ToLower(strings.TrimSpace(signature))
	if normalizedSig == "" || len(normalizedSig) != len(expected) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(expected), []byte(normalizedSig)) == 1
}

// VerifyPayloadWithSecrets verifies a payload signature against one or more candidate secrets.
func VerifyPayloadWithSecrets(payload, signature string, secrets ...string) bool {
	for _, secret := range secrets {
		if VerifyPayload(secret, payload, signature) {
			return true
		}
	}

	return false
}

// FirstNonEmptyString returns the first non-empty string from a list of candidate values.
func FirstNonEmptyString(values ...interface{}) string {
	for _, raw := range values {
		s := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if s == "" || s == "<nil>" || s == "<no value>" {
			continue
		}

		return s
	}

	return ""
}

// PodIDSignaturePayload builds the canonical payload used for pod ID stream signatures.
func PodIDSignaturePayload(podID, ghSlug string) string {
	normalizedPodID := strings.TrimSpace(podID)
	normalizedGHSlug := strings.Trim(strings.TrimSpace(ghSlug), "/")

	return fmt.Sprintf("pod_id=%s&gh_slug=%s", normalizedPodID, normalizedGHSlug)
}
