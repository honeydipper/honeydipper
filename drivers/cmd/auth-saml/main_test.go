// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"encoding/base64"
	"reflect"
	"testing"

	saml2types "github.com/russellhaering/gosaml2/types"
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
	assertion := &saml2types.Assertion{
		Subject: &saml2types.Subject{NameID: &saml2types.NameID{Value: "alice"}},
		AttributeStatement: &saml2types.AttributeStatement{
			Attributes: []saml2types.Attribute{
				{Name: "email", Values: []saml2types.AttributeValue{{Value: "alice@example.com"}}},
				{Name: "groups", FriendlyName: "roles", Values: []saml2types.AttributeValue{{Value: "dev"}, {Value: "ops"}}},
			},
		},
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

func TestExtractSAMLStatusNestedCode(t *testing.T) {
	xml := `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol">` +
		`<samlp:Status>` +
		`<samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:Responder">` +
		`<samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:InvalidNameIDPolicy"/>` +
		`</samlp:StatusCode>` +
		`</samlp:Status>` +
		`</samlp:Response>`

	encoded := base64.StdEncoding.EncodeToString([]byte(xml))
	statusPath, statusMessage, ok := extractSAMLStatus(encoded)
	if !ok {
		t.Fatalf("expected status to be extracted")
	}
	if statusMessage != "" {
		t.Fatalf("expected empty status message, got %q", statusMessage)
	}
	expected := "urn:oasis:names:tc:SAML:2.0:status:Responder -> urn:oasis:names:tc:SAML:2.0:status:InvalidNameIDPolicy"
	if statusPath != expected {
		t.Fatalf("expected status path %q, got %q", expected, statusPath)
	}
}

func TestExtractSAMLStatusSuccess(t *testing.T) {
	xml := `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol">` +
		`<samlp:Status>` +
		`<samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:Success"/>` +
		`</samlp:Status>` +
		`</samlp:Response>`

	encoded := base64.StdEncoding.EncodeToString([]byte(xml))
	statusPath, statusMessage, ok := extractSAMLStatus(encoded)
	if !ok {
		t.Fatalf("expected status to be extracted")
	}
	if statusMessage != "" {
		t.Fatalf("expected empty status message, got %q", statusMessage)
	}
	if statusPath != "urn:oasis:names:tc:SAML:2.0:status:Success" {
		t.Fatalf("unexpected status path %q", statusPath)
	}
}
