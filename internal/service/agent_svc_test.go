// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"sync"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/agent"
	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// mockAgentStore – minimal AgentStore for agent_svc tests
// ---------------------------------------------------------------------------

type mockAgentStore struct {
	mu    sync.Mutex
	calls []string
}

func (m *mockAgentStore) record(s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, s)
}

func (m *mockAgentStore) hasCall(s string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.calls {
		if c == s {
			return true
		}
	}

	return false
}

func (m *mockAgentStore) StartInference(_ *dipper.Message) error {
	m.record("start")

	return nil
}
func (m *mockAgentStore) ContinueInference(_ *dipper.Message) { m.record("continue") }
func (m *mockAgentStore) ReceiveInference(_ *dipper.Message)  { m.record("receive") }
func (m *mockAgentStore) PollInference(_ *dipper.Message)     { m.record("poll") }
func (m *mockAgentStore) StartAgentCall(_ *dipper.Message)    { m.record("agent_call") }
func (m *mockAgentStore) StartMCPCall(_ *dipper.Message)      { m.record("mcp_call") }
func (m *mockAgentStore) CancelConvo(_ *dipper.Message)       { m.record("cancel_convo") }
func (m *mockAgentStore) StartTurn(_, _, _, _, _, _ string, _ bool) error {
	m.record("start_turn")

	return nil
}

func (m *mockAgentStore) StartNewConvo(_, _, _, _, _ string) string {
	m.record("start_new_convo")

	return ""
}
func (m *mockAgentStore) Stop() {}
func (m *mockAgentStore) Wait() {}

func (m *mockAgentStore) GetAgent(_ string) *config.Agent       { return &config.Agent{} }
func (m *mockAgentStore) GetSystem(_ string) *config.System     { return &config.System{} }
func (m *mockAgentStore) GetWorkflow(_ string) *config.Workflow { return &config.Workflow{} }
func (m *mockAgentStore) EmitMessage(_ dipper.Message)          {}
func (m *mockAgentStore) GetConfig() *config.Config             { return &config.Config{} }
func (m *mockAgentStore) GetLogger() *logging.Logger            { return nil }
func (m *mockAgentStore) GetUIURL() string                      { return "" }
func (m *mockAgentStore) GetName() string                       { return "mock-agent-store" }

func (m *mockAgentStore) Call(_, _ string, _ interface{}, _ ...string) ([]byte, error) {
	return nil, nil
}

func (m *mockAgentStore) CallNoWait(_, _ string, _ interface{}, _ ...string) error {
	return nil
}

func (m *mockAgentStore) CallRaw(_, _ string, _ []byte, _ ...string) ([]byte, error) {
	return nil, nil
}

func (m *mockAgentStore) CallRawNoWait(_, _ string, _ []byte, _ string, _ ...string) error {
	return nil
}

// ---------------------------------------------------------------------------
// AgentFeatures
// ---------------------------------------------------------------------------

func TestAgentFeatures_Empty(t *testing.T) {
	ds := &config.DataSet{
		Agents: map[string]config.Agent{},
	}
	features := AgentFeatures(ds)
	assert.Empty(t, features)
}

func TestAgentFeatures_WithDrivers(t *testing.T) {
	ds := &config.DataSet{
		Agents: map[string]config.Agent{
			"agent1": {Driver: "openai"},
			"agent2": {Driver: "anthropic"},
		},
	}
	features := AgentFeatures(ds)
	assert.Len(t, features, 2)
	assert.Contains(t, features, "driver:openai")
	assert.Contains(t, features, "driver:anthropic")
}

func TestAgentFeatures_SkipsAgentsWithoutDriver(t *testing.T) {
	ds := &config.DataSet{
		Agents: map[string]config.Agent{
			"with":    {Driver: "openai"},
			"without": {Driver: ""},
		},
	}
	features := AgentFeatures(ds)
	assert.Len(t, features, 1)
	assert.Contains(t, features, "driver:openai")
	assert.NotContains(t, features, "driver:")
}

// ---------------------------------------------------------------------------
// agentRoute
// ---------------------------------------------------------------------------

// readyAgentSvc builds a minimal *Service with its ready channel already closed.
func readyAgentSvc(runtimes map[string]*driver.Runtime) *Service {
	readyCh := make(chan struct{})
	close(readyCh)
	svc := &Service{
		name:           "agent",
		driverRuntimes: runtimes,
		ready:          readyCh,
	}

	return svc
}

func TestAgentRoute_EventbusWithDriver(t *testing.T) {
	rt := &driver.Runtime{
		Feature: dipper.ChannelEventbus,
		Handler: &driver.NullDriverHandler{},
		State:   driver.DriverAlive,
		Stream:  make(chan *dipper.Message, 1),
	}
	prev := agentSvc
	agentSvc = readyAgentSvc(map[string]*driver.Runtime{
		dipper.ChannelEventbus: rt,
	})
	t.Cleanup(func() { agentSvc = prev })

	msg := &dipper.Message{Channel: dipper.ChannelEventbus, Subject: "agent_command"}
	routed := agentRoute(msg)

	assert.Len(t, routed, 1)
	assert.Same(t, rt, routed[0].driverRuntime)
	assert.Same(t, msg, routed[0].message)
}

func TestAgentRoute_EventbusWithoutDriver(t *testing.T) {
	prev := agentSvc
	agentSvc = readyAgentSvc(map[string]*driver.Runtime{})
	t.Cleanup(func() { agentSvc = prev })

	msg := &dipper.Message{Channel: dipper.ChannelEventbus, Subject: "agent_command"}
	routed := agentRoute(msg)

	assert.Nil(t, routed)
}

func TestAgentRoute_NonEventbusNotRouted(t *testing.T) {
	rt := &driver.Runtime{
		Feature: ChannelAgentbus,
		Handler: &driver.NullDriverHandler{},
		State:   driver.DriverAlive,
		Stream:  make(chan *dipper.Message, 1),
	}
	prev := agentSvc
	agentSvc = readyAgentSvc(map[string]*driver.Runtime{
		ChannelAgentbus: rt,
	})
	t.Cleanup(func() { agentSvc = prev })

	msg := &dipper.Message{Channel: ChannelAgentbus, Subject: SubjectAgentbusReceive}
	routed := agentRoute(msg)

	assert.Nil(t, routed)
}

// ---------------------------------------------------------------------------
// handler dispatch tests
// ---------------------------------------------------------------------------

// setupHandlerGlobals installs a ready agentSvc and a mock agentStore, returning
// a cleanup function that restores the originals.
func setupHandlerGlobals(t *testing.T) *mockAgentStore {
	t.Helper()
	mock := &mockAgentStore{}
	prevSvc := agentSvc
	prevStore := agentStore
	agentSvc = readyAgentSvc(map[string]*driver.Runtime{})
	agentStore = mock
	t.Cleanup(func() {
		agentSvc = prevSvc
		agentStore = prevStore
	})

	return mock
}

func TestHandleAgentStart(t *testing.T) {
	mock := setupHandlerGlobals(t)
	msg := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: SubjectEventbusAgentStart,
		Labels:  map[string]string{"resume_key": "test-resume-key"},
	}
	handleAgentStart(nil, msg)
	assert.Eventually(t, func() bool { return mock.hasCall("start") }, 200*time.Millisecond, 5*time.Millisecond)
}

func TestHandleAgentContinue(t *testing.T) {
	mock := setupHandlerGlobals(t)
	msg := &dipper.Message{Channel: dipper.ChannelEventbus, Subject: SubjectEventbusAgentContinue}
	handleAgentContinue(nil, msg)
	assert.Eventually(t, func() bool { return mock.hasCall("continue") }, 200*time.Millisecond, 5*time.Millisecond)
}

func TestHandleAgentReceive(t *testing.T) {
	mock := setupHandlerGlobals(t)
	msg := &dipper.Message{Channel: ChannelAgentbus, Subject: SubjectAgentbusReceive}
	handleAgentReceive(nil, msg)
	assert.Eventually(t, func() bool { return mock.hasCall("receive") }, 200*time.Millisecond, 5*time.Millisecond)
}

func TestHandleAgentPoll(t *testing.T) {
	mock := setupHandlerGlobals(t)
	msg := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: SubjectEventbusAgentPoll,
		Labels:  map[string]string{"resume_key": "test-resume-key"},
	}
	handleAgentPoll(nil, msg)
	assert.Eventually(t, func() bool { return mock.hasCall("poll") }, 200*time.Millisecond, 5*time.Millisecond)
}

// TestHandleAgentStart_EvictedConvoNoAgent_EmitsErrorResponse verifies that
// when handleAgentStart receives a message for an evicted conversation with
// no agent_name label, it emits an agent_response with status=error and
// reason indicating conversation_expired.
func TestHandleAgentStart_EvictedConvoNoAgent_EmitsErrorResponse(t *testing.T) {
	mock := &errorCapturingMockAgentStore{}
	prevSvc := agentSvc
	prevStore := agentStore
	agentSvc = readyAgentSvc(map[string]*driver.Runtime{})
	agentStore = mock
	t.Cleanup(func() {
		agentSvc = prevSvc
		agentStore = prevStore
	})

	msg := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: SubjectEventbusAgentStart,
		Labels: map[string]string{
			"resume_key": "test-resume-key-error",
			"convo_id":   "evicted-convo",
			// No agent_name label
		},
		Payload: map[string]interface{}{
			"text": "hello",
		},
	}

	handleAgentStart(nil, msg)

	// Wait for the error response to be emitted
	assert.Eventually(t, func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		return len(mock.emitted) > 0
	}, 200*time.Millisecond, 5*time.Millisecond)

	// Find the agent_response with our resume_key
	var resp *dipper.Message
	mock.mu.Lock()
	for _, m := range mock.emitted {
		if m.Labels["resume_key"] == "test-resume-key-error" && m.Subject == "agent_response" {
			resp = m

			break
		}
	}
	mock.mu.Unlock()

	require.NotNil(t, resp, "agent_response must be emitted")
	require.Equal(t, "error", resp.Labels["status"], "status must be error")
	require.Contains(t, resp.Labels["reason"], "conversation expired", "reason must indicate conversation expired")
}

// TestHandleAgentStart_EvictedConvoWithAgent_Recovers verifies that when
// handleAgentStart receives a message for an evicted conversation WITH an
// agent_name label, it recovers successfully (no error response emitted,
// session starts normally).
func TestHandleAgentStart_EvictedConvoWithAgent_Recovers(t *testing.T) {
	mock := &recoveryCapturingMockAgentStore{}
	prevSvc := agentSvc
	prevStore := agentStore
	agentSvc = readyAgentSvc(map[string]*driver.Runtime{})
	agentStore = mock
	t.Cleanup(func() {
		agentSvc = prevSvc
		agentStore = prevStore
	})

	msg := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: SubjectEventbusAgentStart,
		Labels: map[string]string{
			"resume_key": "test-resume-key-recover",
			"convo_id":   "evicted-convo-recover",
			"agent_name": "recovery_agent",
		},
		Payload: map[string]interface{}{
			"text": "hello",
		},
	}

	handleAgentStart(nil, msg)

	// Wait for the success response to be emitted
	assert.Eventually(t, func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		return len(mock.emitted) > 0
	}, 200*time.Millisecond, 5*time.Millisecond)

	// Find the agent_response with our resume_key
	var resp *dipper.Message
	mock.mu.Lock()
	for _, m := range mock.emitted {
		if m.Labels["resume_key"] == "test-resume-key-recover" && m.Subject == "agent_response" {
			resp = m

			break
		}
	}
	mock.mu.Unlock()

	require.NotNil(t, resp, "agent_response must be emitted for recovery case")
	require.Equal(t, "success", resp.Labels["status"], "status must be success")
	require.NotEmpty(t, resp.Labels["agent_session_id"], "agent_session_id must be set")
	require.Empty(t, mock.errorResponses, "no error response should be emitted for recovery case")
}

// errorCapturingMockAgentStore is a mock that returns ErrConvoExpiredNoAgent
// from StartInference and captures emitted messages.
type errorCapturingMockAgentStore struct {
	mu             sync.Mutex
	emitted        []*dipper.Message
	errorResponses []*dipper.Message
}

func (m *errorCapturingMockAgentStore) StartInference(_ *dipper.Message) error {
	return agent.ErrConvoExpiredNoAgent
}

func (m *errorCapturingMockAgentStore) ContinueInference(_ *dipper.Message) {}

func (m *errorCapturingMockAgentStore) ReceiveInference(_ *dipper.Message) {}

func (m *errorCapturingMockAgentStore) PollInference(_ *dipper.Message) {}

func (m *errorCapturingMockAgentStore) StartAgentCall(_ *dipper.Message) {}

func (m *errorCapturingMockAgentStore) StartMCPCall(_ *dipper.Message) {}

func (m *errorCapturingMockAgentStore) CancelConvo(_ *dipper.Message) {}

func (m *errorCapturingMockAgentStore) StartTurn(_, _, _, _, _, _ string, _ bool) error {
	return nil
}

func (m *errorCapturingMockAgentStore) StartNewConvo(_, _, _, _, _ string) string {
	return ""
}

func (m *errorCapturingMockAgentStore) Stop() {}

func (m *errorCapturingMockAgentStore) Wait() {}

func (m *errorCapturingMockAgentStore) GetAgent(_ string) *config.Agent {
	return &config.Agent{}
}

func (m *errorCapturingMockAgentStore) GetSystem(_ string) *config.System {
	return &config.System{}
}

func (m *errorCapturingMockAgentStore) GetWorkflow(_ string) *config.Workflow {
	return &config.Workflow{}
}

func (m *errorCapturingMockAgentStore) EmitMessage(msg dipper.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitted = append(m.emitted, &msg)
	if m.isErrorResponse(msg) {
		m.errorResponses = append(m.errorResponses, &msg)
	}
}

func (m *errorCapturingMockAgentStore) GetConfig() *config.Config {
	return &config.Config{}
}

func (m *errorCapturingMockAgentStore) GetLogger() *logging.Logger {
	return nil
}

func (m *errorCapturingMockAgentStore) GetUIURL() string {
	return ""
}

func (m *errorCapturingMockAgentStore) GetName() string {
	return "error-capturing-mock-agent-store"
}

func (m *errorCapturingMockAgentStore) Call(_, _ string, _ interface{}, _ ...string) ([]byte, error) {
	return nil, nil
}

func (m *errorCapturingMockAgentStore) CallNoWait(_, _ string, _ interface{}, _ ...string) error {
	return nil
}

func (m *errorCapturingMockAgentStore) CallRaw(_, _ string, _ []byte, _ ...string) ([]byte, error) {
	return nil, nil
}

func (m *errorCapturingMockAgentStore) CallRawNoWait(_, _ string, _ []byte, _ string, _ ...string) error {
	return nil
}

func (m *errorCapturingMockAgentStore) isErrorResponse(msg dipper.Message) bool {
	return msg.Subject == "agent_response" && msg.Labels["status"] == "error"
}

// recoveryCapturingMockAgentStore simulates successful recovery.
type recoveryCapturingMockAgentStore struct {
	mu             sync.Mutex
	emitted        []*dipper.Message
	errorResponses []*dipper.Message
}

func (m *recoveryCapturingMockAgentStore) StartInference(msg *dipper.Message) error {
	// Simulate successful recovery by emitting a success response
	m.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "agent_response",
		Labels: map[string]string{
			"resume_key":       msg.Labels["resume_key"],
			"status":           "success",
			"agent_session_id": "sess-recovered-123",
		},
	})

	return nil
}

func (m *recoveryCapturingMockAgentStore) ContinueInference(_ *dipper.Message) {}

func (m *recoveryCapturingMockAgentStore) ReceiveInference(_ *dipper.Message) {}

func (m *recoveryCapturingMockAgentStore) PollInference(_ *dipper.Message) {}

func (m *recoveryCapturingMockAgentStore) StartAgentCall(_ *dipper.Message) {}

func (m *recoveryCapturingMockAgentStore) StartMCPCall(_ *dipper.Message) {}

func (m *recoveryCapturingMockAgentStore) CancelConvo(_ *dipper.Message) {}

func (m *recoveryCapturingMockAgentStore) StartTurn(_, _, _, _, _, _ string, _ bool) error {
	return nil
}

func (m *recoveryCapturingMockAgentStore) StartNewConvo(_, _, _, _, _ string) string {
	return ""
}

func (m *recoveryCapturingMockAgentStore) Stop() {}

func (m *recoveryCapturingMockAgentStore) Wait() {}

func (m *recoveryCapturingMockAgentStore) GetAgent(_ string) *config.Agent {
	return &config.Agent{}
}

func (m *recoveryCapturingMockAgentStore) GetSystem(_ string) *config.System {
	return &config.System{}
}

func (m *recoveryCapturingMockAgentStore) GetWorkflow(_ string) *config.Workflow {
	return &config.Workflow{}
}

func (m *recoveryCapturingMockAgentStore) EmitMessage(msg dipper.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitted = append(m.emitted, &msg)
	if m.isErrorResponse(msg) {
		m.errorResponses = append(m.errorResponses, &msg)
	}
}

func (m *recoveryCapturingMockAgentStore) GetConfig() *config.Config {
	return &config.Config{}
}

func (m *recoveryCapturingMockAgentStore) GetLogger() *logging.Logger {
	return nil
}

func (m *recoveryCapturingMockAgentStore) GetUIURL() string {
	return ""
}

func (m *recoveryCapturingMockAgentStore) GetName() string {
	return "recovery-capturing-mock-agent-store"
}

func (m *recoveryCapturingMockAgentStore) Call(_, _ string, _ interface{}, _ ...string) ([]byte, error) {
	return nil, nil
}

func (m *recoveryCapturingMockAgentStore) CallNoWait(_, _ string, _ interface{}, _ ...string) error {
	return nil
}

func (m *recoveryCapturingMockAgentStore) CallRaw(_, _ string, _ []byte, _ ...string) ([]byte, error) {
	return nil, nil
}

func (m *recoveryCapturingMockAgentStore) CallRawNoWait(_, _ string, _ []byte, _ string, _ ...string) error {
	return nil
}

func (m *recoveryCapturingMockAgentStore) isErrorResponse(msg dipper.Message) bool {
	return msg.Subject == "agent_response" && msg.Labels["status"] == "error"
}
