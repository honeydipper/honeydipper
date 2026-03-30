// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"encoding/json"
	"testing"
)

func TestFilterSessionsByGitRepo_RepoMatch(t *testing.T) {
	input := []byte(`[
		"session_stream_2026032810",
		"session_stream_2026032812",
		{"ID":"1","data":{"event_ctx":{"git_repo":"honeydipper/honeydipper"}}},
		{"ID":"2","data":{"event_ctx":{"git_repo":"honeydipper/other"}}},
		{"ID":"3","data":{"event_ctx":{"git_repo":"otherorg/honeydipper"}}}
	]`)

	out := filterSessionsByGitRepo(input, "honeydipper/honeydipper")

	var got []interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unexpected json decode error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 items (2 markers + 1 session), got %d", len(got))
	}
}

func TestFilterSessionsByGitRepo_OrgMatch(t *testing.T) {
	input := []byte(`[
		"session_stream_2026032810",
		"session_stream_2026032812",
		{"ID":"1","data":{"event_ctx":{"git_repo":"honeydipper/honeydipper"}}},
		{"ID":"2","data":{"event_ctx":{"git_repo":"honeydipper/other"}}},
		{"ID":"3","data":{"event_ctx":{"git_repo":"otherorg/honeydipper"}}}
	]`)

	out := filterSessionsByGitRepo(input, "honeydipper")

	var got []interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unexpected json decode error: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("expected 4 items (2 markers + 2 sessions), got %d", len(got))
	}
}

func TestFilterSessionsByGitRepo_ExcludeMissingCtx(t *testing.T) {
	input := []byte(`[
		"session_stream_2026032810",
		"session_stream_2026032812",
		{"ID":"1","data":{"event_ctx":{"git_repo":"honeydipper/honeydipper"}}},
		{"ID":"2","data":{"event_ctx":{}}},
		{"ID":"3"}
	]`)

	out := filterSessionsByGitRepo(input, "honeydipper")

	var got []interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unexpected json decode error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 items (2 markers + 1 session), got %d", len(got))
	}
}
