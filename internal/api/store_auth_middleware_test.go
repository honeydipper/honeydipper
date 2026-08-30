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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/v4/internal/api/mock_api"
	"github.com/honeydipper/honeydipper/v4/internal/api/session"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper/mock_dipper"
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

func TestGitHubOAuthCallbackRegistersEagerSessionFromUsername(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCaller := mock_dipper.NewMockRPCCaller(ctrl)
	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockReqCtx.EXPECT().GetParam("code").Return("auth-code")
	mockCaller.EXPECT().Call(
		"driver:auth-github",
		"github_oauth_callback",
		gomock.Any(),
	).Return([]byte(`{"username":"alice","email":"alice@example.com"}`), nil)

	// Build an enabled session engine whose manager wraps a plain MemoryStore so
	// the test can inspect the eagerly-registered session directly.
	memStore := session.NewMemoryStore()
	engine := &sessionEngine{
		enabled:     true,
		cacheTTL:    time.Minute,
		manager:     session.NewManager(memStore, session.Policy{}, 0),
		idleTimeout: 0,
		maxLifetime: 0,
	}
	store := NewStore(mockCaller)
	store.sessionEngine = engine

	req := &Request{store: store, ctx: mockReqCtx}
	if _, err := githubOAuthCallbackHandler(req); err != nil {
		t.Fatalf("githubOAuthCallbackHandler: %v", err)
	}

	// The eager registration must have created a session for subject "alice"
	// (read from "username", since the driver reply carries no "subject" key).
	snapshot := memStore.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("expected one eagerly registered session, got %d", len(snapshot))
	}
	for _, rec := range snapshot {
		if rec.Subject != "alice" {
			t.Fatalf("expected subject alice from username fallback, got %+v", rec)
		}
		if rec.SID == "" {
			t.Fatalf("expected a daemon-minted sid to be registered")
		}
	}
}
