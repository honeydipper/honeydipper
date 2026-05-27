// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"strconv"
	"strings"

	"github.com/honeydipper/honeydipper/v4/internal/agent"
	"github.com/honeydipper/honeydipper/v4/internal/api"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

func setupAgentAPIs() {
	agentSvc.APIs["convoList"] = handleConvoList
	agentSvc.APIs["convoHistory"] = handleConvoHistory
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
