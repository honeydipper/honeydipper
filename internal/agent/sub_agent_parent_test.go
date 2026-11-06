// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartAgentCall_RegistersParentSessionInUnifiedConvo verifies the sub-agent's
// session ref in the parent's (unified) ConvoState carries ParentSessionID, so the
// parent's poll does not treat the running sub-agent as a superseding turn.
func TestStartAgentCall_RegistersParentSessionInUnifiedConvo(t *testing.T) {
	helper := newMemCacheHelper()
	helper.cfg.DataSet.Agents["sub"] = config.Agent{Name: "sub", Driver: "openai", Engine: "gpt-4"}
	store := &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      newCaptureLogger(&bytes.Buffer{}),
	}

	store.StartAgentCall(&dipper.Message{
		Labels: map[string]string{
			"sub_agent_name":   "sub",
			"agent_session_id": "parent-sess-1",
			"turn_id":          "1",
			"tool_call_id":     "0",
			"convo_id":         "parent-convo",
			"unified_convo_id": "parent-convo",
		},
		Payload: map[string]interface{}{
			"input":    "collect data",
			"convo_id": "sub-convo",
		},
	})
	store.Wait()

	csData, ok := helper.getCache(ConvoStateKeyPrefix + "parent-convo")
	require.True(t, ok, "sub-agent must register in the unified convo state")
	var cs ConvoState
	require.NoError(t, json.Unmarshal([]byte(csData), &cs))
	require.NotNil(t, cs.ActiveSession)
	assert.Equal(t, "parent-sess-1", cs.ActiveSession.ParentSessionID)
}
