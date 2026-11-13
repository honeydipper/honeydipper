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

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// mockAgentStore – minimal AgentStore for agent_svc tests
// ---------------------------------------------------------------------------

type mockAgentStore struct {
	mu    sync.Mutex
	calls []string

	// Optional config maps backing GetAgent/GetSystem/GetWorkflow so tests can
	// express found vs missing entities (nil maps => missing).
	agents    map[string]config.Agent
	systems   map[string]config.System
	workflows map[string]config.Workflow
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

func (m *mockAgentStore) StartInference(_ *dipper.Message) {
	m.record("start")
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

// GetAgent/GetSystem/GetWorkflow return nil (missing semantics) by default so
// this mock expresses "entity not found", matching the real store's nil-return
// behavior. Tests that need a found entity can set the corresponding field on
// the mock and have the getter consult it.
func (m *mockAgentStore) GetAgent(name string) *config.Agent {
	if m.agents == nil {
		return nil
	}
	a, ok := m.agents[name]
	if !ok {
		return nil
	}

	return &a
}

func (m *mockAgentStore) GetSystem(name string) *config.System {
	if m.systems == nil {
		return nil
	}
	s, ok := m.systems[name]
	if !ok {
		return nil
	}

	return &s
}

func (m *mockAgentStore) GetWorkflow(name string) *config.Workflow {
	if m.workflows == nil {
		return nil
	}
	w, ok := m.workflows[name]
	if !ok {
		return nil
	}

	return &w
}
func (m *mockAgentStore) EmitMessage(_ dipper.Message) {}
func (m *mockAgentStore) GetConfig() *config.Config    { return &config.Config{} }
func (m *mockAgentStore) GetLogger() *logging.Logger   { return nil }
func (m *mockAgentStore) GetUIURL() string             { return "" }
func (m *mockAgentStore) GetName() string              { return "mock-agent-store" }

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

// ---------------------------------------------------------------------------
// mockAgentStore nil semantics
// ---------------------------------------------------------------------------

// TestMockAgentStore_GetAgent_NilOnMissing verifies the mock expresses the real
// store's nil-return semantics for missing entities (and returns the entity
// when the backing map contains it).
func TestMockAgentStore_GetAgent_NilOnMissing(t *testing.T) {
	m := &mockAgentStore{}
	assert.Nil(t, m.GetAgent("anything"), "missing agent must return nil by default")

	m.agents = map[string]config.Agent{"bot": {Name: "bot"}}
	assert.NotNil(t, m.GetAgent("bot"), "found agent must be returned")
	assert.Nil(t, m.GetAgent("missing"), "missing agent must return nil even with a backing map")
}

func TestMockAgentStore_GetSystem_NilOnMissing(t *testing.T) {
	m := &mockAgentStore{}
	assert.Nil(t, m.GetSystem("anything"))

	m.systems = map[string]config.System{"s1": {}}
	assert.NotNil(t, m.GetSystem("s1"))
	assert.Nil(t, m.GetSystem("missing"))
}

func TestMockAgentStore_GetWorkflow_NilOnMissing(t *testing.T) {
	m := &mockAgentStore{}
	assert.Nil(t, m.GetWorkflow("anything"))

	m.workflows = map[string]config.Workflow{"wf1": {}}
	assert.NotNil(t, m.GetWorkflow("wf1"))
	assert.Nil(t, m.GetWorkflow("missing"))
}
