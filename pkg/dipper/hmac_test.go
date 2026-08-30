// Copyright 2026 PayPal Inc.

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

func TestSignAndVerifyPayload(t *testing.T) {
	secret := "current-secret"
	payload := PodIDSignaturePayload("pod-123", "org/repo")
	signature := SignPayload(secret, payload)

	assert.NotEmpty(t, signature)
	assert.True(t, VerifyPayload(secret, payload, signature))
	assert.False(t, VerifyPayload(secret, PodIDSignaturePayload("pod-456", "org/repo"), signature))
}

func TestVerifyPayloadWithSecrets(t *testing.T) {
	payload := PodIDSignaturePayload("pod-123", "org/repo")
	oldSignature := SignPayload("old-secret", payload)

	assert.True(t, VerifyPayloadWithSecrets(payload, oldSignature, "current-secret", "old-secret"))
	assert.False(t, VerifyPayloadWithSecrets(payload, oldSignature, "current-secret"))
}

func TestFirstNonEmptyString(t *testing.T) {
	assert.Equal(t, "org/repo", FirstNonEmptyString("", "<no value>", " org/repo "))
	assert.Equal(t, "", FirstNonEmptyString("", "<nil>", nil))
}

func TestPodIDSignaturePayload(t *testing.T) {
	assert.Equal(t, "pod_id=pod-123&gh_slug=org/repo", PodIDSignaturePayload(" pod-123 ", "/org/repo/"))
}
