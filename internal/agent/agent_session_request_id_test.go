// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers for request_id correlation
// ---------------------------------------------------------------------------

func newTestAgent() *config.Agent {
	return &config.Agent{
		Name:   "a",
		Driver: "openai",
		Engine: "gpt-4",
	}
}

func newSessionWithRequestID(t *testing.T, store *mockStore, requestID string) *AgentSession {
	t.Helper()
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	return &AgentSession{
		store:            store,
		Agent:            agent,
		ID:               "test-session",
		CurrentRequestID: requestID,
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "test"},
		},
	}
}

// ---------------------------------------------------------------------------
// AgentSession.sendToDriver() request_id tests
// ---------------------------------------------------------------------------

func TestSendToDriver_GeneratesRequestID(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store: store,
		Agent: agent,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "test"},
		},
	}

	// Initially CurrentRequestID should be empty
	assert.Empty(t, s.CurrentRequestID)

	// Call sendToDriver
	s.sendToDriver()

	// CurrentRequestID should now be set
	assert.NotEmpty(t, s.CurrentRequestID)

	// Verify the request_id was passed to the driver
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	assert.Equal(t, s.CurrentRequestID, params["request_id"])
}

func TestSendToDriver_NewRequestIDEachCall(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store: store,
		Agent: agent,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "test"},
		},
	}

	// First call
	s.sendToDriver()
	firstRequestID := s.CurrentRequestID
	assert.NotEmpty(t, firstRequestID)

	// Second call
	s.sendToDriver()
	secondRequestID := s.CurrentRequestID
	assert.NotEmpty(t, secondRequestID)

	// Each call should generate a new request ID
	assert.NotEqual(t, firstRequestID, secondRequestID)
}

func TestSendToDriver_RequestIDInPayload(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store: store,
		Agent: agent,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "test"},
		},
	}

	s.sendToDriver()

	// Verify request_id is in the payload
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	assert.Contains(t, params, "request_id")
	assert.NotEmpty(t, params["request_id"])
}

// ---------------------------------------------------------------------------
// AgentSession.processAgentResponse() request_id validation tests
// ---------------------------------------------------------------------------

func TestProcessAgentResponse_AcceptsMatchingRequestID(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionWithRequestID(t, store, "test-uuid-123")

	// Create a response with matching request_id
	msg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": "test-uuid-123",
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "response",
				"is_complete": true,
			},
		},
	}

	s.processAgentResponse(msg)

	// Should process the response (no error)
	assert.Empty(t, s.ErrorReason)
	// History should contain the response
	require.Len(t, s.history, 1)
	assert.Equal(t, "response", s.history[0].Content)
}

func TestProcessAgentResponse_IgnoresMismatchedRequestID(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionWithRequestID(t, store, "test-uuid-123")

	// Create a response with different request_id
	msg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": "different-uuid-456",
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "response",
				"is_complete": true,
			},
		},
	}

	s.processAgentResponse(msg)

	// Should ignore the response (not process it)
	assert.Empty(t, s.history) // History should be empty
}

func TestProcessAgentResponse_AcceptsEmptyRequestID_LenientMode(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionWithRequestID(t, store, "test-uuid-123")

	// Create a response with no request_id (lenient mode for backward compatibility)
	msg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "response",
				"is_complete": true,
			},
		},
	}

	s.processAgentResponse(msg)

	// Should process the response (lenient mode)
	assert.Empty(t, s.ErrorReason)
	require.Len(t, s.history, 1)
	assert.Equal(t, "response", s.history[0].Content)
}

func TestProcessAgentResponse_IgnoresResponseWhenCurrentRequestIDEmpty(t *testing.T) {
	store := newMockStore(nil)
	// Session with empty CurrentRequestID (no active request)
	s := newSessionWithRequestID(t, store, "")

	// Create a response with a request_id
	msg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": "some-uuid",
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "response",
				"is_complete": true,
			},
		},
	}

	s.processAgentResponse(msg)

	// Should ignore the response because CurrentRequestID is empty
	// (no active request to match against)
	assert.Empty(t, s.history)
}

func TestProcessAgentResponse_ValidatesRequestIDBeforeProcessing(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionWithRequestID(t, store, "valid-uuid")

	// Create a response with mismatched request_id
	msg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": "invalid-uuid",
			"message": map[string]interface{}{
				"Role": RoleAgent,
				"ToolCalls": []interface{}{
					map[string]interface{}{
						"FuncName": "sys_test__func",
						"Params":   map[string]interface{}{},
					},
				},
				"is_complete": true,
			},
		},
	}

	s.processAgentResponse(msg)

	// Should not process the tool calls because request_id doesn't match
	assert.Empty(t, s.ToolCalls)
	assert.Empty(t, store.getEmitted())
}

// ---------------------------------------------------------------------------
// AgentSession.syncConvoStateStatus() request_id clearing tests
// ---------------------------------------------------------------------------

func TestSyncConvoStateStatus_ClearsRequestIDOnCompletion(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store:            store,
		Agent:            agent,
		ID:               "test-session",
		CurrentRequestID: "test-uuid-123",
		ConvoID:          "convo-1",
		history: []AgentMessage{
			{Role: RoleAgent, Content: "done", IsComplete: true},
		},
	}

	// Verify CurrentRequestID is set
	assert.Equal(t, "test-uuid-123", s.CurrentRequestID)

	// Call syncConvoStateStatus (which should clear CurrentRequestID)
	s.syncConvoStateStatus()

	// CurrentRequestID should be cleared
	assert.Empty(t, s.CurrentRequestID)
}

func TestSyncConvoStateStatus_ClearsRequestIDOnError(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store:            store,
		Agent:            agent,
		ID:               "test-session",
		CurrentRequestID: "test-uuid-123",
		ConvoID:          "convo-1",
		ErrorReason:      "some error",
	}

	// Verify CurrentRequestID is set
	assert.Equal(t, "test-uuid-123", s.CurrentRequestID)

	// Call syncConvoStateStatus (which should clear CurrentRequestID)
	s.syncConvoStateStatus()

	// CurrentRequestID should be cleared
	assert.Empty(t, s.CurrentRequestID)
}

func TestSyncConvoStateStatus_DoesNotClearRequestIDBeforeCompletion(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store:            store,
		Agent:            agent,
		ID:               "test-session",
		CurrentRequestID: "test-uuid-123",
		ConvoID:          "convo-1",
		history: []AgentMessage{
			{Role: RoleAgent, Content: "in progress"}, // Not complete
		},
	}

	// Verify CurrentRequestID is set
	assert.Equal(t, "test-uuid-123", s.CurrentRequestID)

	// Call syncConvoStateStatus (should return early, not clear CurrentRequestID)
	s.syncConvoStateStatus()

	// CurrentRequestID should NOT be cleared because turn is not complete
	assert.Equal(t, "test-uuid-123", s.CurrentRequestID)
}

// ---------------------------------------------------------------------------
// Integration tests: full flow with request_id
// ---------------------------------------------------------------------------

func TestFullFlow_RequestIDCorrelation(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store: store,
		Agent: agent,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "test"},
		},
	}

	// Step 1: Send to driver (generates request_id)
	s.sendToDriver()
	originalRequestID := s.CurrentRequestID
	assert.NotEmpty(t, originalRequestID)

	// Step 2: Simulate a response with matching request_id
	msg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": originalRequestID,
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "response",
				"is_complete": true,
			},
		},
	}
	s.processAgentResponse(msg)

	// Should process the response
	assert.Empty(t, s.ErrorReason)
	require.Len(t, s.history, 1)

	// Step 3: Simulate a new send_to_model (new request_id generated)
	s.sendToDriver()
	newRequestID := s.CurrentRequestID
	assert.NotEmpty(t, newRequestID)
	assert.NotEqual(t, originalRequestID, newRequestID)

	// Step 4: Old response should be ignored now
	oldMsg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": originalRequestID,
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "old response",
				"is_complete": true,
			},
		},
	}
	s.processAgentResponse(oldMsg)

	// History should still have only 1 message (old response ignored)
	assert.Len(t, s.history, 1)
	assert.Equal(t, "response", s.history[0].Content)
}

func TestFullFlow_CancelledRequestIgnored(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store: store,
		Agent: agent,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "test"},
		},
		ConvoID: "convo-1",
	}

	// Step 1: Send to driver (generates request_id)
	s.sendToDriver()
	requestID := s.CurrentRequestID

	// Step 2: Simulate cancellation (new request started, old one cancelled)
	// This happens when a new turn starts before the old one completes
	s.CurrentRequestID = "new-request-id"

	// Step 3: Old response arrives
	oldMsg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": requestID,
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "old response",
				"is_complete": true,
			},
		},
	}
	s.processAgentResponse(oldMsg)

	// Old response should be ignored
	assert.Empty(t, s.history)

	// Step 4: New response with correct request_id should be accepted
	newMsg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": "new-request-id",
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "new response",
				"is_complete": true,
			},
		},
	}
	s.processAgentResponse(newMsg)

	// New response should be processed
	require.Len(t, s.history, 1)
	assert.Equal(t, "new response", s.history[0].Content)
}

// ---------------------------------------------------------------------------
// Scenario tests: specific edge cases
// ---------------------------------------------------------------------------

func TestLateResponseAfterSessionComplete(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store:            store,
		Agent:            agent,
		ID:               "test-session",
		CurrentRequestID: "old-request-id",
		ConvoID:          "convo-1",
		history: []AgentMessage{
			{Role: RoleAgent, Content: "done", IsComplete: true},
		},
	}

	// Step 1: Session completes normally
	s.syncConvoStateStatus()
	
	// Verify CurrentRequestID is cleared
	assert.Empty(t, s.CurrentRequestID)
	
	// Step 2: Late response arrives with old request_id
	lateMsg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": "old-request-id",
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "late response",
				"is_complete": true,
			},
		},
	}
	s.processAgentResponse(lateMsg)
	
	// Late response should be IGNORED because CurrentRequestID is now empty
	// and the response has a non-empty request_id that doesn't match
	// The history should NOT have the late response
	for _, h := range s.history {
		assert.NotEqual(t, "late response", h.Content)
	}
}

func TestLateResponseAfterNewTurnStarted(t *testing.T) {
	store := newMockStore(nil)
	agent := newTestAgent()
	store.cfg.DataSet.Agents["a"] = *agent

	s := &AgentSession{
		store: store,
		Agent: agent,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "test"},
		},
	}

	// Step 1: First turn sends to driver
	s.sendToDriver()
	oldRequestID := s.CurrentRequestID
	
	// Step 2: First turn completes (CurrentRequestID cleared)
	s.history = append(s.history, AgentMessage{Role: RoleAgent, Content: "done", IsComplete: true})
	s.syncConvoStateStatus()
	
	// Verify CurrentRequestID is cleared
	assert.Empty(t, s.CurrentRequestID)
	
	// Step 3: New turn starts (new request_id generated)
	s.sendToDriver()
	newRequestID := s.CurrentRequestID
	assert.NotEqual(t, oldRequestID, newRequestID)
	
	// Step 4: Late response from old turn arrives
	lateMsg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"request_id": oldRequestID,
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"Content":     "late response from old turn",
				"is_complete": true,
			},
		},
	}
	s.processAgentResponse(lateMsg)
	
	// Late response should be IGNORED
	assert.Len(t, s.history, 1) // Only the "done" message, not the late response
	assert.Equal(t, "done", s.history[0].Content)
}
