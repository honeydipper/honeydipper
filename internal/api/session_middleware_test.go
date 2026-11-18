// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/honeydipper/honeydipper/v4/internal/api/session"
)

// newTestCtx builds a gin context with a recorder for middleware tests.
func newTestCtx() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	return ctx, w
}

func TestEnforceSession_IAPSkipsSessionStore(t *testing.T) {
	store := &Store{sessionEngine: newTestSessionEngine(session.Policy{MaxLifetime: time.Minute}, 0)}
	ctx, _ := newTestCtx()

	principal := Principal{Subject: "iap-user", Provider: "auth-gcp-iap", SID: ""}
	proceed, eff := store.enforceSession(ctx, principal, store.sessionEngine, false)
	if !proceed {
		t.Fatalf("IAP should always proceed, got false")
	}
	if eff.Subject != "iap-user" {
		t.Fatalf("unexpected principal: %+v", eff)
	}
}

func TestEnforceSession_GraceAcceptsPreUpgradeToken(t *testing.T) {
	store := &Store{sessionEngine: newTestSessionEngine(session.Policy{}, time.Hour)}
	ctx, _ := newTestCtx()

	principal := Principal{Subject: "alice", Provider: "auth-github", SID: ""}
	proceed, _ := store.enforceSession(ctx, principal, store.sessionEngine, false)
	if !proceed {
		t.Fatalf("pre-upgrade token should be accepted during grace")
	}
}

func TestEnforceSession_NoGraceRejectsPreUpgradeToken(t *testing.T) {
	store := &Store{sessionEngine: newTestSessionEngine(session.Policy{}, 0)}
	ctx, w := newTestCtx()

	principal := Principal{Subject: "alice", Provider: "auth-github", SID: ""}
	proceed, _ := store.enforceSession(ctx, principal, store.sessionEngine, false)
	if proceed {
		t.Fatalf("pre-upgrade token should be rejected after grace")
	}
	if w.Header().Get(SessionExpiredHeader) != "true" {
		t.Fatalf("expected session-expired header, got %q", w.Header().Get(SessionExpiredHeader))
	}
	if !strings.Contains(w.Header().Get("Access-Control-Expose-Headers"), SessionExpiredHeader) {
		t.Fatalf("expected session-expired header exposed, got %q", w.Header().Get("Access-Control-Expose-Headers"))
	}
}

func TestEnforceSession_IdleNotExceededProceeds(t *testing.T) {
	engine := newTestSessionEngine(session.Policy{IdleTimeout: time.Hour}, 0)
	now := time.Now().Unix()
	registerTestSession(engine, "sid-1", "alice", "auth-github", now, now)
	store := &Store{sessionEngine: engine}
	ctx, _ := newTestCtx()

	principal := Principal{Subject: "alice", Provider: "auth-github", SID: "sid-1"}
	proceed, _ := store.enforceSession(ctx, principal, store.sessionEngine, false)
	if !proceed {
		t.Fatalf("expected active session to proceed")
	}
}

func TestEnforceSession_IdleExceeded_NotVouched_Reauth(t *testing.T) {
	engine := newTestSessionEngine(session.Policy{IdleTimeout: time.Hour}, 0)
	now := time.Now().Unix()
	registerTestSession(engine, "sid-1", "alice", "auth-github", now, now-2*3600)
	store := &Store{sessionEngine: engine}
	ctx, w := newTestCtx()

	principal := Principal{Subject: "alice", Provider: "auth-github", SID: "sid-1"}
	proceed, _ := store.enforceSession(ctx, principal, store.sessionEngine, false)
	if proceed {
		t.Fatalf("expected idle daemon JWT to require re-auth")
	}
	if w.Header().Get(SessionExpiredHeader) != "true" {
		t.Fatalf("expected session-expired header")
	}
	if w.Header().Get("X-Honeydipper-Reauth") != "new-session" {
		t.Fatalf("expected new-session reauth marker")
	}
}

func TestEnforceSession_MaxLifetimeExceeded_HardReauth(t *testing.T) {
	engine := newTestSessionEngine(session.Policy{MaxLifetime: time.Hour}, 0)
	now := time.Now().Unix()
	registerTestSession(engine, "sid-1", "alice", "auth-github", now-2*3600, now)
	store := &Store{sessionEngine: engine}
	ctx, _ := newTestCtx()

	principal := Principal{Subject: "alice", Provider: "auth-github", SID: "sid-1"}
	proceed, _ := store.enforceSession(ctx, principal, store.sessionEngine, true) // even vouched
	if proceed {
		t.Fatalf("maxLifetime-exceeded must be a hard re-auth even when vouched")
	}
}

func TestEnforceSession_RevokedRejects(t *testing.T) {
	engine := newTestSessionEngine(session.Policy{}, 0)
	now := time.Now().Unix()
	registerTestSession(engine, "sid-1", "alice", "auth-github", now, now)
	_ = engine.manager.Store().Revoke(context.Background(), "sid-1")
	store := &Store{sessionEngine: engine}
	ctx, _ := newTestCtx()

	principal := Principal{Subject: "alice", Provider: "auth-github", SID: "sid-1"}
	proceed, _ := store.enforceSession(ctx, principal, store.sessionEngine, false)
	if proceed {
		t.Fatalf("revoked session must be rejected")
	}
}

func TestEnforceSession_GitHubVouchedIdleRenews(t *testing.T) {
	engine := newTestSessionEngine(session.Policy{IdleTimeout: time.Hour}, 0)
	now := time.Now().Unix()
	registerTestSession(engine, "sid-1", "alice", "auth-github", now, now-2*3600)
	store := &Store{sessionEngine: engine}
	ctx, _ := newTestCtx()

	principal := Principal{Subject: "alice", Provider: "auth-github", SID: "sid-1"}
	proceed, _ := store.enforceSession(ctx, principal, store.sessionEngine, true)
	if !proceed {
		t.Fatalf("expected idle+already-vouched GitHub session to silently renew")
	}
}

func TestEnforceSession_AuthSimpleIdleHardReauth(t *testing.T) {
	engine := newTestSessionEngine(session.Policy{IdleTimeout: time.Hour}, 0)
	now := time.Now().Unix()
	registerTestSession(engine, "sid-1", "admin", "auth-simple", now, now-2*3600)
	store := &Store{sessionEngine: engine}
	ctx, w := newTestCtx()

	principal := Principal{Subject: "admin", Provider: "auth-simple", SID: "sid-1"}
	proceed, _ := store.enforceSession(ctx, principal, store.sessionEngine, false)
	if proceed {
		t.Fatalf("expected auth-simple idle to require re-login")
	}
	if w.Header().Get(SessionExpiredHeader) != "true" {
		t.Fatalf("expected session-expired header")
	}
}

func TestProviderTierMapping(t *testing.T) {
	r := &Store{}
	cases := map[string]string{
		"auth-gcp-iap": tierIAP,
		"auth-github":  tierGitHub,
		"auth-saml":    tierSAML,
		"auth-simple":  tierSimple,
		"something":    tierOther,
	}
	for provider, want := range cases {
		if got := r.providerTier(provider); got != want {
			t.Fatalf("providerTier(%s) = %s, want %s", provider, got, want)
		}
	}
}

func TestAbortSessionExpiredBody(t *testing.T) {
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	abortSessionExpired(ctx)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "session expired") {
		t.Fatalf("expected session expired body, got %q", body)
	}
}
