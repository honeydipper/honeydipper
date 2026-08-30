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

// parseExpFromPrincipalJWT parses a daemon JWT and returns its exp timestamp.
func parseExpFromPrincipalJWT(t *testing.T, token string, cfg *JWTConfig) time.Time {
	t.Helper()
	claims := &PrincipalClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidJWT
		}

		return cfg.SigningKey, nil
	}, jwt.WithIssuer(cfg.Issuer))
	if err != nil {
		t.Fatalf("ParseWithClaims: %v", err)
	}
	if !parsed.Valid || claims.ExpiresAt == nil {
		t.Fatalf("expected valid token with exp, got %+v", claims)
	}

	return claims.ExpiresAt.Time
}

func TestSignPrincipalJWTSessionBindsExpToMaxLifetime(t *testing.T) {
	cfg := &JWTConfig{SigningKey: testJWTCfg.SigningKey, Issuer: "honeydipper", ExpiresIn: 24 * time.Hour}
	principal := Principal{Subject: "alice", Provider: "auth-github", SID: "sid-1", IssuedAt: time.Now().Unix()}

	// Strong cap: session expires well before the default 24h JWT exp.
	token, err := SignPrincipalJWTSession(principal, cfg, principal.IssuedAt, time.Hour)
	if err != nil {
		t.Fatalf("SignPrincipalJWTSession: %v", err)
	}
	expTime := parseExpFromPrincipalJWT(t, token, cfg)
	now := time.Now()
	if expTime.Before(now) || expTime.After(now.Add(90*time.Minute)) {
		t.Fatalf("expected session-bound exp near now+1h, got %v", expTime)
	}
}

func TestSignPrincipalJWTSessionFallsBackToExpiresIn(t *testing.T) {
	cfg := &JWTConfig{SigningKey: testJWTCfg.SigningKey, Issuer: "honeydipper", ExpiresIn: time.Hour}
	principal := Principal{Subject: "alice", Provider: "auth-github", SID: "sid-1", IssuedAt: time.Now().Unix()}

	// No maxLifetime configured -> default ExpiresIn applies unchanged.
	token, err := SignPrincipalJWTSession(principal, cfg, principal.IssuedAt, 0)
	if err != nil {
		t.Fatalf("SignPrincipalJWTSession: %v", err)
	}
	expTime := parseExpFromPrincipalJWT(t, token, cfg)
	now := time.Now()
	if expTime.Before(now.Add(50*time.Minute)) || expTime.After(now.Add(70*time.Minute)) {
		t.Fatalf("expected default 1h exp, got %v", expTime)
	}
}
