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
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/v4/internal/api/mock_api"
	"github.com/honeydipper/honeydipper/v4/internal/api/session"
)

func TestLogoutHandlerRevokesCurrentSID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	engine := newTestSessionEngine(session.Policy{}, 0)
	now := time.Now().Unix()
	registerTestSession(engine, "sid-1", "alice", "auth-github", now, now)

	mockCtx := mock_api.NewMockRequestContext(ctrl)
	mockCtx.EXPECT().Get("principal").Return(Principal{Subject: "alice", Provider: "auth-github", SID: "sid-1"}, true)
	mockCtx.EXPECT().GetParam("all").Return("").AnyTimes()

	store := &Store{sessionEngine: engine}
	req := &Request{store: store, ctx: mockCtx}
	result, err := logoutHandler(req)
	if err != nil {
		t.Fatalf("logoutHandler: %v", err)
	}
	if result["loggedOut"] != true {
		t.Fatalf("expected loggedOut result, got %+v", result)
	}

	// The sid must now be revoked in the store.
	r, _ := store.sessionEngine.manager.Store().Get(context.Background(), "sid-1")
	if !r.Revoked {
		t.Fatalf("expected sid revoked after logout")
	}
}

func TestLogoutHandlerRevokeAllForSubject(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	engine := newTestSessionEngine(session.Policy{}, 0)
	now := time.Now().Unix()
	registerTestSession(engine, "sid-1", "bob", "auth-github", now, now)
	registerTestSession(engine, "sid-2", "bob", "auth-github", now, now)

	mockCtx := mock_api.NewMockRequestContext(ctrl)
	mockCtx.EXPECT().Get("principal").Return(Principal{Subject: "bob", Provider: "auth-github", SID: "sid-1"}, true)
	mockCtx.EXPECT().GetParam("all").Return("true").AnyTimes()

	store := &Store{sessionEngine: engine}
	req := &Request{store: store, ctx: mockCtx}
	if _, err := logoutHandler(req); err != nil {
		t.Fatalf("logoutHandler: %v", err)
	}

	for _, sid := range []string{"sid-1", "sid-2"} {
		r, _ := store.sessionEngine.manager.Store().Get(context.Background(), sid)
		if !r.Revoked {
			t.Fatalf("expected %s revoked in revoke-all", sid)
		}
	}
}

func TestLogoutHandlerNoPrincipalAborts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCtx := mock_api.NewMockRequestContext(ctrl)
	mockCtx.EXPECT().Get("principal").Return(nil, false)
	mockCtx.EXPECT().AbortWithStatusJSON(gomock.Any(), gomock.Any()).Return()

	store := &Store{sessionEngine: newTestSessionEngine(session.Policy{}, 0)}
	req := &Request{store: store, ctx: mockCtx}
	result, err := logoutHandler(req)
	if err != nil {
		t.Fatalf("logoutHandler: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result on abort, got %+v", result)
	}
}

func TestLogoutRouteDefRegistered(t *testing.T) {
	defs := GetDefs()
	routeDefs, ok := defs["auth/logout"]
	if !ok {
		t.Fatalf("missing auth/logout route")
	}
	def, ok := routeDefs[http.MethodPost]
	if !ok {
		t.Fatalf("missing POST auth/logout")
	}
	if def.ReqType != TypeLocal {
		t.Fatalf("expected local handler")
	}
	if def.AllowAnonymous {
		t.Fatalf("logout must not be anonymous")
	}
}
