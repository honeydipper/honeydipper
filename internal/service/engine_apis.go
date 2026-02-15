// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"time"

	"github.com/honeydipper/honeydipper/v3/internal/api"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

func setupEngineAPIs() {
	engine.APIs["eventWait"] = handleEventWait
	engine.APIs["eventList"] = handleEventList
}

func handleEventWait(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	eventID := dipper.MustGetMapDataStr(resp.Request.Payload, "eventID")
	sessions := sessionStore.ByEventID(eventID)
	if len(sessions) == 0 {
		return
	}

	resp.Ack()
	for _, session := range sessions {
		session.Watch()
	}
	for _, session := range sessions {
		<-session.Watch()
	}
	ret := make([]interface{}, len(sessions))
	for i, session := range sessions {
		status, reason := session.GetStatus()
		ret[i] = map[string]interface{}{
			"name":        session.GetName(),
			"description": session.GetDescription(),
			"exported":    session.GetExported(),
			"event":       session.GetEventName(),
			"status":      status,
			"reason":      reason,
		}
	}
	resp.Return(map[string]interface{}{
		"sessions": ret,
	})
}

func handleEventList(resp *api.Response) {
	resp.Request = dipper.DeserializePayload(resp.Request)
	sessions := sessionStore.GetEvents()
	ret := make([]interface{}, len(sessions))
	for i, session := range sessions {
		ret[i] = map[string]interface{}{
			"name":        session.GetName(),
			"description": session.GetDescription(),
			"exported":    session.GetExported(),
			"eventID":     session.GetEventID(),
			"event":       session.GetEventName(),
			"startTime":   session.GetStartTime().Format(time.RFC3339),
		}
		if c := session.GetCompletionTime(); !c.IsZero() {
			ret[i].(map[string]interface{})["completionTime"] = c.Format(time.RFC3339)
		}
	}
	resp.Return(map[string]interface{}{
		"sessions": ret,
	})
}
