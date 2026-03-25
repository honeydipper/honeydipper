// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
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
