// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package api

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var testJWTCfg = &JWTConfig{
	SigningKey: []byte("test-secret"),
	Issuer:     "honeydipper",
	ExpiresIn:  time.Hour,
}

func TestParsePrincipalJWT_ValidToken(t *testing.T) {
	principal := Principal{Subject: "alice", ProfileName: "Alice", Provider: "test"}
	token, err := SignPrincipalJWT(principal, testJWTCfg)
	if err != nil {
		t.Fatalf("SignPrincipalJWT: %v", err)
	}

	got, err := ParsePrincipalJWT(token, testJWTCfg)
	if err != nil {
		t.Fatalf("ParsePrincipalJWT: %v", err)
	}
	if got.Subject != principal.Subject || got.Provider != principal.Provider {
		t.Fatalf("principal mismatch: got %+v", got)
	}
}

func TestParsePrincipalJWT_WrongIssuerRejected(t *testing.T) {
	// Sign with a different issuer than what the middleware expects.
	wrongCfg := &JWTConfig{SigningKey: testJWTCfg.SigningKey, Issuer: "other-system", ExpiresIn: time.Hour}
	principal := Principal{Subject: "alice", Provider: "test"}
	token, err := SignPrincipalJWT(principal, wrongCfg)
	if err != nil {
		t.Fatalf("SignPrincipalJWT: %v", err)
	}

	if _, err := ParsePrincipalJWT(token, testJWTCfg); err == nil {
		t.Fatal("expected error for wrong issuer, got nil")
	}
}

func TestParsePrincipalJWT_WrongSigningMethodRejected(t *testing.T) {
	// Build a token signed with RS256 (asymmetric) — should be rejected.
	claims := PrincipalClaims{
		Subject:  "alice",
		Provider: "test",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testJWTCfg.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	// Use HMAC with a *different* key to simulate a foreign HMAC token.
	foreignCfg := &JWTConfig{SigningKey: []byte("attacker-key"), Issuer: testJWTCfg.Issuer, ExpiresIn: time.Hour}
	token, err := SignPrincipalJWT(Principal{Subject: "alice", Provider: "test"}, foreignCfg)
	if err != nil {
		t.Fatalf("SignPrincipalJWT: %v", err)
	}
	_ = claims

	if _, err := ParsePrincipalJWT(token, testJWTCfg); err == nil {
		t.Fatal("expected error for wrong signing key, got nil")
	}
}

func TestParsePrincipalJWT_ExpiredTokenRejected(t *testing.T) {
	expiredCfg := &JWTConfig{SigningKey: testJWTCfg.SigningKey, Issuer: testJWTCfg.Issuer, ExpiresIn: -time.Hour}
	token, err := SignPrincipalJWT(Principal{Subject: "alice", Provider: "test"}, expiredCfg)
	if err != nil {
		t.Fatalf("SignPrincipalJWT: %v", err)
	}

	if _, err := ParsePrincipalJWT(token, testJWTCfg); err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestParsePrincipalJWT_NonHDJWTRejected(t *testing.T) {
	// A SAML session-style JWT: signed with a different key AND different issuer.
	samlCfg := &JWTConfig{SigningKey: []byte("saml-driver-secret"), Issuer: "auth-saml", ExpiresIn: time.Hour}
	token, err := SignPrincipalJWT(Principal{Subject: "alice", Provider: "auth-saml"}, samlCfg)
	if err != nil {
		t.Fatalf("SignPrincipalJWT: %v", err)
	}

	if _, err := ParsePrincipalJWT(token, testJWTCfg); err == nil {
		t.Fatal("expected SAML session JWT to be rejected by middleware, got nil error")
	}
}

func TestSignPrincipalJWTEmbedsAndPreservesSID(t *testing.T) {
	principal := Principal{Subject: "alice", ProfileName: "Alice", Provider: "test", SID: "stable-sid-123"}
	token, err := SignPrincipalJWT(principal, testJWTCfg)
	if err != nil {
		t.Fatalf("SignPrincipalJWT: %v", err)
	}

	got, err := ParsePrincipalJWT(token, testJWTCfg)
	if err != nil {
		t.Fatalf("ParsePrincipalJWT: %v", err)
	}
	if got.SID != "stable-sid-123" {
		t.Fatalf("expected sid embedded and preserved, got %q", got.SID)
	}
}

func TestSignPrincipalJWTNoSIDForOlderClients(t *testing.T) {
	// A principal without a sid produces a token without a sid claim, matching
	// the pre-upgrade (sid-less) token path.
	principal := Principal{Subject: "bob", Provider: "test"}
	token, err := SignPrincipalJWT(principal, testJWTCfg)
	if err != nil {
		t.Fatalf("SignPrincipalJWT: %v", err)
	}

	got, err := ParsePrincipalJWT(token, testJWTCfg)
	if err != nil {
		t.Fatalf("ParsePrincipalJWT: %v", err)
	}
	if got.SID != "" {
		t.Fatalf("expected empty sid for sid-less principal, got %q", got.SID)
	}
}
