// Copyright 2026 Chun Huang (Charles).

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestApplyRotatedJWTHeaderSetsHeader(t *testing.T) {
	store := &Store{}
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	store.applyRotatedJWTHeader(ctx, Principal{
		Data: map[string]interface{}{
			"rotatedJwt": "new.jwt.token",
		},
	})

	if got := w.Header().Get(RefreshedJWTHeader); got != "new.jwt.token" {
		t.Fatalf("expected refreshed JWT header, got %q", got)
	}

	if got := w.Header().Get("Access-Control-Expose-Headers"); got != RefreshedJWTHeader {
		t.Fatalf("expected exposed headers to include refreshed JWT header, got %q", got)
	}
}

func TestApplyRotatedJWTHeaderAppendsExposeHeadersWithoutDup(t *testing.T) {
	store := &Store{}
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Writer.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")

	store.applyRotatedJWTHeader(ctx, Principal{
		Data: map[string]interface{}{
			"rotatedJwt": "new.jwt.token",
		},
	})

	exposed := w.Header().Get("Access-Control-Expose-Headers")
	if exposed != "X-Request-ID, "+RefreshedJWTHeader {
		t.Fatalf("unexpected exposed headers value: %q", exposed)
	}

	store.applyRotatedJWTHeader(ctx, Principal{
		Data: map[string]interface{}{
			"rotatedJwt": "newer.jwt.token",
		},
	})

	exposed = w.Header().Get("Access-Control-Expose-Headers")
	if exposed != "X-Request-ID, "+RefreshedJWTHeader {
		t.Fatalf("expected no duplicate exposed header entries, got %q", exposed)
	}
}

func TestApplyRotatedJWTHeaderNoDataNoHeader(t *testing.T) {
	store := &Store{}
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	store.applyRotatedJWTHeader(ctx, Principal{Data: map[string]interface{}{"foo": "bar"}})

	if got := w.Header().Get(RefreshedJWTHeader); got != "" {
		t.Fatalf("expected empty refreshed JWT header, got %q", got)
	}
}
