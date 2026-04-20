// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/honeydipper/honeydipper/v4/pkg/workflow"
)

type rerunHelper struct {
	mu          sync.Mutex
	calls       []string
	resp        map[string][]byte
	rpushValues []string
}

func (f *rerunHelper) record(feature, method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, feature+":"+method)
}

func (f *rerunHelper) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	f.record(feature, method)
	if feature == "cache" && method == "rpush" {
		if m, ok := params.(map[string]any); ok {
			if value, ok := m["value"].(string); ok {
				f.rpushValues = append(f.rpushValues, value)
			}
		}
	}
	if m, ok := params.(map[string]any); ok {
		if key, ok := m["key"].(string); ok {
			if v, ok := f.resp[feature+":"+method+":"+key]; ok {
				return v, nil
			}
		}
	}
	if v, ok := f.resp[feature+":"+method]; ok {
		return v, nil
	}

	return []byte("1"), nil
}

func (f *rerunHelper) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	f.record(feature, method)

	return nil
}

func (f *rerunHelper) CallRaw(feature string, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return f.Call(feature, method, params, labelsKV...)
}

func (f *rerunHelper) CallRawNoWait(feature string, method string, params []byte, rpcID string, labelsKV ...string) error {
	return f.CallNoWait(feature, method, params, labelsKV...)
}

func (f *rerunHelper) GetName() string                                     { return "rerun-helper" }
func (f *rerunHelper) CallWithMessage(msg *dipper.Message) ([]byte, error) { return nil, nil }
func (f *rerunHelper) CallWithMessageNoWait(msg *dipper.Message) error     { return nil }
func (f *rerunHelper) GetConfig() *config.Config {
	return &config.Config{DataSet: &config.DataSet{Contexts: map[string]interface{}{}}}
}
func (f *rerunHelper) SendMessage(msg *dipper.Message) {}

func TestRerunSession_HappyPath(t *testing.T) {
	helper := &rerunHelper{resp: map[string][]byte{"locker:getID": []byte("100")}}
	store := workflow.NewStore(helper)
	ps := store.(*workflow.PersistedStore)
	ps.Logger = dipper.GetLogger("engine-api-test", "ERROR")

	source := workflow.NewSession("sid-1", &config.Workflow{Name: "demo {{.event.name}}", Description: "desc"}, store)
	source.CurrentMsg = &dipper.Message{Labels: map[string]string{"cursor": "0"}}
	source.EventID = "evt-source"
	source.Event = map[string]interface{}{"name": "payload"}
	source.EventCtx = map[string]interface{}{"git_repo": "org/repo"}
	source.RerunCtx = map[string]interface{}{"thread_number": 7}
	source.LoadedContexts = []string{"_preloaded_ctx"}
	helper.resp["cache:lrange:"+workflow.StoreSessionPrefix+"sid-1"] = source.Marshal()

	prev := sessionStore
	sessionStore = store
	defer func() { sessionStore = prev }()

	result, err := rerunSession("sid-1")
	if err != nil {
		t.Fatalf("unexpected rerun error: %v", err)
	}

	if result["sourceSessionID"] != "sid-1" {
		t.Fatalf("unexpected sourceSessionID: %+v", result)
	}
	if result["sourceEventID"] != "evt-source" {
		t.Fatalf("unexpected sourceEventID: %+v", result)
	}
	if result["sessionID"] == "" {
		t.Fatalf("expected new sessionID in rerun result: %+v", result)
	}
	if result["eventID"] == "" {
		t.Fatalf("expected new eventID in rerun result: %+v", result)
	}

	if len(helper.rpushValues) == 0 {
		t.Fatalf("expected rerun-created session to be persisted")
	}
	createdSession := &workflow.Session{}
	createdSession.Unmarshal([]byte(helper.rpushValues[len(helper.rpushValues)-1]))
	if createdSession.EventCtx["git_repo"] != "org/repo" {
		t.Fatalf("expected event ctx to be preserved, got %+v", createdSession.EventCtx)
	}
	if createdSession.Ctx["thread_number"] != float64(7) {
		t.Fatalf("expected rerun ctx to be injected into runtime context, got %+v", createdSession.Ctx)
	}
	if len(createdSession.LoadedContexts) != 1 || createdSession.LoadedContexts[0] != "_preloaded_ctx" {
		t.Fatalf("expected loaded contexts to be preserved, got %+v", createdSession.LoadedContexts)
	}
}

func TestRerunSession_NotRerunnable(t *testing.T) {
	helper := &rerunHelper{resp: map[string][]byte{}}
	store := workflow.NewStore(helper)
	source := &workflow.Session{ID: "sid-2", EventID: "evt-2", CurrentMsg: &dipper.Message{Labels: map[string]string{"cursor": "0"}}}
	helper.resp["cache:lrange:"+workflow.StoreSessionPrefix+"sid-2"] = source.Marshal()

	prev := sessionStore
	sessionStore = store
	defer func() { sessionStore = prev }()

	_, err := rerunSession("sid-2")
	if err == nil {
		t.Fatal("expected rerun error for non-rerunnable session")
	}
	if !errors.Is(err, ErrSessionNotRerunnable) {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
