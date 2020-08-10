// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/honeydipper/honeydipper/internal/api"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

func setupReceiverAPIs() {
	receiver.APIs["eventAdd"] = handleEventAdd
}

func handleEventAdd(resp *api.Response) {
	defer func() {
		if r := recover(); r != nil {
			resp.ReturnError(r.(error))
		}
	}()
	resp.Request = dipper.DeserializePayload(resp.Request)
	body := dipper.MustGetMapDataStr(resp.Request.Payload, "body")
	contentType := resp.Request.Labels["content-type"]
	if !strings.HasPrefix(contentType, "application/json") {
		panic(fmt.Errorf("%w: content-type: %s", http.ErrNotSupported, contentType))
	}

	type simulatedEvent struct {
		Events []string
		Data   map[string]interface{}
	}

	se := simulatedEvent{}
	dipper.Must(json.Unmarshal([]byte(body), &se))

	eventID := dipper.NewUUID()
	msg := &dipper.Message{
		Channel: "eventbus",
		Subject: "message",
		Labels: map[string]string{
			"eventID": eventID,
		},
		Payload: map[string]interface{}{
			"events": se.Events,
			"data":   se.Data,
		},
	}

	eventBus := receiver.getDriverRuntime("eventbus")
	go eventBus.SendMessage(msg)

	resp.Return(map[string]interface{}{
		"eventID": eventID,
	})
}
