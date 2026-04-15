package main

import "testing"

func TestClampLimit(t *testing.T) {
	if got := clampLimit(-1); got != defaultTailMaxLines {
		t.Fatalf("expected default limit %d, got %d", defaultTailMaxLines, got)
	}
	if got := clampLimit(hardTailMaxLines + 123); got != hardTailMaxLines {
		t.Fatalf("expected hard max %d, got %d", hardTailMaxLines, got)
	}
	if got := clampLimit(321); got != 321 {
		t.Fatalf("expected 321, got %d", got)
	}
}

func TestParseKubernetesCursor(t *testing.T) {
	payload := map[string]interface{}{
		"cursor": map[string]interface{}{
			"pods": map[string]interface{}{
				"pod-1": map[string]interface{}{
					"containers": map[string]interface{}{
						"c1": map[string]interface{}{"ts": "2026-04-06T12:00:00Z", "skip": 7},
					},
				},
			},
		},
	}

	cursor := parseKubernetesCursor(payload)
	if cursor["pod-1"].Containers["c1"].TS != "2026-04-06T12:00:00Z" || cursor["pod-1"].Containers["c1"].Skip != 7 {
		t.Fatalf("unexpected cursor: %+v", cursor)
	}
}

func TestParseKubernetesCursorLegacyShape(t *testing.T) {
	payload := map[string]interface{}{
		"cursor": map[string]interface{}{
			"container-a": map[string]interface{}{"skip": 3},
		},
	}

	cursor := parseKubernetesCursor(payload)
	state, ok := cursor["container-a"]
	if !ok {
		t.Fatalf("expected legacy cursor key to be parsed: %+v", cursor)
	}
	if state.Containers["container-a"].Skip != 3 {
		t.Fatalf("unexpected legacy cursor state: %+v", state)
	}
}

func TestExtractLineTimestamp(t *testing.T) {
	ts := extractLineTimestamp("2026-04-06T12:00:00.123456789Z hello")
	if ts == "" {
		t.Fatal("expected RFC3339Nano timestamp to be detected")
	}

	ts = extractLineTimestamp("not-a-ts hello")
	if ts != "" {
		t.Fatalf("expected empty timestamp for invalid line, got %q", ts)
	}
}

func TestFilterByCursor(t *testing.T) {
	lines := []string{
		"2026-04-06T12:00:00Z one",
		"2026-04-06T12:00:00Z two",
		"2026-04-06T12:00:01Z three",
	}

	state := containerCursor{TS: "2026-04-06T12:00:00Z", Skip: 1}
	out := filterByCursor(lines, state)
	if len(out) != 2 {
		t.Fatalf("expected 2 lines after filtering, got %d (%+v)", len(out), out)
	}
	if out[0] != "2026-04-06T12:00:00Z two" || out[1] != "2026-04-06T12:00:01Z three" {
		t.Fatalf("unexpected filtered lines: %+v", out)
	}
}

func TestUpdateCursorWithLine(t *testing.T) {
	state := containerCursor{}
	state = updateCursorWithLine(state, "2026-04-06T12:00:00Z first")
	if state.TS != "2026-04-06T12:00:00Z" || state.Skip != 1 {
		t.Fatalf("unexpected state after first line: %+v", state)
	}

	state = updateCursorWithLine(state, "2026-04-06T12:00:00Z second")
	if state.Skip != 2 {
		t.Fatalf("expected skip=2 for same timestamp, got %+v", state)
	}

	state = updateCursorWithLine(state, "2026-04-06T12:00:01Z third")
	if state.TS != "2026-04-06T12:00:01Z" || state.Skip != 1 {
		t.Fatalf("unexpected state after new timestamp: %+v", state)
	}
}

func TestPodDone(t *testing.T) {
	if podDone(nil) {
		t.Fatal("nil pod should not be done")
	}
}
