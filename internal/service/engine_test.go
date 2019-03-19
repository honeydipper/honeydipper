// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package service

import (
	"testing"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestExecuteWorkflow(t *testing.T) {
	sessionID := "123"
	wf := config.Workflow{
		Type: "if",
		Content: []config.Workflow{
			{
				Type: "",
				Content: "noop",
			},
		},
		Condition: "false",
	}
	msg := &dipper.Message{
		Channel: "eventbus",
		Subject: "return",
		Labels: map[string]string{
			"status": "success",
		},
		Payload: nil,
	}
	parent := &WorkflowSession {
		work: []*config.Workflow {
			{
				Type: "pipe",
				Content: []config.Workflow{
					{
						Type: "function",
					},
					wf,
				},
			},
		},
	}
	sessions[sessionID] = parent
	testFunc := func() {
		executeWorkflow(sessionID, &wf, msg)
	}
	assert.NotPanics(t, testFunc, "Should not panic when Payload is nil")
}
