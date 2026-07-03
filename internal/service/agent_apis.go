// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/honeydipper/honeydipper/v4/internal/agent"
	"github.com/honeydipper/honeydipper/v4/internal/api"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

func setupAgentAPIs() {
	agentSvc.APIs["convoList"] = handleConvoList
	agentSvc.APIs["convoHistory"] = handleConvoHistory
	agentSvc.APIs["convoCancel"] = handleConvoCancelAPI
	agentSvc.APIs["convoTurn"] = handleConvoTurnAPI
	agentSvc.APIs["convoNew"] = handleConvoNewAPI
	agentSvc.APIs["agentList"] = handleAgentListAPI
	agentSvc.APIs["agentEngines"] = handleAgentListEnginesAPI
}

// buildUserTag combines the authenticated user name and provider into a tag like "user@provider".
// If either is empty the other is returned as-is; if both are empty the empty string is returned.
func buildUserTag(user, provider string) string {
	user = strings.TrimSpace(user)
	provider = strings.TrimSpace(provider)
	switch {
	case user != "" && provider != "":
		return user + "@" + provider
	case user != "":
		return user
	default:
		return provider
	}
}

// handleConvoList returns a stream_hvals snapshot of recent convo_stream_ blocks.
// Query params: look_back (int, number of 2-h blocks, default 12), as_of (cursor).
func handleConvoList(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	lookBackStr, _ := dipper.GetMapDataStr(resp.Request.Payload, "look_back")
	asOf, _ := dipper.GetMapDataStr(resp.Request.Payload, "as_of")

	lookBack := 12
	if lookBackStr != "" {
		lookBack = dipper.Must(strconv.Atoi(lookBackStr)).(int)
	}

	if asOf != "" {
		parts := strings.Split(asOf, "_")
		asOf = parts[len(parts)-1]
	}

	data := dipper.Must(agentStore.Call("cache", "stream_hvals", map[string]any{
		"prefix":    agent.ConvoStream,
		"look_back": lookBack,
		"asOf":      asOf,
	})).([]byte)

	resp.Return(data)
}

// handleConvoHistory returns the full chat history for a single conversation.
// Expects convoID path param in the payload.
func handleConvoHistory(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	convoID := dipper.MustGetMapDataStr(resp.Request.Payload, "convoID")

	data := dipper.Must(agentStore.Call("cache", "lrange", map[string]any{
		"key": agent.ConvoHistoryKeyPrefix + convoID,
	})).([]byte)

	resp.Return(data)
}

// handleConvoCancelAPI marks a conversation as cancelled by convo_id.
// Expects convoID path param in the payload. Active sessions belonging to the
// conversation will detect the flag on their next poll cycle and abort.
func handleConvoCancelAPI(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	convoID := dipper.MustGetMapDataStr(resp.Request.Payload, "convoID")

	agentStore.CancelConvo(&dipper.Message{
		Payload: map[string]any{"convo_id": convoID},
	})

	resp.Return([]byte(`{"ok":true}`))
}

// handleConvoTurnAPI starts a new chat turn on an existing conversation from the UI.
// Expects convoID path param plus text (and optional user, engine, driver) in the
// request body. The turn runs asynchronously; the API returns immediately with {"ok":true}.
func handleConvoTurnAPI(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	convoID := dipper.MustGetMapDataStr(resp.Request.Payload, "convoID")

	// POST body arrives as a JSON string under payload["body"].
	var body struct {
		Text   string `json:"text"`
		Engine string `json:"engine"`
		Driver string `json:"driver"`
	}
	if bodyStr, ok := dipper.GetMapDataStr(resp.Request.Payload, "body"); ok && bodyStr != "" {
		_ = json.Unmarshal([]byte(bodyStr), &body)
	}

	user := buildUserTag(resp.Request.Labels["user"], resp.Request.Labels["user_provider"])
	agentStore.StartTurn(convoID, body.Text, user, body.Engine, body.Driver)

	resp.Return([]byte(`{"ok":true}`))
}

// handleConvoNewAPI starts a brand-new conversation from the UI without an
// existing convo ID. The agent name, user message text, and optional user are
// supplied in the request body. The generated convo_id is returned synchronously
// so the UI can navigate directly to the new conversation.
func handleConvoNewAPI(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)

	var body struct {
		Agent  string `json:"agent"`
		Text   string `json:"text"`
		Engine string `json:"engine"`
		Driver string `json:"driver"`
	}
	if bodyStr, ok := dipper.GetMapDataStr(resp.Request.Payload, "body"); ok && bodyStr != "" {
		_ = json.Unmarshal([]byte(bodyStr), &body)
	}

	user := buildUserTag(resp.Request.Labels["user"], resp.Request.Labels["user_provider"])
	convoID := agentStore.StartNewConvo(body.Agent, body.Text, user, body.Engine, body.Driver)

	data := dipper.Must(json.Marshal(map[string]string{"convo_id": convoID})).([]byte)
	resp.Return(data)
}

// handleAgentListAPI returns a sorted JSON array of agent names from the current
// configuration. The UI uses this to populate the agent-selection dropdown.
func handleAgentListAPI(resp *api.Response) {
	agents := agentStore.GetConfig().DataSet.Agents
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)

	data := dipper.Must(json.Marshal(names)).([]byte)
	resp.Return(data)
}

// handleAgentListEnginesAPI returns a sorted JSON array of unique {driver, engine}
// pairs from all configured agents. The UI uses this to populate the engine/driver
// override dropdown. Pairs are deduplicated by "driver:engine" key.
func handleAgentListEnginesAPI(resp *api.Response) {
	agents := agentStore.GetConfig().DataSet.Agents
	type engineEntry struct {
		Driver string `json:"driver"`
		Engine string `json:"engine"`
	}
	seen := map[string]bool{}
	entries := []engineEntry{}
	for _, agent := range agents {
		key := agent.Driver + ":" + agent.Engine
		if seen[key] {
			continue
		}
		seen[key] = true
		entries = append(entries, engineEntry{Driver: agent.Driver, Engine: agent.Engine})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Driver+":"+entries[i].Engine < entries[j].Driver+":"+entries[j].Engine
	})

	data := dipper.Must(json.Marshal(entries)).([]byte)
	resp.Return(data)
}
