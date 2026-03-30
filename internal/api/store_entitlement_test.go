// Copyright 2026 Chun Huang (Charles).

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package api

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/v4/internal/api/mock_api"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper/mock_dipper"
)

func TestCheckEntitlementEmptyStringResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockCaller := mock_dipper.NewMockRPCCaller(ctrl)
	store := NewStore(mockCaller)

	principal := Principal{Subject: "charles", Provider: "auth-github"}
	def := Def{EntitlementProvider: "auth-github", EntitlementKey: "gh_slug"}
	cacheKey := store.entitlementCacheKey(def.EntitlementProvider, principal.Subject, "honeydipper")

	mockReqCtx.EXPECT().Get("principal").Return(principal, true)
	mockReqCtx.EXPECT().GetParam("gh_slug").Return("honeydipper")
	mockCaller.EXPECT().Call(
		"cache",
		"load",
		map[string]interface{}{"key": cacheKey},
	).Return(nil, nil)
	mockCaller.EXPECT().Call(
		"driver:auth-github",
		"check_entitlements",
		map[string]interface{}{
			"principal":         principal,
			"entitlementTarget": "honeydipper",
		},
	).Return([]byte(`""`), nil)

	if ok := store.CheckEntitlement(mockReqCtx, def); ok {
		t.Fatalf("expected CheckEntitlement to return false for empty string payload")
	}
}

func TestCheckEntitlementSingleStringResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockCaller := mock_dipper.NewMockRPCCaller(ctrl)
	store := NewStore(mockCaller)

	principal := Principal{Subject: "charles", Provider: "auth-github"}
	def := Def{EntitlementProvider: "auth-github", EntitlementKey: "gh_slug"}
	cacheKey := store.entitlementCacheKey(def.EntitlementProvider, principal.Subject, "honeydipper")

	mockReqCtx.EXPECT().Get("principal").Return(principal, true)
	mockReqCtx.EXPECT().GetParam("gh_slug").Return("honeydipper")
	mockCaller.EXPECT().Call(
		"cache",
		"load",
		map[string]interface{}{"key": cacheKey},
	).Return(nil, nil)
	mockCaller.EXPECT().Call(
		"driver:auth-github",
		"check_entitlements",
		map[string]interface{}{
			"principal":         principal,
			"entitlementTarget": "honeydipper",
		},
	).Return([]byte(`"org:members"`), nil)
	mockCaller.EXPECT().Call(
		"cache",
		"save",
		map[string]interface{}{
			"key":   cacheKey,
			"value": `"org:members"`,
			"ttl":   "30m0s",
		},
	).Return(nil, nil)
	mockReqCtx.EXPECT().Set("derivedSubjects", []string{"org:members"})

	if ok := store.CheckEntitlement(mockReqCtx, def); !ok {
		t.Fatalf("expected CheckEntitlement to return true for single string payload")
	}
}

func TestCheckEntitlementUsesCacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockCaller := mock_dipper.NewMockRPCCaller(ctrl)
	store := NewStore(mockCaller)

	principal := Principal{Subject: "charles", Provider: "auth-github"}
	def := Def{EntitlementProvider: "auth-github", EntitlementKey: "gh_slug"}
	cacheKey := store.entitlementCacheKey(def.EntitlementProvider, principal.Subject, "honeydipper")

	mockReqCtx.EXPECT().Get("principal").Return(principal, true)
	mockReqCtx.EXPECT().GetParam("gh_slug").Return("honeydipper")
	mockCaller.EXPECT().Call(
		"cache",
		"load",
		map[string]interface{}{"key": cacheKey},
	).Return([]byte(`["org:members"]`), nil)
	mockReqCtx.EXPECT().Set("derivedSubjects", []string{"org:members"})

	if ok := store.CheckEntitlement(mockReqCtx, def); !ok {
		t.Fatalf("expected CheckEntitlement to return true for cached payload")
	}
}

func TestCheckEntitlementCacheMissSavesResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockCaller := mock_dipper.NewMockRPCCaller(ctrl)
	store := NewStore(mockCaller)

	principal := Principal{Subject: "charles", Provider: "auth-github"}
	def := Def{EntitlementProvider: "auth-github", EntitlementKey: "gh_slug"}
	cacheKey := store.entitlementCacheKey(def.EntitlementProvider, principal.Subject, "honeydipper")

	mockReqCtx.EXPECT().Get("principal").Return(principal, true)
	mockReqCtx.EXPECT().GetParam("gh_slug").Return("honeydipper")
	mockCaller.EXPECT().Call(
		"cache",
		"load",
		map[string]interface{}{"key": cacheKey},
	).Return(nil, nil)
	mockCaller.EXPECT().Call(
		"driver:auth-github",
		"check_entitlements",
		map[string]interface{}{
			"principal":         principal,
			"entitlementTarget": "honeydipper",
		},
	).Return([]byte(`["org:members"]`), nil)
	mockCaller.EXPECT().Call(
		"cache",
		"save",
		map[string]interface{}{
			"key":   cacheKey,
			"value": `["org:members"]`,
			"ttl":   "30m0s",
		},
	).Return(nil, nil)
	mockReqCtx.EXPECT().Set("derivedSubjects", []string{"org:members"})

	if ok := store.CheckEntitlement(mockReqCtx, def); !ok {
		t.Fatalf("expected CheckEntitlement to return true after cache miss and save")
	}
}
