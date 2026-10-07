// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidJWT           = errors.New("invalid JWT token")
	ErrMissingJWTSigningKey = errors.New("missing JWT signing key in config")
)

// SIDClaim is the JWT claim name carrying the daemon-minted opaque session
// identifier (sid). The sid is stable across token rotation and is the primary
// key of the server-side session store.
const SIDClaim = "sid"

type JWTConfig struct {
	SigningKey []byte
	Issuer     string
	ExpiresIn  time.Duration
}

type PrincipalClaims struct {
	Subject     string                 `json:"sub"`
	ProfileName string                 `json:"profile_name"`
	Provider    string                 `json:"provider"`
	SID         string                 `json:"sid,omitempty"`
	Data        map[string]interface{} `json:"data"`
	jwt.RegisteredClaims
}

func getJWTConfig() (*JWTConfig, error) {
	key := os.Getenv("HD_JWT_SIGNING_KEY")
	if key == "" {
		return nil, ErrMissingJWTSigningKey
	}
	issuer := os.Getenv("HD_JWT_ISSUER")
	if issuer == "" {
		issuer = "honeydipper"
	}

	return &JWTConfig{
		SigningKey: []byte(key),
		Issuer:     issuer,
		ExpiresIn:  24 * time.Hour,
	}, nil
}

// SignPrincipalJWT signs a daemon JWT for the given principal. The principal's
// SID (if set) is embedded in the token and preserved verbatim so rotation
// never regenerates the session identity. The token exp is the configured
// ExpiresIn unless a session cap is applied via SignPrincipalJWTSession.
func SignPrincipalJWT(principal Principal, cfg *JWTConfig) (string, error) {
	return signPrincipalJWTWithExp(principal, cfg, time.Now().Add(cfg.ExpiresIn))
}

// SignPrincipalJWTSession signs a daemon JWT whose expiry is bound to the
// session's absolute cap when one is configured and the session issuance time
// is known. This keeps the JWT exp from disagreeing with the sid-based
// maxLifetime cap (the token does not outlive the session). When maxLifetime
// is <= 0 or issuedAt is unknown, the configured ExpiresIn is used unchanged.
func SignPrincipalJWTSession(principal Principal, cfg *JWTConfig, issuedAt int64, maxLifetime time.Duration) (string, error) {
	exp := time.Now().Add(cfg.ExpiresIn)
	if maxLifetime > 0 && issuedAt > 0 {
		sessionExp := time.Unix(issuedAt, 0).Add(maxLifetime)
		if sessionExp.Before(exp) {
			exp = sessionExp
		}
	}

	return signPrincipalJWTWithExp(principal, cfg, exp)
}

func signPrincipalJWTWithExp(principal Principal, cfg *JWTConfig, exp time.Time) (string, error) {
	claims := PrincipalClaims{
		Subject:     principal.Subject,
		ProfileName: principal.ProfileName,
		Provider:    principal.Provider,
		SID:         principal.SID,
		Data:        principal.Data,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	//nolint:wrapcheck
	return token.SignedString(cfg.SigningKey)
}

func ParsePrincipalJWT(tokenString string, cfg *JWTConfig) (*Principal, error) {
	parsed, err := jwt.ParseWithClaims(tokenString, &PrincipalClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidJWT
		}

		return cfg.SigningKey, nil
	}, jwt.WithIssuer(cfg.Issuer))
	if err != nil {
		return nil, ErrInvalidJWT
	}
	claims, ok := parsed.Claims.(*PrincipalClaims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidJWT
	}

	issuedAt := int64(0)
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.Unix()
	}

	return &Principal{
		Subject:     claims.Subject,
		ProfileName: claims.ProfileName,
		Provider:    claims.Provider,
		SID:         claims.SID,
		IssuedAt:    issuedAt,
		Data:        claims.Data,
	}, nil
}

func ExtractJWTFromRequest(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}

	return ""
}
