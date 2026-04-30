// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"reflect"
	"testing"

	"github.com/crewjam/saml"
)

func TestChooseClaim(t *testing.T) {
	claims := map[string]interface{}{
		"email": []interface{}{"a@example.com"},
		"name":  "Alice",
	}
	if got := chooseClaim(claims, "name", "email"); got != "Alice" {
		t.Fatalf("expected Alice, got %q", got)
	}
	if got := chooseClaim(claims, "missing", "email"); got != "a@example.com" {
		t.Fatalf("expected a@example.com, got %q", got)
	}
}

func TestAssertionToClaims(t *testing.T) {
	assertion := &saml.Assertion{
		Subject: &saml.Subject{NameID: &saml.NameID{Value: "alice"}},
		AttributeStatements: []saml.AttributeStatement{{
			Attributes: []saml.Attribute{
				{Name: "email", Values: []saml.AttributeValue{{Value: "alice@example.com"}}},
				{Name: "groups", FriendlyName: "roles", Values: []saml.AttributeValue{{Value: "dev"}, {Value: "ops"}}},
			},
		}},
	}

	got := assertionToClaims(assertion)
	if got["nameID"] != "alice" {
		t.Fatalf("expected nameID alice, got %#v", got["nameID"])
	}
	if got["email"] != "alice@example.com" {
		t.Fatalf("expected email alice@example.com, got %#v", got["email"])
	}
	expectedGroups := []interface{}{"dev", "ops"}
	if !reflect.DeepEqual(got["groups"], expectedGroups) {
		t.Fatalf("expected groups %#v, got %#v", expectedGroups, got["groups"])
	}
	if !reflect.DeepEqual(got["roles"], expectedGroups) {
		t.Fatalf("expected roles %#v, got %#v", expectedGroups, got["roles"])
	}
}

func TestPayloadStringFromBody(t *testing.T) {
	payload := map[string]interface{}{
		"body": "SAMLResponse=abc123&RelayState=xyz",
	}
	if got, ok := payloadString(payload, "SAMLResponse"); !ok || got != "abc123" {
		t.Fatalf("expected saml response abc123, got %q (ok=%t)", got, ok)
	}
	if got, ok := payloadString(payload, "RelayState"); !ok || got != "xyz" {
		t.Fatalf("expected relay state xyz, got %q (ok=%t)", got, ok)
	}
}
