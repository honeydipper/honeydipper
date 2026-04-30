// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidJWT = errors.New("invalid JWT token")
)

type JWTConfig struct {
	SigningKey []byte
	Issuer     string
	ExpiresIn  time.Duration
}

type PrincipalClaims struct {
	Subject     string                 `json:"sub"`
	ProfileName string                 `json:"profile_name"`
	Provider    string                 `json:"provider"`
	Data        map[string]interface{} `json:"data"`
	jwt.RegisteredClaims
}

func getJWTConfig() (*JWTConfig, error) {
	key := os.Getenv("HD_JWT_SIGNING_KEY")
	if key == "" {
		return nil, fmt.Errorf("HD_JWT_SIGNING_KEY not set")
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

func SignPrincipalJWT(principal Principal, cfg *JWTConfig) (string, error) {
	claims := PrincipalClaims{
		Subject:     principal.Subject,
		ProfileName: principal.ProfileName,
		Provider:    principal.Provider,
		Data:        principal.Data,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.ExpiresIn)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(cfg.SigningKey)
}

func ParsePrincipalJWT(tokenString string, cfg *JWTConfig) (*Principal, error) {
	parsed, err := jwt.ParseWithClaims(tokenString, &PrincipalClaims{}, func(token *jwt.Token) (interface{}, error) {
		return cfg.SigningKey, nil
	})
	if err != nil {
		return nil, ErrInvalidJWT
	}
	claims, ok := parsed.Claims.(*PrincipalClaims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidJWT
	}
	return &Principal{
		Subject:     claims.Subject,
		ProfileName: claims.ProfileName,
		Provider:    claims.Provider,
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
