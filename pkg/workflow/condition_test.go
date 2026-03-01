// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package workflow

import (
	"testing"

	cfg "github.com/honeydipper/honeydipper/v4/internal/config"
)

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Falsy values
		{"", false},
		{"false", false},
		{"FALSE", false},
		{"False", false},
		{"nil", false},
		{"NIL", false},
		{"0", false},
		{"{}", false},
		{"[]", false},
		{"<no value>", false},
		{"  false  ", false},
		{"\tnil\t", false},

		// Truthy values
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"something", true},
		{"0.1", true},
		{" non-empty ", true},
		{"  true  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isTruthy(tt.input)
			if result != tt.expected {
				t.Fatalf("isTruthy(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCheckCondition_If(t *testing.T) {
	// If: all must be truthy
	s := newSession(&cfg.Workflow{If: []string{"true", "yes"}})
	if !s.checkCondition() {
		t.Fatal("expected true when all If conditions truthy")
	}

	s = newSession(&cfg.Workflow{If: []string{"true", "false"}})
	if s.checkCondition() {
		t.Fatal("expected false when any If condition falsy")
	}

	s = newSession(&cfg.Workflow{If: []string{"false"}})
	if s.checkCondition() {
		t.Fatal("expected false when single If condition falsy")
	}

	// Empty If slice falls through to default true
	s = newSession(&cfg.Workflow{If: []string{}})
	if !s.checkCondition() {
		t.Fatal("expected true when If is empty")
	}
}

func TestCheckCondition_IfAny(t *testing.T) {
	// IfAny: at least one must be truthy
	s := newSession(&cfg.Workflow{IfAny: []string{"true", "false"}})
	if !s.checkCondition() {
		t.Fatal("expected true when any IfAny condition truthy")
	}

	s = newSession(&cfg.Workflow{IfAny: []string{"false", "false"}})
	if s.checkCondition() {
		t.Fatal("expected false when all IfAny conditions falsy")
	}

	s = newSession(&cfg.Workflow{IfAny: []string{"true"}})
	if !s.checkCondition() {
		t.Fatal("expected true when single IfAny condition truthy")
	}

	// Empty IfAny slice falls through to default true
	s = newSession(&cfg.Workflow{IfAny: []string{}})
	if !s.checkCondition() {
		t.Fatal("expected true when IfAny is empty")
	}
}

func TestCheckCondition_Unless(t *testing.T) {
	// Unless: all must be falsy
	s := newSession(&cfg.Workflow{Unless: []string{"false", "nil"}})
	if !s.checkCondition() {
		t.Fatal("expected true when all Unless conditions falsy")
	}

	s = newSession(&cfg.Workflow{Unless: []string{"true", "false"}})
	if s.checkCondition() {
		t.Fatal("expected false when any Unless condition truthy")
	}

	s = newSession(&cfg.Workflow{Unless: []string{"true"}})
	if s.checkCondition() {
		t.Fatal("expected false when Unless condition truthy")
	}

	// Empty Unless slice falls through to default true
	s = newSession(&cfg.Workflow{Unless: []string{}})
	if !s.checkCondition() {
		t.Fatal("expected true when Unless is empty")
	}
}

func TestCheckCondition_UnlessAll(t *testing.T) {
	// UnlessAll: at least one must be falsy
	s := newSession(&cfg.Workflow{UnlessAll: []string{"false", "true"}})
	if !s.checkCondition() {
		t.Fatal("expected true when any UnlessAll condition falsy")
	}

	s = newSession(&cfg.Workflow{UnlessAll: []string{"true", "yes"}})
	if s.checkCondition() {
		t.Fatal("expected false when all UnlessAll conditions truthy")
	}

	s = newSession(&cfg.Workflow{UnlessAll: []string{"false"}})
	if !s.checkCondition() {
		t.Fatal("expected true when UnlessAll condition falsy")
	}

	// Empty UnlessAll slice falls through to default true
	s = newSession(&cfg.Workflow{UnlessAll: []string{}})
	if !s.checkCondition() {
		t.Fatal("expected true when UnlessAll is empty")
	}
}

func TestCheckCondition_Match(t *testing.T) {
	// Match: uses CompareAll
	s := newSession(&cfg.Workflow{Match: map[string]interface{}{"key": "value"}})
	//nolint:goconst
	s.Ctx["key"] = "value"
	if !s.checkCondition() {
		t.Fatal("expected true when Match succeeds")
	}

	s = newSession(&cfg.Workflow{Match: map[string]interface{}{"key": "value"}})
	//nolint:goconst
	s.Ctx["key"] = "other"
	if s.checkCondition() {
		t.Fatal("expected false when Match fails")
	}
}

func TestCheckCondition_UnlessMatch(t *testing.T) {
	// UnlessMatch: inverts match result
	s := newSession(&cfg.Workflow{UnlessMatch: map[string]interface{}{"key": "value"}})
	s.Ctx["key"] = "value"
	if s.checkCondition() {
		t.Fatal("expected false when UnlessMatch condition met")
	}

	s = newSession(&cfg.Workflow{UnlessMatch: map[string]interface{}{"key": "value"}})
	s.Ctx["key"] = "other"
	if !s.checkCondition() {
		t.Fatal("expected true when UnlessMatch condition not met")
	}

	// Empty UnlessMatch
	s = newSession(&cfg.Workflow{UnlessMatch: map[string]interface{}{}})
	if !s.checkCondition() {
		t.Fatal("expected true when UnlessMatch is empty")
	}
}

func TestCheckCondition_DefaultTrue(t *testing.T) {
	// No conditions set -> default true
	s := newSession(&cfg.Workflow{})
	if !s.checkCondition() {
		t.Fatal("expected true when no conditions set")
	}
}

func TestCheckLoopCondition_While(t *testing.T) {
	// While: all must be truthy
	s := newSession(&cfg.Workflow{While: []string{"true", "yes"}})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when all While conditions truthy")
	}

	s = newSession(&cfg.Workflow{While: []string{"true", "false"}})
	if s.checkLoopCondition() {
		t.Fatal("expected false when any While condition falsy")
	}

	// Empty While slice falls through to default true
	s = newSession(&cfg.Workflow{While: []string{}})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when While is empty")
	}
}

func TestCheckLoopCondition_WhileAny(t *testing.T) {
	// WhileAny: at least one must be truthy
	s := newSession(&cfg.Workflow{WhileAny: []string{"true", "false"}})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when any WhileAny condition truthy")
	}

	s = newSession(&cfg.Workflow{WhileAny: []string{"false", "false"}})
	if s.checkLoopCondition() {
		t.Fatal("expected false when all WhileAny conditions falsy")
	}

	// Empty WhileAny slice falls through to default true
	s = newSession(&cfg.Workflow{WhileAny: []string{}})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when WhileAny is empty")
	}
}

func TestCheckLoopCondition_Until(t *testing.T) {
	// Until: all must be falsy (opposite of while)
	s := newSession(&cfg.Workflow{Until: []string{"false", "nil"}})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when all Until conditions falsy")
	}

	s = newSession(&cfg.Workflow{Until: []string{"true", "false"}})
	if s.checkLoopCondition() {
		t.Fatal("expected false when any Until condition truthy")
	}

	// Empty Until slice falls through to default true
	s = newSession(&cfg.Workflow{Until: []string{}})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when Until is empty")
	}
}

func TestCheckLoopCondition_UntilAll(t *testing.T) {
	// UntilAll: at least one must be falsy (opposite of whileany)
	s := newSession(&cfg.Workflow{UntilAll: []string{"false", "true"}})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when any UntilAll condition falsy")
	}

	s = newSession(&cfg.Workflow{UntilAll: []string{"true", "yes"}})
	if s.checkLoopCondition() {
		t.Fatal("expected false when all UntilAll conditions truthy")
	}

	// Empty UntilAll slice falls through to default true
	s = newSession(&cfg.Workflow{UntilAll: []string{}})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when UntilAll is empty")
	}
}

func TestCheckLoopCondition_WhileMatch(t *testing.T) {
	// WhileMatch: interpolate then compare
	s := newSession(&cfg.Workflow{WhileMatch: map[string]interface{}{"key": "value"}})
	s.Ctx["key"] = "value"
	if !s.checkLoopCondition() {
		t.Fatal("expected true when WhileMatch succeeds")
	}

	s = newSession(&cfg.Workflow{WhileMatch: map[string]interface{}{"key": "value"}})
	s.Ctx["key"] = "other"
	if s.checkLoopCondition() {
		t.Fatal("expected false when WhileMatch fails")
	}
}

func TestCheckLoopCondition_UntilMatch(t *testing.T) {
	// UntilMatch: interpolate then negate compare
	s := newSession(&cfg.Workflow{UntilMatch: map[string]interface{}{"key": "value"}})
	s.Ctx["key"] = "value"
	if s.checkLoopCondition() {
		t.Fatal("expected false when UntilMatch condition met")
	}

	s = newSession(&cfg.Workflow{UntilMatch: map[string]interface{}{"key": "value"}})
	s.Ctx["key"] = "other"
	if !s.checkLoopCondition() {
		t.Fatal("expected true when UntilMatch condition not met")
	}

	// Empty UntilMatch
	s = newSession(&cfg.Workflow{UntilMatch: map[string]interface{}{}})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when UntilMatch is empty")
	}
}

func TestCheckLoopCondition_DefaultTrue(t *testing.T) {
	// No loop conditions set -> default true
	s := newSession(&cfg.Workflow{})
	if !s.checkLoopCondition() {
		t.Fatal("expected true when no loop conditions set")
	}
}
