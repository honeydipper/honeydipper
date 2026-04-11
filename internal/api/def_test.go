// Copyright 2026 Chun Huang (Charles).

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package api

import (
	"net/http"
	"testing"
)

func TestGetDefsIncludesGHPodLogChunkEntitlementRoute(t *testing.T) {
	defs := GetDefs()

	routeDefs, ok := defs["gh/pods/:pod_id/log/chunk/*gh_slug"]
	if !ok {
		t.Fatalf("missing gh pod log chunk route")
	}

	def, ok := routeDefs[http.MethodGet]
	if !ok {
		t.Fatalf("missing GET definition for gh pod log chunk route")
	}

	if def.Name != "ghPodLogChunk" {
		t.Fatalf("unexpected API name %q", def.Name)
	}
	if def.Service != "operator" {
		t.Fatalf("unexpected API service %q", def.Service)
	}
	if def.ReqType != TypeFirst {
		t.Fatalf("unexpected API req type %d", def.ReqType)
	}
	if def.EntitlementProvider != "auth-github" {
		t.Fatalf("unexpected entitlement provider %q", def.EntitlementProvider)
	}
	if def.EntitlementKey != "gh_slug" {
		t.Fatalf("unexpected entitlement key %q", def.EntitlementKey)
	}
}

func TestGetDefsIncludesEventRerunRoute(t *testing.T) {
	defs := GetDefs()

	routeDefs, ok := defs["events/:sessionID/rerun"]
	if !ok {
		t.Fatalf("missing event rerun route")
	}

	def, ok := routeDefs[http.MethodPost]
	if !ok {
		t.Fatalf("missing POST definition for event rerun route")
	}

	if def.Name != "eventRerun" {
		t.Fatalf("unexpected API name %q", def.Name)
	}
	if def.Service != "engine" {
		t.Fatalf("unexpected API service %q", def.Service)
	}
	if def.Object != "event" {
		t.Fatalf("unexpected API object %q", def.Object)
	}
	if def.ReqType != TypeFirst {
		t.Fatalf("unexpected API req type %d", def.ReqType)
	}
}
