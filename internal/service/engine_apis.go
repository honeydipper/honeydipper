// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package service

import (
	"github.com/honeydipper/honeydipper/internal/api"
	"github.com/honeydipper/honeydipper/pkg/dipper"
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
		}
	}
	resp.Return(map[string]interface{}{
		"sessions": ret,
	})
}
