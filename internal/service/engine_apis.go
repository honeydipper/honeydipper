// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/honeydipper/honeydipper/v4/internal/api"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/honeydipper/honeydipper/v4/pkg/workflow"
)

var (
	ErrSessionNotFound           = errors.New("session not found")
	ErrSessionNotRerunnable      = errors.New("session cannot be rerun")
	ErrSessionControlUnsupported = errors.New("session store does not support session control")
	ErrUnknownSessionAction      = errors.New("unknown action")
	ErrSessionStoreNoRerun       = errors.New("session store does not support rerun")
	ErrUnauthorized              = errors.New("unauthorized: session does not belong to this repository")
)

func setupEngineAPIs() {
	engine.APIs["eventWait"] = handleEventWait
	engine.APIs["eventList"] = handleEventList
	engine.APIs["eventRerun"] = handleEventRerun
	engine.APIs["eventPause"] = handleEventPause
	engine.APIs["eventResume"] = handleEventResume
	engine.APIs["eventInteract"] = handleEventInteract
	engine.APIs["eventCancel"] = handleEventCancel
	engine.APIs["ghEventList"] = handleGHEventList
	engine.APIs["ghEventRerun"] = handleGHEventRerun
	engine.APIs["ghEventPause"] = handleGHEventPause
	engine.APIs["ghEventResume"] = handleGHEventResume
	engine.APIs["ghEventInteract"] = handleGHEventInteract
}

type sessionController interface {
	PauseSession(sessionID string) (map[string]interface{}, error)
	ResumeSessionByID(sessionID string) (map[string]interface{}, error)
	InteractSessionByID(sessionID string, key string, actor string) (map[string]interface{}, error)
	CancelSessionByID(sessionID string, reason string) (map[string]interface{}, error)
}

func handleEventWait(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	eventID := dipper.MustGetMapDataStr(resp.Request.Payload, "eventID")

	ret := workflow.Wait(engine.context, engine, eventID, resp.Request.Labels["uuid"])

	resp.Return(ret.Dump())
}

func handleEventRerun(resp *api.Response) {
	defer func() {
		if r := recover(); r != nil {
			resp.ReturnError(r.(error))
		}
	}()

	resp.Request = dipper.DeserializePayload(resp.Request)
	sessionID := dipper.MustGetMapDataStr(resp.Request.Payload, "sessionID")
	resp.Return(dipper.Must(rerunSession(sessionID)).(map[string]interface{}))
}

func handleEventPause(resp *api.Response) {
	handleSessionControl(resp, "pause")
}

func handleEventResume(resp *api.Response) {
	handleSessionControl(resp, "resume")
}

func handleEventInteract(resp *api.Response) {
	handleSessionControl(resp, "interact")
}

func handleEventCancel(resp *api.Response) {
	handleSessionControl(resp, "cancel")
}

func handleSessionControl(resp *api.Response, action string) {
	defer func() {
		if r := recover(); r != nil {
			resp.ReturnError(r.(error))
		}
	}()

	resp.Request = dipper.DeserializePayload(resp.Request)
	sessionID := dipper.MustGetMapDataStr(resp.Request.Payload, "sessionID")
	rawPayload, _ := resp.Request.Payload.(map[string]interface{})
	input := normalizeSessionControlInput(rawPayload)

	controller, ok := sessionStore.(sessionController)
	if !ok {
		resp.ReturnError(ErrSessionControlUnsupported)

		return
	}

	var (
		ret map[string]interface{}
		err error
	)
	switch action {
	case "pause":
		ret, err = controller.PauseSession(sessionID)
	case "resume":
		ret, err = controller.ResumeSessionByID(sessionID)
	case "interact":
		key := dipper.MustGetMapDataStr(input, "key")
		actor := resolveSessionControlActor(resp.Request, input)
		ret, err = controller.InteractSessionByID(sessionID, key, actor)
	case "cancel":
		reason, _ := dipper.GetMapDataStr(input, "reason")
		ret, err = controller.CancelSessionByID(sessionID, reason)
	default:
		err = ErrUnknownSessionAction
	}
	if err != nil {
		resp.ReturnError(err)

		return
	}

	resp.Return(ret)
}

func normalizeSessionControlInput(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{}
	}

	ret := map[string]interface{}{}
	for k, v := range payload {
		ret[k] = v
	}

	body, ok := payload["body"].(string)
	if !ok || strings.TrimSpace(body) == "" {
		return ret
	}

	bodyMap := map[string]interface{}{}
	if err := json.Unmarshal([]byte(body), &bodyMap); err != nil {
		return ret
	}

	for k, v := range bodyMap {
		ret[k] = v
	}

	return ret
}

func resolveSessionControlActor(req *dipper.Message, input map[string]interface{}) string {
	if req == nil || req.Labels == nil {
		if user, ok := dipper.GetMapDataStr(input, "user"); ok {
			return strings.TrimSpace(user)
		}

		return ""
	}

	if user := strings.TrimSpace(req.Labels["user"]); user != "" {
		return user
	}
	if profileName := strings.TrimSpace(req.Labels["profile_name"]); profileName != "" {
		return profileName
	}
	if subject := strings.TrimSpace(req.Labels["subject"]); subject != "" {
		return subject
	}

	if user, ok := dipper.GetMapDataStr(input, "user"); ok {
		return strings.TrimSpace(user)
	}

	return ""
}

func rerunSession(sessionID string) (map[string]interface{}, error) {
	if sessionStore == nil {
		return nil, ErrSessionNotFound
	}

	source := workflow.GetStoredSession(sessionStore, sessionID)
	if source == nil {
		return nil, ErrSessionNotFound
	}
	if source.OriginalWorkflow == nil {
		return nil, ErrSessionNotRerunnable
	}

	eventID := dipper.NewUUID()
	eventPayload := map[string]interface{}{}
	if source.Event != nil {
		eventPayload = dipper.MustDeepCopyMap(source.Event)
	}

	var eventCtx map[string]interface{}
	if source.EventCtx != nil {
		eventCtx = dipper.MustDeepCopyMap(source.EventCtx)
	}

	var rerunCtx map[string]interface{}
	if source.RerunCtx != nil {
		rerunCtx = dipper.MustDeepCopyMap(source.RerunCtx)
	}

	loadedContexts := append([]string(nil), source.LoadedContexts...)

	created := sessionStore.StartSessionWithInitContextHook(source.OriginalWorkflow, &dipper.Message{
		Channel: "eventbus",
		Subject: "message",
		Labels: map[string]string{
			"eventID": eventID,
		},
		Payload: map[string]interface{}{
			"data": eventPayload,
		},
	}, eventCtx, rerunCtx, loadedContexts, nil)

	return map[string]interface{}{
		"eventID":         created.EventID,
		"sessionID":       created.ID,
		"sourceSessionID": sessionID,
		"sourceEventID":   source.EventID,
	}, nil
}

func handleEventList(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	lookBackStr, _ := dipper.GetMapDataStr(resp.Request.Payload, "look_back")
	asOf, _ := dipper.GetMapDataStr(resp.Request.Payload, "as_of")

	lookBack := 12 // 12 blocks by default, 24 hours of session history
	if lookBackStr != "" {
		lookBack = dipper.Must(strconv.Atoi(lookBackStr)).(int)
	}

	if asOf != "" {
		asOfParts := strings.Split(asOf, "_")
		asOf = asOfParts[len(asOfParts)-1]
	}

	resp.Return(sessionStore.DumpSessions(lookBack, asOf))
}

func handleGHEventList(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	lookBackStr, _ := dipper.GetMapDataStr(resp.Request.Payload, "look_back")
	asOf, _ := dipper.GetMapDataStr(resp.Request.Payload, "as_of")
	ghSlug, _ := dipper.GetMapDataStr(resp.Request.Payload, "gh_slug")

	lookBack := 12 // 12 blocks by default, 24 hours of session history
	if lookBackStr != "" {
		lookBack = dipper.Must(strconv.Atoi(lookBackStr)).(int)
	}

	if asOf != "" {
		asOfParts := strings.Split(asOf, "_")
		asOf = asOfParts[len(asOfParts)-1]
	}

	ghSlug = strings.TrimPrefix(strings.TrimSpace(ghSlug), "/")
	data := sessionStore.DumpSessions(lookBack, asOf)
	if ghSlug == "" {
		resp.Return(data)

		return
	}

	resp.Return(filterSessionsByGitRepo(data, ghSlug))
}

func filterSessionsByGitRepo(data []byte, ghSlug string) []byte {
	if len(bytes.TrimSpace(data)) == 0 {
		return data
	}

	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		// Keep original payload if format is unexpected.
		return data
	}

	normalizedSlug := strings.ToLower(strings.Trim(strings.TrimSpace(ghSlug), "/"))
	if normalizedSlug == "" {
		return data
	}

	isRepo := strings.Contains(normalizedSlug, "/")
	ret := make([]json.RawMessage, 0, len(items))

	for _, item := range items {
		trimmed := bytes.TrimSpace(item)
		if len(trimmed) == 0 {
			continue
		}

		if trimmed[0] != '{' {
			// Keep stream markers and non-session entries unchanged.
			ret = append(ret, item)

			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal(item, &event); err != nil {
			continue
		}

		repo := getSessionGitRepo(event)
		if strings.TrimSpace(repo) == "" {
			continue
		}

		normalizedRepo := strings.ToLower(strings.Trim(strings.TrimSpace(repo), "/"))
		if (isRepo && normalizedRepo == normalizedSlug) || (!isRepo && strings.HasPrefix(normalizedRepo, normalizedSlug+"/")) {
			ret = append(ret, item)
		}
	}

	filtered, err := json.Marshal(ret)
	if err != nil {
		return data
	}

	return filtered
}

func getSessionGitRepo(event map[string]interface{}) string {
	data, ok := event["data"].(map[string]interface{})
	if ok {
		eventCtx, ok := data["event_ctx"].(map[string]interface{})
		if ok {
			if repo, ok := eventCtx["git_repo"].(string); ok {
				return repo
			}
		}
	}

	return ""
}

func verifySessionBelongsToGHSlug(sessionID string, ghSlug string) error {
	if sessionStore == nil {
		return ErrSessionNotFound
	}

	session := workflow.GetStoredSession(sessionStore, sessionID)
	if session == nil {
		return ErrSessionNotFound
	}

	// Extract git_repo from session's EventCtx
	var repo string
	if session.EventCtx != nil {
		if r, ok := session.EventCtx["git_repo"].(string); ok {
			repo = r
		}
	}

	if strings.TrimSpace(repo) == "" {
		return ErrUnauthorized
	}

	normalizedSlug := strings.ToLower(strings.Trim(strings.TrimSpace(ghSlug), "/"))
	normalizedRepo := strings.ToLower(strings.Trim(strings.TrimSpace(repo), "/"))

	if normalizedSlug == "" {
		return ErrUnauthorized
	}

	isRepo := strings.Contains(normalizedSlug, "/")
	if (isRepo && normalizedRepo == normalizedSlug) || (!isRepo && strings.HasPrefix(normalizedRepo, normalizedSlug+"/")) {
		return nil
	}

	return ErrUnauthorized
}

func handleGHEventRerun(resp *api.Response) {
	defer func() {
		if r := recover(); r != nil {
			resp.ReturnError(r.(error))
		}
	}()

	resp.Request = dipper.DeserializePayload(resp.Request)
	sessionID := dipper.MustGetMapDataStr(resp.Request.Payload, "sessionID")
	ghSlug, _ := dipper.GetMapDataStr(resp.Request.Payload, "gh_slug")

	if err := verifySessionBelongsToGHSlug(sessionID, ghSlug); err != nil {
		resp.ReturnError(err)

		return
	}

	resp.Return(dipper.Must(rerunSession(sessionID)).(map[string]interface{}))
}

func handleGHEventPause(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	ghSlug, _ := dipper.GetMapDataStr(resp.Request.Payload, "gh_slug")
	sessionID := dipper.MustGetMapDataStr(resp.Request.Payload, "sessionID")

	if err := verifySessionBelongsToGHSlug(sessionID, ghSlug); err != nil {
		resp.ReturnError(err)

		return
	}

	handleSessionControl(resp, "pause")
}

func handleGHEventResume(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	ghSlug, _ := dipper.GetMapDataStr(resp.Request.Payload, "gh_slug")
	sessionID := dipper.MustGetMapDataStr(resp.Request.Payload, "sessionID")

	if err := verifySessionBelongsToGHSlug(sessionID, ghSlug); err != nil {
		resp.ReturnError(err)

		return
	}

	handleSessionControl(resp, "resume")
}

func handleGHEventInteract(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	ghSlug, _ := dipper.GetMapDataStr(resp.Request.Payload, "gh_slug")
	sessionID := dipper.MustGetMapDataStr(resp.Request.Payload, "sessionID")

	if err := verifySessionBelongsToGHSlug(sessionID, ghSlug); err != nil {
		resp.ReturnError(err)

		return
	}

	handleSessionControl(resp, "interact")
}
