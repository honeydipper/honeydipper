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

var ErrSessionNotFound = errors.New("session not found")

func setupEngineAPIs() {
	engine.APIs["eventWait"] = handleEventWait
	engine.APIs["eventList"] = handleEventList
	engine.APIs["ghEventList"] = handleGHEventList
}

func handleEventWait(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	eventID := dipper.MustGetMapDataStr(resp.Request.Payload, "eventID")

	ret := workflow.Wait(engine.context, engine, eventID, resp.Request.Labels["uuid"])

	resp.Return(ret.Dump())
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
