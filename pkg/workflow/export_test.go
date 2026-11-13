// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package workflow

import (
	"strings"
	"testing"

	cfg "github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// reuse newSession from other tests and customStore defined elsewhere

// TestProcessNoExport_Wildcard ensures that "*" removes everything.
func TestProcessNoExport_Wildcard(t *testing.T) {
	s := newSession(&cfg.Workflow{NoExport: []string{"*"}})
	exported := map[string]interface{}{"a": 1, "b": 2, "c-": 3, "d+": 4}
	s.processNoExport(exported)
	if len(exported) != 0 {
		t.Fatalf("expected map empty, got %+v", exported)
	}
}

// TestProcessNoExport_Specific verifies individual keys are pruned.
func TestProcessNoExport_Specific(t *testing.T) {
	s := newSession(&cfg.Workflow{NoExport: []string{"foo"}})
	exported := map[string]interface{}{"foo": 1, "foo-": 2, "foo+": 3, "bar": 4}
	s.processNoExport(exported)
	if _, ok := exported["foo"]; ok {
		t.Error("foo should have been removed")
	}
	if _, ok := exported["foo-"]; ok {
		t.Error("foo- should have been removed")
	}
	if _, ok := exported["foo+"]; ok {
		t.Error("foo+ should have been removed")
	}
	if exported["bar"] != 4 {
		t.Error("bar should remain")
	}
}

// TestProcessExport_Basic exercises merging and appending behaviour.
func TestProcessExport_Basic(t *testing.T) {
	s := newSession(&cfg.Workflow{NoExport: []string{"bad"}})
	s.Ctx = map[string]interface{}{"orig": "v"}
	env := map[string]interface{}{"ctx": s.Ctx}

	// nonempty delta
	exportMap := map[string]interface{}{"ok": "yes", "bad": "no"}
	s.processExport(exportMap, env)
	if s.Ctx["ok"] != "yes" {
		t.Error("ctx should include ok")
	}
	// no-export should not remove the key from the session context
	if _, ok := s.Ctx["bad"]; !ok {
		t.Error("bad key should remain in ctx")
	}
	if len(s.Exported) != 1 {
		t.Fatalf("expected 1 exported entry, got %d", len(s.Exported))
	}
	// exported slice should include all keys from the export map, even no-export keys
	// no-export is only applied to child exports, not the session's own exports
	if _, ok := s.Exported[0]["bad"]; !ok {
		t.Errorf("exported entry should include bad")
	}
	// empty delta shouldn't append
	s.Exported = nil
	exportMap = map[string]interface{}{}
	s.processExport(exportMap, env)
	if len(s.Exported) != 0 {
		t.Fatal("expected no exported entries for empty map")
	}
}

// makeExportSession builds a minimal session with logger/config store. It
// also ensures the global logger is initialized so that deferred error
// handlers (SafeExitOnError) don't panic when they attempt to log.
func makeExportSession(wf *cfg.Workflow) *Session {
	if dipper.Logger == nil {
		dipper.GetLogger("test", "ERROR")
	}

	s := newSession(wf)
	s.store = &customStore{} // from session_init_test
	s.CurrentMsg = newMsg()
	s.Ctx = map[string]interface{}{}

	return s
}

// TestProcessWorkflowExport_StatusPaths covers success/failure/error branches.
func TestProcessWorkflowExport_StatusPaths(t *testing.T) {
	wf := &cfg.Workflow{
		Export:          map[string]interface{}{"e": "1"},
		ExportOnSuccess: map[string]interface{}{"s": "1"},
		ExportOnFailure: map[string]interface{}{"f": "1"},
		ExportOnError:   map[string]interface{}{"err": "1"},
	}
	s := makeExportSession(wf)

	// success
	s.CurrentMsg.Labels["status"] = SessionStatusSuccess
	s.processWorkflowExport()
	if s.Ctx["e"] != "1" || s.Ctx["s"] != "1" {
		t.Error("success export not applied")
	}
	s.Ctx = map[string]interface{}{}
	s.Exported = nil

	// failure
	s.CurrentMsg.Labels["status"] = SessionStatusFailure
	s.processWorkflowExport()
	if s.Ctx["e"] != "1" || s.Ctx["f"] != "1" {
		t.Error("failure export not applied")
	}
	s.Ctx = map[string]interface{}{}
	s.Exported = nil

	// error status triggers ExportOnError only
	s.CurrentMsg.Labels["status"] = SessionStatusError
	s.processWorkflowExport()
	if s.Ctx["err"] != "1" {
		t.Error("error export not applied")
	}
}

// TestProcessWorkflowExport_InFly merges function export when status is not
// error. This also exercises the code path that appends to Exported slice.
func TestProcessWorkflowExport_InFly(t *testing.T) {
	wf := &cfg.Workflow{Export: map[string]interface{}{"e": "1"}}
	s := makeExportSession(wf)
	s.InFlyFunction = &cfg.Function{Export: map[string]interface{}{"x": "y"}}
	s.CurrentMsg.Labels["status"] = SessionStatusSuccess
	s.processWorkflowExport()
	if s.Ctx["x"] != "y" {
		t.Error("in-fly export missing")
	}
	if len(s.Exported) == 0 {
		t.Error("exported slice not populated")
	}
}

// TestProcessWorkflowExport_InFly_Skipped ensures an in-fly function is not
// evaluated when the session already carries an error status. In that case
// the export should remain untouched and the exported slice empty.
func TestProcessWorkflowExport_InFly_Skipped(t *testing.T) {
	wf := &cfg.Workflow{Export: map[string]interface{}{"e": "1"}}
	s := makeExportSession(wf)
	s.InFlyFunction = &cfg.Function{Export: map[string]interface{}{"x": "y"}}
	s.CurrentMsg.Labels["status"] = SessionStatusError
	s.processWorkflowExport()
	if _, ok := s.Ctx["x"]; ok {
		t.Error("in-fly export should have been skipped")
	}
	if len(s.Exported) != 0 {
		t.Error("exported slice should remain empty")
	}
}

// TestProcessWorkflowExport_PanicHandled ensures panic is recovered.
func TestProcessWorkflowExport_PanicHandled(t *testing.T) {
	wf := &cfg.Workflow{}
	s := makeExportSession(wf)
	s.Performing = &[]string{"act"}
	s.InFlyFunction = &cfg.Function{Target: cfg.Action{System: "nope", Function: "f"}}
	s.CurrentMsg.Labels["status"] = SessionStatusSuccess
	// pre-populate reason to exercise the newline concatenation logic in the
	// deferred SafeExitOnError handler.
	s.CurrentMsg.Labels["reason"] = "initial"
	s.processWorkflowExport()
	if s.CurrentMsg.Labels["status"] != SessionStatusError {
		t.Error("panic should set status error")
	}
	if !strings.Contains(s.CurrentMsg.Labels["reason"], "Error on exporting data") {
		t.Error("reason not recorded")
	}
	if !strings.Contains(s.CurrentMsg.Labels["reason"], "initial") {
		t.Error("existing reason should be preserved")
	}
	if !strings.Contains(s.CurrentMsg.Labels["reason"], "\n") {
		t.Error("reason should be joined with newline")
	}
	if s.CurrentMsg.Labels["performing"] == "" {
		t.Error("performing should be recorded")
	}
}
