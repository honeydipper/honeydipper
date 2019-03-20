// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package service

import (
	"bytes"
	"testing"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/driver"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

type bytesBuffer struct {
	*bytes.Buffer
}

func (b bytesBuffer) Close() error {
	return nil
}

func TestExecuteWorkflow(t *testing.T) {
	wf := config.Workflow{
		Type: "function",
		Content: map[string]interface{}{
			"driver":    "test",
			"rawAction": "test",
		},
		Data: map[string]interface{}{
			"param1": "{{ empty .data.test }}",
		},
	}
	msg := &dipper.Message{
		Channel: "eventbus",
		Subject: "return",
		Labels: map[string]string{
			"status": "success",
		},
		Payload: nil,
	}
	parent := &WorkflowSession{
		step:  1,
		Type:  "pipe",
		event: map[string]interface{}{},
		work: []*config.Workflow{
			&config.Workflow{
				Type: "function",
			},
			&wf,
		},
	}
	sessions = map[string]*WorkflowSession{}
	dipper.IDMapMetadata = map[dipper.IDMap]*dipper.IDMapMeta{}
	dipper.InitIDMap(&sessions)

	var b = bytesBuffer{&bytes.Buffer{}}
	b.Grow(512)
	engine = &Service{
		name: "test",
		driverRuntimes: map[string]*driver.Runtime{
			"eventbus": &driver.Runtime{
				Output: b,
			},
		},
	}
	sessionID := dipper.IDMapPut(&sessions, parent)
	testFunc := func() {
		executeWorkflow(sessionID, &wf, msg)
	}
	assert.NotPanics(t, testFunc, "Should not panic when Payload is nil")
	assert.NotZero(t, b.Len(), "Should send message to eventbus")
}
