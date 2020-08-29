// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package workflow

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/workflow/mock_workflow"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestNewSessionStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelper := mock_workflow.NewMockSessionStoreHelper(ctrl)
	s := NewSessionStore(mockHelper)

	// tear down
	defer delete(dipper.IDMapMetadata, &s.sessions)

	assert.NotNil(t, s)
	assert.NotNil(t, s.sessions)
	assert.NotNil(t, s.suspendedSessions)
	assert.Contains(t, dipper.IDMapMetadata, &s.sessions)
	assert.Zero(t, s.Len())
}

func TestNewSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelper := mock_workflow.NewMockSessionStoreHelper(ctrl)
	s := NewSessionStore(mockHelper)

	// tear down
	defer delete(dipper.IDMapMetadata, &s.sessions)

	var testcase map[string]interface{}
	var session *Session
	var previous *Session

	testFunc := func() {
		session = s.newSession(testcase["parent"].(string), "uuid", testcase["workflow"].(*config.Workflow)).(*Session)
	}

	testCases := []map[string]interface{}{
		{ // test #1 -- create a workflow without parent and save
			"parent":   "",
			"workflow": &config.Workflow{},
			"asserts": func() {
				assert.Equal(t, "", session.ID)
				session.save()
				assert.Equal(t, "0", session.ID)
			},
		},
		{ // test #2 -- create a workflow with parent and save
			"parent": "0",
			"workflow": &config.Workflow{
				Workflow: "test",
			},
			"asserts": func() {
				assert.Equal(t, "test", session.performing)
				assert.Equal(t, "0", session.parent)
				session.save()
				assert.Equal(t, "1", session.ID)
			},
		},
	}

	for _, c := range testCases {
		testcase = c
		if shouldPanic, ok := c["panic"]; ok && shouldPanic.(bool) {
			assert.Panics(t, testFunc)
		} else {
			assert.NotPanics(t, testFunc)
			if assertFunc, ok := c["asserts"]; ok {
				assertFunc.(func())()
			}
		}
		previous = session
	}
	_ = previous
}
