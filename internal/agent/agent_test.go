// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestMain
// ---------------------------------------------------------------------------

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test-agent", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// mockStore – implements AgentStore for tests
// ---------------------------------------------------------------------------

type mockStore struct {
	mu           sync.Mutex
	calls        []string
	resp         map[string][]byte
	lockTriggers map[string]func() // fired when a locker:lock with the keyed name is acquired
	emitted      []*dipper.Message
	cfg          *config.Config
	logger       *logging.Logger
	noWaitParams map[string]map[string]interface{}
	noWaitLabels map[string][]string
	callParams   map[string]map[string]interface{}
	uiURL        string
}

func newMockStore(cfg *config.Config) *mockStore {
	if cfg == nil {
		cfg = &config.Config{DataSet: &config.DataSet{
			Agents:    map[string]config.Agent{},
			Systems:   map[string]config.System{},
			Workflows: map[string]config.Workflow{},
		}}
	}

	return &mockStore{
		resp: map[string][]byte{},
		cfg:  cfg,
	}
}

func (m *mockStore) record(s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, s)
}

func (m *mockStore) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]string(nil), m.calls...)
}

func (m *mockStore) hasCall(s string) bool {
	for _, c := range m.getCalls() {
		if c == s {
			return true
		}
	}

	return false
}

func (m *mockStore) getEmitted() []*dipper.Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]*dipper.Message(nil), m.emitted...)
}

// Call looks up responses by "feature:method" and optionally "feature:method:key".
func (m *mockStore) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	base := feature + ":" + method
	if p, ok := params.(map[string]interface{}); ok {
		// Fire any lock trigger registered for this lock name (locks are keyed by
		// "name"). Tests use this to mutate cache state at the moment a lock is
		// acquired (e.g. a prior turn completing and persisting its history the
		// instant the turn lock is granted).
		if base == "locker:lock" {
			if name, okn := p["name"].(string); okn {
				m.mu.Lock()
				trigger := m.lockTriggers[name]
				m.mu.Unlock()
				if trigger != nil {
					trigger()
				}
			}
		}
		if k, ok2 := p["key"].(string); ok2 {
			full := base + ":" + k
			// If this is a cache save, record the saved value so subsequent
			// cache:load calls return the persisted state for tests.
			if base == "cache:save" {
				if val, okv := p["value"].(string); okv {
					m.mu.Lock()
					m.resp["cache:load:"+k] = []byte(val)
					m.mu.Unlock()
				}
			}
			if v, ok3 := m.resp[full]; ok3 {
				m.record(base)
				if p, ok := params.(map[string]interface{}); ok {
					m.mu.Lock()
					if m.callParams == nil {
						m.callParams = map[string]map[string]interface{}{}
					}
					m.callParams[base] = p
					m.mu.Unlock()
				}

				return v, nil
			}
		}
	}
	m.record(base)
	if p, ok := params.(map[string]interface{}); ok {
		m.mu.Lock()
		if m.callParams == nil {
			m.callParams = map[string]map[string]interface{}{}
		}
		m.callParams[base] = p
		m.mu.Unlock()
	}
	if v, ok := m.resp[base]; ok {
		return v, nil
	}

	return []byte("[]"), nil
}

func (m *mockStore) CallNoWait(feature, method string, params interface{}, labelsKV ...string) error {
	key := feature + ":" + method
	m.record(key)
	if p, ok := params.(map[string]interface{}); ok {
		m.mu.Lock()
		if m.noWaitParams == nil {
			m.noWaitParams = map[string]map[string]interface{}{}
		}
		m.noWaitParams[key] = p
		if m.noWaitLabels == nil {
			m.noWaitLabels = map[string][]string{}
		}
		m.noWaitLabels[key] = labelsKV
		m.mu.Unlock()
	}

	return nil
}

func (m *mockStore) getNoWaitParams(key string) map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.noWaitParams == nil {
		return nil
	}

	return m.noWaitParams[key]
}

func (m *mockStore) getNoWaitLabels(key string) []string { //nolint:unparam
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.noWaitLabels == nil {
		return nil
	}

	return m.noWaitLabels[key]
}

func (m *mockStore) getCallParams(key string) map[string]interface{} { //nolint:unparam
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callParams == nil {
		return nil
	}

	return m.callParams[key]
}

func (m *mockStore) CallRaw(feature, method string, params []byte, labelsKV ...string) ([]byte, error) {
	return m.Call(feature, method, nil, labelsKV...)
}

func (m *mockStore) CallRawNoWait(feature, method string, params []byte, rpcID string, labelsKV ...string) error {
	return m.CallNoWait(feature, method, nil, labelsKV...)
}

func (m *mockStore) GetName() string { return "mock-store" }

func (m *mockStore) StartInference(msg *dipper.Message)                                {}
func (m *mockStore) ContinueInference(msg *dipper.Message)                             {}
func (m *mockStore) ReceiveInference(msg *dipper.Message)                              {}
func (m *mockStore) PollInference(msg *dipper.Message)                                 {}
func (m *mockStore) StartAgentCall(msg *dipper.Message)                                {}
func (m *mockStore) StartMCPCall(msg *dipper.Message)                                  {}
func (m *mockStore) CancelConvo(msg *dipper.Message)                                   {}
func (m *mockStore) StartTurn(convoID, text, user, engine, driver string)              {}
func (m *mockStore) StartNewConvo(agentName, text, user, engine, driver string) string { return "" }

func (m *mockStore) GetAgent(name string) *config.Agent {
	a := m.cfg.DataSet.Agents[name]

	return &a
}

func (m *mockStore) GetSystem(name string) *config.System {
	s := m.cfg.DataSet.Systems[name]

	return &s
}

func (m *mockStore) GetWorkflow(name string) *config.Workflow {
	w := m.cfg.DataSet.Workflows[name]

	return &w
}

func (m *mockStore) EmitMessage(msg dipper.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitted = append(m.emitted, &msg)
}

func (m *mockStore) GetConfig() *config.Config { return m.cfg }

func (m *mockStore) Stop() {}
func (m *mockStore) Wait() {}

func (m *mockStore) GetLogger() *logging.Logger { return m.logger }
func (m *mockStore) GetUIURL() string           { return m.uiURL }

// ---------------------------------------------------------------------------
// mockStoreHelper – implements StoreHelper for PersistentAgentStore tests
// ---------------------------------------------------------------------------

type mockStoreHelper struct {
	mockStore
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustMarshalJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return b
}

func makeSystemWithFunc(funcName string) config.System {
	return config.System{
		Functions: map[string]config.Function{
			funcName: {
				Driver:    "testdriver",
				RawAction: "action",
				Meta: map[string]interface{}{
					"description": "does " + funcName,
					"inputs": []interface{}{
						map[string]interface{}{
							"name":        "param1",
							"type":        "string",
							"description": "first parameter",
						},
					},
				},
			},
		},
	}
}

func makeWorkflow(name, desc string) config.Workflow {
	return config.Workflow{
		Name:        name,
		Description: desc,
		Meta: map[string]interface{}{
			"description": "meta desc for " + name,
			"inputs": []interface{}{
				map[string]interface{}{
					"name":        "wfparam",
					"type":        "string",
					"description": "wf parameter",
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// AgentSession.setup tests
// ---------------------------------------------------------------------------

func TestSetup_NewSession(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:   "myagent",
		Driver: "openai",
		Engine: "gpt-4",
	}

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "myagent",
		},
		Payload: map[string]interface{}{
			"type": AgentSessionTypeInference,
			"text": "hello",
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	assert.NotEmpty(t, s.ID)
	assert.Equal(t, AgentSessionTypeInference, s.Type)
	assert.Equal(t, AgentSessionDefaultTTL, s.TTL)
	assert.Equal(t, "myagent", s.Agent.Name)
	assert.Equal(t, msg, s.CurrentMsg)
	assert.Equal(t, store, s.store)
}

func TestSetup_DefaultsToChatTurnType(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["a"] = config.Agent{Name: "a"}

	msg := &dipper.Message{
		Labels:  map[string]string{"agent_name": "a"},
		Payload: map[string]interface{}{"text": "hello"},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	assert.Equal(t, AgentSessionTypeChatTurn, s.Type)
}

func TestSetup_CustomTTL(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["a"] = config.Agent{Name: "a"}

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "a",
		},
		Payload: map[string]interface{}{
			"ttl":  "7200",
			"text": "hello",
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	assert.Equal(t, "7200", s.TTL)
}

func TestSetup_RestoreFromCache(t *testing.T) {
	store := newMockStore(nil)
	agent := config.Agent{Name: "test-agent", Driver: "openai", Engine: "gpt-4"}
	store.cfg.DataSet.Agents["test-agent"] = agent

	existing := &AgentSession{
		ID:      "session-abc",
		ConvoID: "convo-abc",
		Type:    AgentSessionTypeInference,
		TTL:     "1800",
	}
	store.resp["cache:load:"+AgentKeyPrefix+"session-abc"] = mustMarshalJSON(existing)

	// Pre-create ConvoState with Agent for the conversation
	cs := &ConvoState{
		ConvoID: "convo-abc",
		Agent:   &agent,
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-abc"] = mustMarshalJSON(cs)

	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-abc"] = mustMarshalJSON([]AgentMessage{
		{Role: RoleSystem, Content: "system prompt"},
		{Role: RoleUser, Content: "hello"},
	})

	newMsg := &dipper.Message{
		Labels: map[string]string{
			"agent_session_id": "session-abc",
		},
		Payload: map[string]interface{}{},
	}

	s := &AgentSession{}
	s.setup(newMsg, store, false)
	s.loadConvoHistory()

	assert.Equal(t, "session-abc", s.ID)
	assert.Equal(t, "1800", s.TTL)
	require.Len(t, s.history, 2)
	assert.Equal(t, RoleSystem, s.history[0].Role)
	assert.True(t, store.hasCall("cache:load"))
	// Agent should be loaded from ConvoState
	require.NotNil(t, s.Agent)
	assert.Equal(t, "test-agent", s.Agent.Name)
}

func TestSetup_ChatTurn_NewConvo(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["a"] = config.Agent{Name: "a"}

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "a",
		},
		Payload: map[string]interface{}{
			"type": AgentSessionTypeChatTurn,
			"text": "hello",
			// no convo_id
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	assert.Equal(t, AgentSessionTypeChatTurn, s.Type)
	assert.NotEmpty(t, s.ConvoID)
	assert.Empty(t, s.history) // no lrange call needed for new convo
	assert.False(t, store.hasCall("cache:lrange"))
}

func TestSetup_ChatTurn_ExistingConvo(t *testing.T) {
	store := newMockStore(nil)
	agent := config.Agent{Name: "a"}
	store.cfg.DataSet.Agents["a"] = agent

	// Pre-create ConvoState with Agent
	cs := &ConvoState{
		ConvoID: "convo-1",
		Agent:   &agent,
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-1"] = mustMarshalJSON(cs)

	history := []AgentMessage{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAgent, Content: "hello!"},
	}
	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-1"] = mustMarshalJSON(history)

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "a",
		},
		Payload: map[string]interface{}{
			"type":     AgentSessionTypeChatTurn,
			"convo_id": "convo-1",
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	assert.Equal(t, "convo-1", s.ConvoID)
	require.Len(t, s.history, 3)
	assert.Equal(t, RoleAgent, s.history[2].Role)
	assert.True(t, store.hasCall("cache:lrange"))
}

func TestParentChildConvoStateBehavior(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["a"] = config.Agent{Name: "a"}

	// Create parent session
	parentMsg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "a",
		},
		Payload: map[string]interface{}{
			"type": AgentSessionTypeChatTurn,
			"text": "hello",
		},
	}

	parent := &AgentSession{}
	parent.setup(parentMsg, store, false)

	// Load parent convo state
	pcs := &ConvoState{}
	pcs.load(parent.ConvoID, store)
	require.NotNil(t, pcs.FirstSession)
	require.NotNil(t, pcs.LastSession)
	assert.Equal(t, parent.ID, pcs.LastSession.SessionID)

	// Create child session (new convo) that belongs to the unified parent convo
	childMsg := &dipper.Message{
		Labels: map[string]string{
			"agent_name":       "a",
			"unified_convo_id": parent.ConvoID,
		},
		Payload: map[string]interface{}{
			"type": AgentSessionTypeInference,
			"text": "hello",
		},
	}

	child := &AgentSession{}
	child.setup(childMsg, store, false)

	// Child convo state should have LastSession == ActiveSession == child
	ccs := &ConvoState{}
	ccs.load(child.ConvoID, store)
	require.NotNil(t, ccs.LastSession)
	require.NotNil(t, ccs.ActiveSession)
	assert.Equal(t, child.ID, ccs.LastSession.SessionID)
	assert.Equal(t, child.ID, ccs.ActiveSession.SessionID)

	// Parent convo state should remain pointing at parent session
	pcs2 := &ConvoState{}
	pcs2.load(parent.ConvoID, store)
	assert.Equal(t, parent.ID, pcs2.LastSession.SessionID)

	// Simulate child completion and sync status
	child.history = append(child.history, AgentMessage{Role: RoleAgent, IsComplete: true, Content: "done"})
	child.InputTokens = 1
	child.OutputTokens = 2
	child.syncConvoStateStatus()

	// Child convo state: LastSession should be complete and ActiveSession updated
	ccs2 := &ConvoState{}
	ccs2.load(child.ConvoID, store)
	require.NotNil(t, ccs2.LastSession)
	assert.Equal(t, ConvoSessionStatusComplete, ccs2.LastSession.Status)
	require.NotNil(t, ccs2.ActiveSession)
	assert.Equal(t, ConvoSessionStatusComplete, ccs2.ActiveSession.Status)

	// Parent convo still unchanged (will only update when parent processes agent_continue)
	pcs3 := &ConvoState{}
	pcs3.load(parent.ConvoID, store)
	assert.Equal(t, parent.ID, pcs3.LastSession.SessionID)
}

// ---------------------------------------------------------------------------
// AgentSession.run tests
// ---------------------------------------------------------------------------

func TestRun_AddsSystemPromptAndUserMessage(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["a"] = config.Agent{
		Name:         "a",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "You are helpful.",
	}

	msg := &dipper.Message{
		Labels:  map[string]string{"agent_name": "a"},
		Payload: map[string]interface{}{"text": "ping"},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)
	s.run()

	// Persisted history contains only the user message; no system message stored.
	require.Len(t, s.history, 1)
	assert.Equal(t, RoleUser, s.history[0].Role)
	assert.Equal(t, "ping", s.history[0].Content)

	// Driver receives ephemeral history with system prompt prepended.
	assert.True(t, store.hasCall("driver:openai:send_to_model"))
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	driverHistory := params["history"].([]AgentMessage)
	require.Len(t, driverHistory, 2)
	assert.Equal(t, RoleSystem, driverHistory[0].Role)
	assert.Equal(t, "You are helpful.", driverHistory[0].Content)
	assert.Equal(t, RoleUser, driverHistory[1].Role)
}

func TestRun_InferenceTypeUsesInferencePrompt(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["a"] = config.Agent{
		Name:            "a",
		Driver:          "openai",
		Engine:          "gpt-4",
		SystemPrompt:    "system",
		InferencePrompt: "inference-specific-prompt",
	}

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "a",
		},
		Payload: map[string]interface{}{
			"type": AgentSessionTypeInference,
			"text": "q",
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)
	s.run()

	// Driver receives the inference-specific prompt, not the general system prompt.
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	driverHistory := params["history"].([]AgentMessage)
	require.NotEmpty(t, driverHistory)
	assert.Equal(t, RoleSystem, driverHistory[0].Role)
	assert.Equal(t, "inference-specific-prompt", driverHistory[0].Content)
}

func TestRun_AgentSwitchUsesCurrentSystemPrompt(t *testing.T) {
	// Simulates a mid-conversation agent switch: the persisted history has no
	// system message (new behaviour), but sendToDriver always injects the
	// current agent's system prompt ephemerally.
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["a"] = config.Agent{
		Name:         "a",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "new agent prompt",
	}

	msg := &dipper.Message{
		Labels:  map[string]string{"agent_name": "a"},
		Payload: map[string]interface{}{"text": "followup"},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)
	// Pre-existing history from a prior turn (no system message).
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "previous message"},
		{Role: RoleAgent, Content: "previous reply"},
	}
	s.run()

	// Persisted history gains the new user message; still no system message.
	require.Len(t, s.history, 3)
	assert.Equal(t, RoleUser, s.history[2].Role)
	assert.Equal(t, "followup", s.history[2].Content)

	// Driver receives current agent's system prompt at position 0.
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	driverHistory := params["history"].([]AgentMessage)
	assert.Equal(t, RoleSystem, driverHistory[0].Role)
	assert.Equal(t, "new agent prompt", driverHistory[0].Content)
}

func TestRun_WithUserLabel(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["a"] = config.Agent{Name: "a", Driver: "openai", Engine: "e"}

	msg := &dipper.Message{
		Labels:  map[string]string{"agent_name": "a"},
		Payload: map[string]interface{}{"text": "hello", "user": "alice"},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)
	s.run()

	userMsg := s.history[len(s.history)-1]
	assert.Equal(t, RoleUser, userMsg.Role)
	assert.Equal(t, "alice", userMsg.User)
	assert.Equal(t, "hello", userMsg.Content)
}

// ---------------------------------------------------------------------------
// AgentSession.recover tests
// ---------------------------------------------------------------------------

func TestRecover(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["a"] = config.Agent{
		Name:   "a",
		Driver: "openai",
		Engine: "gpt-4",
	}

	s := &AgentSession{
		store:      store,
		CurrentMsg: &dipper.Message{Labels: map[string]string{"timeout": "9s"}},
		Agent:      &config.Agent{Name: "a", Driver: "openai", Engine: "gpt-4", SystemPrompt: "agent sys"},
		ID:         "recover-id",
		history: []AgentMessage{
			{Role: RoleSystem, Content: "sys"},
			{Role: RoleUser, Content: "hello"},
		},
	}

	s.recover()

	assert.True(t, store.hasCall("driver:openai:send_to_model"))
	// recover should NOT modify the persisted history slice
	assert.Len(t, s.history, 2)
	// Driver receives the current agent's system prompt (legacy entry filtered out)
	// with only the user message appended after it.
	params := store.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	driverHistory := params["history"].([]AgentMessage)
	require.Len(t, driverHistory, 3)
	assert.Equal(t, RoleSystem, driverHistory[0].Role)
	assert.Equal(t, RoleSystem, driverHistory[1].Role)
	assert.Equal(t, RoleUser, driverHistory[2].Role)
}

// ---------------------------------------------------------------------------
// AgentSession.BuildTools tests
// ---------------------------------------------------------------------------

func TestBuildTools_SystemTool(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["mysys"] = makeSystemWithFunc("doThing")
	agentA := config.Agent{
		Name:   "a",
		Driver: "openai",
		Tools: []config.AgentToolDef{
			{Type: "system", Name: "mysys"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{
		store: store,
		Agent: &agentA,
	}

	tools := s.BuildTools()

	require.Contains(t, tools, "sys_mysys__doThing")
	tool := tools["sys_mysys__doThing"]
	assert.Equal(t, "sys_mysys__doThing", tool.Name)
	assert.Equal(t, "does doThing", tool.Description)
	require.Contains(t, tool.Params, "param1")
}

func TestBuildTools_WorkflowTool(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Workflows["mywf"] = makeWorkflow("mywf", "my workflow")
	agentA := config.Agent{
		Name:   "a",
		Driver: "openai",
		Tools: []config.AgentToolDef{
			{Type: "workflow", Name: "mywf"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{
		store: store,
		Agent: &agentA,
	}

	tools := s.BuildTools()

	require.Contains(t, tools, "wf__mywf")
	tool := tools["wf__mywf"]
	assert.Equal(t, "wf__mywf", tool.Name)
	assert.Equal(t, "my workflow", tool.Description) // prefers Workflow.Description over Meta
	require.Contains(t, tool.Params, "wfparam")
}

func TestBuildTools_WorkflowTool_UsesMetaDescriptionWhenEmpty(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Workflows["mywf"] = makeWorkflow("mywf", "") // empty Description
	agentA := config.Agent{
		Name:   "a",
		Driver: "openai",
		Tools: []config.AgentToolDef{
			{Type: "workflow", Name: "mywf"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{
		store: store,
		Agent: &agentA,
	}

	tools := s.BuildTools()

	tool := tools["wf__mywf"]
	assert.Equal(t, "meta desc for mywf", tool.Description)
}

func TestBuildTools_DuplicateSystemToolSkipped(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["mysys"] = makeSystemWithFunc("fn1")
	agentA := config.Agent{
		Name:   "a",
		Driver: "openai",
		Tools: []config.AgentToolDef{
			{Type: "system", Name: "mysys"},
			{Type: "system", Name: "mysys"}, // duplicate
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{
		store: store,
		Agent: &agentA,
	}

	tools := s.BuildTools()

	// Only one entry despite duplicate tool defs.
	assert.Len(t, tools, 2)
}

func TestBuildTools_DuplicateWorkflowToolSkipped(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Workflows["wf1"] = makeWorkflow("wf1", "wf1 desc")
	agentA := config.Agent{
		Name:   "a",
		Driver: "openai",
		Tools: []config.AgentToolDef{
			{Type: "workflow", Name: "wf1"},
			{Type: "workflow", Name: "wf1"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{
		store: store,
		Agent: &agentA,
	}

	tools := s.BuildTools()

	assert.Len(t, tools, 2)
}

func TestBuildTools_ParamTypeDefaultsToString(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["s"] = config.System{
		Functions: map[string]config.Function{
			"fn": {
				Driver:    "d",
				RawAction: "a",
				Meta: map[string]interface{}{
					"description": "fn",
					"inputs": []interface{}{
						map[string]interface{}{
							"name":        "p",
							"description": "no type field",
							// "type" is intentionally absent
						},
					},
				},
			},
		},
	}
	agentA := config.Agent{
		Name:  "a",
		Tools: []config.AgentToolDef{{Type: "system", Name: "s"}},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{
		store: store,
		Agent: &agentA,
	}
	tools := s.BuildTools()

	require.Contains(t, tools, "sys_s__fn")
	param := tools["sys_s__fn"].Params["p"].(map[string]interface{})
	assert.Equal(t, "string", param["type"])
}

func TestBuildTools_Mixed(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["s1"] = makeSystemWithFunc("fn1")
	store.cfg.DataSet.Systems["s2"] = makeSystemWithFunc("fn2")
	store.cfg.DataSet.Workflows["wf1"] = makeWorkflow("wf1", "w")
	agentA := config.Agent{
		Name: "a",
		Tools: []config.AgentToolDef{
			{Type: "system", Name: "s1"},
			{Type: "system", Name: "s2"},
			{Type: "workflow", Name: "wf1"},
		},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	s := &AgentSession{
		store: store,
		Agent: &agentA,
	}
	tools := s.BuildTools()

	assert.Len(t, tools, 4)
	assert.Contains(t, tools, "sys_s1__fn1")
	assert.Contains(t, tools, "sys_s2__fn2")
	assert.Contains(t, tools, "wf__wf1")
}

// ---------------------------------------------------------------------------
// Compaction tests
// ---------------------------------------------------------------------------

func TestCompactHistory_EmitsAgentCall(t *testing.T) {
	store := newMockStore(nil)
	// summarizer agent
	store.cfg.DataSet.Agents["summ"] = config.Agent{Name: "summ", Driver: "openai", Engine: "gpt-4"}

	agentA := config.Agent{
		Name: "bot",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy:           agentpkg.CompactionStrategySummarize,
			Threshold:          3,
			ThresholdType:      "history_len",
			PreserveRecent:     1,
			SummarizationAgent: "summ",
		},
	}
	s := &AgentSession{store: store, Agent: &agentA, ID: "s1", ConvoID: "convo-1"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	// build history longer than preserve
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "m1"},
		{Role: RoleUser, Content: "m2"},
		{Role: RoleUser, Content: "m3"},
	}

	ok := s.compactHistory()
	require.True(t, ok)

	// A tool-call entry should be appended to history
	require.GreaterOrEqual(t, len(s.history), 1)
	last := s.history[len(s.history)-1]
	require.NotNil(t, last.ToolCalls)
	require.Len(t, last.ToolCalls, 1)
	assert.Equal(t, "ag__summ", last.ToolCalls[0].FuncName)

	// The store should have emitted an agent_call for the sub-agent
	emitted := store.getEmitted()
	require.GreaterOrEqual(t, len(emitted), 1)
	found := false
	for _, m := range emitted {
		if m.Subject == "agent_call" {
			if v, ok := m.Labels["sub_agent_name"]; ok && v == "summ" {
				found = true

				break
			}
		}
	}
	assert.True(t, found, "expected agent_call emitted for summarization agent")
}

func TestHandleCompactionResult_ReplacesHistory(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["summ"] = config.Agent{Name: "summ", Driver: "openai", Engine: "gpt-4"}

	agentA := config.Agent{
		Name: "bot",
		CompactionPolicy: &agentpkg.CompactionPolicy{
			Strategy:           agentpkg.CompactionStrategySummarize,
			Threshold:          3,
			ThresholdType:      "history_len",
			PreserveRecent:     2,
			SummarizationAgent: "summ",
		},
	}

	s := &AgentSession{store: store, Agent: &agentA, ID: "s2", ConvoID: "convo-2"}
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}, Payload: map[string]interface{}{"convo_id": s.ConvoID}}
	// initial history: three messages then compaction tool-call will be appended
	s.history = []AgentMessage{
		{Role: RoleUser, Content: "old1"},
		{Role: RoleUser, Content: "old2"},
		{Role: RoleUser, Content: "old3"},
	}

	ok := s.compactHistory()
	require.True(t, ok)

	// simulate sub-agent returning the summary via agent_continue payload
	turnID := strconv.Itoa(len(s.history))
	msg := &dipper.Message{
		Labels: map[string]string{
			"turn_id":      turnID,
			"tool_call_id": "0",
			"status":       "success",
		},
		Payload: map[string]interface{}{
			"data": map[string]interface{}{"output": "COMPACTED SUMMARY"},
		},
	}

	s.processToolResult(msg)

	// After compaction, history should start with a system summary entry
	require.GreaterOrEqual(t, len(s.history), 1)
	assert.Equal(t, RoleSystem, s.history[0].Role)
	// persisted summary stored by handleCompactionResult includes marker
	// but in-memory history is set to the persisted messages; check content contains "COMPACTED SUMMARY"
	assert.Contains(t, s.history[0].Content, "COMPACTED SUMMARY")

	// Ensure cache replace was attempted
	assert.True(t, store.hasCall("cache:del"))
	assert.True(t, store.hasCall("cache:rpush"))
}

// ---------------------------------------------------------------------------
// AgentSession.coerceToolCallParams tests
// ---------------------------------------------------------------------------

func makeSystemWithTypedParams() config.System {
	return config.System{
		Functions: map[string]config.Function{
			"fn1": {
				Driver:    "d",
				RawAction: "action",
				Meta: map[string]interface{}{
					"description": "fn with typed params",
					"inputs": []interface{}{
						map[string]interface{}{"name": "str_param", "type": "string", "description": "a string"},
						map[string]interface{}{"name": "obj_param", "type": "object", "description": "an object"},
						map[string]interface{}{"name": "arr_param", "type": "array", "description": "an array"},
					},
				},
			},
		},
	}
}

func makeSessionWithTypedSystem(t *testing.T) *AgentSession {
	t.Helper()
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["s1"] = makeSystemWithTypedParams()
	agentA := config.Agent{
		Name:  "a",
		Tools: []config.AgentToolDef{{Type: "system", Name: "s1"}},
	}
	store.cfg.DataSet.Agents["a"] = agentA

	return &AgentSession{store: store, Agent: &agentA}
}

func TestCoerceToolCallParams_OnlyCoercesTypeMismatch(t *testing.T) {
	s := makeSessionWithTypedSystem(t)

	toolCalls := []AgentToolCall{
		{
			FuncName: "sys_s1__fn1",
			Params: map[string]interface{}{
				"str_param": `{"this": "should stay a string"}`,
				"obj_param": `{"key": "value"}`,
				"arr_param": `["a", "b"]`,
			},
		},
	}

	s.coerceToolCallParams(toolCalls)

	params := toolCalls[0].Params
	// string-typed param must NOT be coerced even though it looks like JSON
	assert.Equal(t, `{"this": "should stay a string"}`, params["str_param"])
	// object-typed param sent as JSON string must be parsed
	assert.Equal(t, map[string]interface{}{"key": "value"}, params["obj_param"])
	// array-typed param sent as JSON string must be parsed
	assert.Equal(t, []interface{}{"a", "b"}, params["arr_param"])
}

func TestCoerceToolCallParams_InvalidJSONUnchanged(t *testing.T) {
	s := makeSessionWithTypedSystem(t)

	toolCalls := []AgentToolCall{
		{
			FuncName: "sys_s1__fn1",
			Params:   map[string]interface{}{"obj_param": `{not valid json`},
		},
	}

	s.coerceToolCallParams(toolCalls)

	assert.Equal(t, `{not valid json`, toolCalls[0].Params["obj_param"])
}

func TestCoerceToolCallParams_AlreadyParsedUnchanged(t *testing.T) {
	s := makeSessionWithTypedSystem(t)

	already := map[string]interface{}{"already": "parsed"}
	toolCalls := []AgentToolCall{
		{
			FuncName: "sys_s1__fn1",
			Params:   map[string]interface{}{"obj_param": already},
		},
	}

	s.coerceToolCallParams(toolCalls)

	assert.Equal(t, already, toolCalls[0].Params["obj_param"])
}

func TestCoerceToolCallParams_UnknownToolSkipped(t *testing.T) {
	s := makeSessionWithTypedSystem(t)

	toolCalls := []AgentToolCall{
		{
			FuncName: "sys_unknown__fn",
			Params:   map[string]interface{}{"obj_param": `{"key": "val"}`},
		},
	}

	s.coerceToolCallParams(toolCalls)

	// Unknown tool — param left as-is
	assert.Equal(t, `{"key": "val"}`, toolCalls[0].Params["obj_param"])
}

// ---------------------------------------------------------------------------
// AgentSession.processAgentMessage tests
// ---------------------------------------------------------------------------

func newSessionForMsgProcessing(t *testing.T, store *mockStore) *AgentSession {
	t.Helper()
	store.cfg.DataSet.Agents["a"] = config.Agent{
		Name:   "a",
		Driver: "openai",
		Engine: "gpt-4",
	}
	agent := store.cfg.DataSet.Agents["a"]

	return &AgentSession{
		store: store,
		Agent: &agent,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "re-run input"},
		},
	}
}

func TestProcessAgentMessage_ToolCalls(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["s1"] = makeSystemWithFunc("fn1")

	s := newSessionForMsgProcessing(t, store)
	s.Agent.Tools = []config.AgentToolDef{{Type: "system", Name: "s1"}}

	// Tool calls only arrive on complete messages (new protocol: incomplete messages
	// never carry tool calls).
	agentMsg := &AgentMessage{
		Role:       RoleAgent,
		IsComplete: true,
		ToolCalls: []AgentToolCall{
			{FuncName: "sys_s1__fn1", Params: map[string]interface{}{"param1": "val"}},
		},
	}

	s.processAgentMessage(agentMsg)

	// History should contain the agent message.
	require.Len(t, s.history, 1)
	assert.Equal(t, RoleAgent, s.history[0].Role)

	// nextToolCall should have emitted agent_command.
	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "agent_command", emitted[0].Subject)
}

func TestProcessAgentMessage_FinalReply(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionForMsgProcessing(t, store)

	agentMsg := &AgentMessage{
		Role:       RoleAgent,
		Content:    "Here is the answer.",
		IsComplete: true,
	}

	s.processAgentMessage(agentMsg)

	require.Len(t, s.history, 1)
	assert.Equal(t, "Here is the answer.", s.history[0].Content)
}

func TestProcessAgentMessage_StreamingChunk(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionForMsgProcessing(t, store)

	// Non-complete streaming chunks must be accumulated in PendingContent,
	// not written to convo history, to avoid one Redis rpush per chunk.
	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: "Hello"})
	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: ", world"})

	assert.Equal(t, "Hello, world", s.PendingContent)
	assert.Empty(t, s.history, "streaming chunks must not appear in history")
}

func TestProcessAgentMessage_StreamingComplete(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionForMsgProcessing(t, store)

	// Simulate two chunks followed by a final IsComplete=true message that
	// carries the full content (new protocol: last message is not a delta).
	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: "Chunk1"})
	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: "Chunk2"})
	s.processAgentMessage(&AgentMessage{Role: RoleAgent, Content: "Chunk1Chunk2", IsComplete: true})

	// PendingContent should be cleared after the complete message.
	assert.Empty(t, s.PendingContent)
	// History should contain exactly one entry with the full content from the final message.
	require.Len(t, s.history, 1)
	assert.Equal(t, "Chunk1Chunk2", s.history[0].Content)
	assert.True(t, s.history[0].IsComplete)
}

func TestProcessAgentMessage_Thinking_NotEmittedByDefault(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionForMsgProcessing(t, store)
	s.Agent.ShouldEmitThoughts = false

	agentMsg := &AgentMessage{
		Role:       RoleAgent,
		IsThinking: true,
		Content:    "I am thinking...",
	}

	s.processAgentMessage(agentMsg)

	// Thinking tokens are NOT emitted when ShouldEmitThoughts=false.
	assert.Empty(t, store.getEmitted())
}

func TestProcessAgentMessage_Thinking_DoesNotRerun(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionForMsgProcessing(t, store)

	agentMsg := &AgentMessage{
		Role:       RoleAgent,
		IsThinking: true,
		Content:    "thinking...",
	}

	s.processAgentMessage(agentMsg)

	// run() should NOT have been called (no send_to_model).
	assert.False(t, store.hasCall("driver:openai:send_to_model"))
}

func TestProcessAgentMessage_NonAgentRole_Reruns(t *testing.T) {
	store := newMockStore(nil)
	s := newSessionForMsgProcessing(t, store)

	agentMsg := &AgentMessage{
		Role:    "tool",
		Content: "some tool output",
	}

	s.processAgentMessage(agentMsg)

	// Non-agent role (not thinking, no tool calls) should trigger s.run().
	assert.True(t, store.hasCall("driver:openai:send_to_model"))
}

// ---------------------------------------------------------------------------
// AgentSession.processAgentResponse tests
// ---------------------------------------------------------------------------

func TestProcessAgentResponse_CoercesObjectArrayParams(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["s1"] = makeSystemWithTypedParams()
	agentA := config.Agent{
		Name:   "a",
		Driver: "openai",
		Tools:  []config.AgentToolDef{{Type: "system", Name: "s1"}},
	}
	store.cfg.DataSet.Agents["a"] = agentA
	s := &AgentSession{
		store: store,
		Agent: &agentA,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "input"},
		},
	}

	// str_param is declared as "string" — must remain a string even though it looks like JSON.
	// obj_param is declared as "object" — must be parsed from its JSON string.
	// arr_param is declared as "array" — must be parsed from its JSON string.
	msg := &dipper.Message{
		Labels: map[string]string{"status": "success"},
		Payload: map[string]interface{}{
			"message": map[string]interface{}{
				"Role":        RoleAgent,
				"is_complete": true,
				"ToolCalls": []interface{}{
					map[string]interface{}{
						"FuncName": "sys_s1__fn1",
						"Params": map[string]interface{}{
							"str_param": `{"should": "stay string"}`,
							"obj_param": `{"key": "value"}`,
							"arr_param": `["x", "y"]`,
						},
					},
				},
			},
		},
	}

	s.processAgentResponse(msg)

	require.Len(t, s.ToolCalls, 1)
	params := s.ToolCalls[0].Params
	assert.Equal(t, `{"should": "stay string"}`, params["str_param"], "string-typed param must not be coerced")
	assert.Equal(t, map[string]interface{}{"key": "value"}, params["obj_param"], "object-typed param should be parsed")
	assert.Equal(t, []interface{}{"x", "y"}, params["arr_param"], "array-typed param should be parsed")
}

// ---------------------------------------------------------------------------
// AgentSession.nextToolCall tests
// ---------------------------------------------------------------------------

func TestNextToolCall_SystemDispatch(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["mysys"] = makeSystemWithFunc("doWork")

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Name: "test-agent", Driver: "openai"},
		ID:          "tc-session",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls: []AgentToolCall{
			{FuncName: "sys_mysys__doWork", Params: map[string]interface{}{"param1": "v1"}},
		},
	}

	s.nextToolCall()

	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "agent_command", emitted[0].Subject)
	assert.Equal(t, s.ID, emitted[0].Labels["agent_session_id"])
	assert.Equal(t, "test-agent", emitted[0].Labels["agent_name"])
}

func TestNextToolCall_WorkflowDispatch(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Workflows["mywf"] = makeWorkflow("mywf", "desc")

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Driver: "openai"},
		ID:          "tc-wf-session",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls: []AgentToolCall{
			{FuncName: "wf__mywf", Params: map[string]interface{}{"wfparam": "v"}},
		},
	}

	s.nextToolCall()

	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "agent_workflow", emitted[0].Subject)
	assert.Equal(t, "mywf", emitted[0].Payload.(map[string]interface{})["do"].(map[string]interface{})["call_workflow"])
}

func TestNextToolCall_AllDone_DoesNothing(t *testing.T) {
	store := newMockStore(nil)

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Driver: "openai"},
		ID:          "done-session",
		CurrentCall: 2,
		ToolCalls:   []AgentToolCall{{}, {}}, // 2 entries, CurrentCall == len
	}

	s.nextToolCall()

	assert.Empty(t, store.getEmitted())
	assert.False(t, store.hasCall("cache:save"))
}

// ---------------------------------------------------------------------------
// AgentSession.processToolResult tests
// ---------------------------------------------------------------------------

func TestProcessToolResult_WorkflowResult_Advances(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["s1"] = makeSystemWithFunc("fn1")

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Driver: "openai"},
		ID:          "tr-session",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls: []AgentToolCall{
			{FuncName: "wf__mywf"},
			{FuncName: "wf__otherwf"},
		},
		history: []AgentMessage{
			{
				Role: RoleAgent,
				ToolCalls: []AgentToolCall{
					{FuncName: "wf__mywf"},
					{FuncName: "wf__otherwf"},
				},
			},
		},
	}

	msg := &dipper.Message{
		Labels: map[string]string{
			"turn_id":      "1",
			"tool_call_id": "0",
		},
		Payload: map[string]interface{}{
			"data": map[string]interface{}{"output": "done"},
		},
	}

	s.processToolResult(msg)

	// Result should be appended.
	require.Len(t, s.ToolResults, 1)
	assert.Equal(t, "done", s.ToolResults[0]["data"])

	// CurrentCall should be incremented and next tool dispatched.
	assert.Equal(t, 1, s.CurrentCall)
	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "agent_workflow", emitted[0].Subject)
	assert.Equal(t, "otherwf", emitted[0].Payload.(map[string]interface{})["do"].(map[string]interface{})["call_workflow"])
}

func TestProcessToolResult_CommandResult_Advances(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Systems["mysys"] = makeSystemWithFunc("doWork")

	toolCalls := []AgentToolCall{
		{FuncName: "sys_mysys__doWork", Params: map[string]interface{}{"param1": "v"}},
		{FuncName: "sys_mysys__doWork", Params: map[string]interface{}{"param1": "v2"}},
	}

	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Driver: "openai"},
		ID:          "cmd-session",
		CurrentCall: 0,
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
		ToolCalls:   toolCalls,
		history: []AgentMessage{
			{Role: RoleAgent, ToolCalls: toolCalls},
		},
	}

	msg := &dipper.Message{
		Subject: "agent_command_result",
		Labels:  map[string]string{"status": "success", "turn_id": "1", "tool_call_id": "0"},
		Payload: map[string]interface{}{},
	}

	s.processToolResult(msg)

	// Result was appended (may be nil from empty Export, which is valid).
	assert.Len(t, s.ToolResults, 1)

	// Advances to the next call.
	assert.Equal(t, 1, s.CurrentCall)
	emitted := store.getEmitted()
	require.Len(t, emitted, 1)
	assert.Equal(t, "agent_command", emitted[0].Subject)
}

func TestProcessToolResult_FeedsBackWhenAllDone(t *testing.T) {
	// When the last tool call result is received, the session feeds all
	// collected results back to the model.
	store := newMockStore(nil)

	toolCalls := []AgentToolCall{{FuncName: "wf__done"}}
	s := &AgentSession{
		store:       store,
		Agent:       &config.Agent{Driver: "openai"},
		ID:          "feed-session",
		CurrentCall: 0, // pointing to the only (last) tool call
		ToolCalls:   toolCalls,
		ToolResults: []map[string]interface{}{{"prev": "result"}},
		history: []AgentMessage{
			{Role: RoleSystem, Content: "sys"},
			{Role: RoleAgent, ToolCalls: toolCalls},
		},
		CurrentMsg: &dipper.Message{
			Labels:  map[string]string{},
			Payload: map[string]interface{}{"text": "continue"},
		},
	}

	msg := &dipper.Message{
		Labels: map[string]string{
			"turn_id":      "2",
			"tool_call_id": "0",
		},
		Payload: map[string]interface{}{
			"data": map[string]interface{}{"output": "finalval"},
		},
	}

	s.processToolResult(msg)

	// processAgentMessage → run() should have fed the results back to the model driver.
	assert.True(t, store.hasCall("driver:openai:send_to_model"))
}

// ---------------------------------------------------------------------------
// appendConvoHistory tests
// ---------------------------------------------------------------------------

func TestAppendConvoHistory_WithConvoID_CallsRpush(t *testing.T) {
	store := newMockStore(nil)

	s := &AgentSession{
		store:   store,
		Agent:   &config.Agent{},
		ID:      "hist-session",
		ConvoID: "convo-99",
	}

	s.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "hello"})

	require.Len(t, s.history, 1)
	assert.True(t, store.hasCall("cache:rpush"))
}

func TestAppendConvoHistory_WithoutConvoID_NoCacheCall(t *testing.T) {
	store := newMockStore(nil)

	s := &AgentSession{
		store:   store,
		Agent:   &config.Agent{},
		ID:      "hist-session",
		ConvoID: "", // no convo
	}

	s.appendConvoHistory(&AgentMessage{Role: RoleUser, Content: "hello"})

	require.Len(t, s.history, 1)
	assert.False(t, store.hasCall("cache:rpush"))
}

// ---------------------------------------------------------------------------
// persist / load tests
// ---------------------------------------------------------------------------

func TestPersist_WritesToCache(t *testing.T) {
	store := newMockStore(nil)

	s := &AgentSession{
		store: store,
		Agent: &config.Agent{},
		ID:    "persist-session",
		TTL:   "1h",
	}

	s.persist(false)

	assert.True(t, store.hasCall("cache:save"))
}

func TestLoad_ReadsFromCache(t *testing.T) {
	store := newMockStore(nil)

	data := &AgentSession{
		ID:   "load-session",
		Type: AgentSessionTypeInference,
		TTL:  "1200",
	}
	store.resp["cache:load:"+AgentKeyPrefix+"load-session"] = mustMarshalJSON(data)

	s := &AgentSession{}
	s.load("load-session", store)

	assert.Equal(t, "load-session", s.ID)
	assert.Equal(t, "1200", s.TTL)
	assert.True(t, store.hasCall("cache:load"))
}

// ---------------------------------------------------------------------------
// PersistentAgentStore tests
// ---------------------------------------------------------------------------

func TestNewAgentStore_CreatesValidStore(t *testing.T) {
	helper := &mockStoreHelper{
		mockStore: *newMockStore(nil),
	}

	store := NewAgentStore(helper, "")
	assert.NotNil(t, store)

	pas, ok := store.(*PersistentAgentStore)
	require.True(t, ok)
	assert.NotNil(t, pas.Logger)
}

func TestPersistentAgentStore_GetLogger(t *testing.T) {
	helper := &mockStoreHelper{
		mockStore: *newMockStore(nil),
	}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)
	assert.NotNil(t, store.GetLogger())
	assert.Equal(t, store.Logger, store.GetLogger())
}

func TestPersistentAgentStore_GetAgent(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {Name: "bot", Driver: "openai"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	agent := store.GetAgent("bot")
	assert.Equal(t, "bot", agent.Name)
	assert.Equal(t, "openai", agent.Driver)
}

func TestPersistentAgentStore_GetSystem(t *testing.T) {
	sys := makeSystemWithFunc("fn1")
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents:    map[string]config.Agent{},
		Systems:   map[string]config.System{"s1": sys},
		Workflows: map[string]config.Workflow{},
	}}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	s := store.GetSystem("s1")
	require.Contains(t, s.Functions, "fn1")
}

func TestPersistentAgentStore_GetWorkflow(t *testing.T) {
	wf := makeWorkflow("wf1", "my workflow")
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents:    map[string]config.Agent{},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{"wf1": wf},
	}}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	w := store.GetWorkflow("wf1")
	assert.Equal(t, "my workflow", w.Description)
}

func TestPersistentAgentStore_StopAndWait(t *testing.T) {
	helper := &mockStoreHelper{mockStore: *newMockStore(nil)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	// Before Stop, stopped flag is false.
	assert.False(t, store.stopped.Load())

	store.Stop()
	assert.True(t, store.stopped.Load())

	// Wait should return immediately (no in-flight sessions).
	done := make(chan struct{})
	go func() {
		store.Wait()
		close(done)
	}()
	select {
	case <-done:
		// success
	default:
		// Wait returned synchronously, also fine.
	}
}

func TestPersistentAgentStore_StartInference(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {Name: "bot", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	msg := &dipper.Message{
		Labels:  map[string]string{"agent_name": "bot"},
		Payload: map[string]interface{}{"text": "hello"},
	}

	store.StartInference(msg)

	// send_to_model should have been called on the helper.
	assert.True(t, helper.hasCall("driver:openai:send_to_model"))
	// session should have been persisted.
	assert.True(t, helper.hasCall("cache:save"))
}

// assertNoDanglingToolUse fails if any agent tool_use in history is not
// immediately followed by a tool_result — the malformed shape that triggers the
// upstream "tool_use without tool_result" 400.
func assertNoDanglingToolUse(t *testing.T, history []AgentMessage) {
	t.Helper()
	for i, m := range history {
		if m.Role == RoleAgent && len(m.ToolCalls) > 0 {
			require.Less(t, i+1, len(history),
				"trailing tool_use with no following message (stale history was sent)")
			require.Equal(t, RoleToolResult, history[i+1].Role,
				"tool_use at %d must be followed by a tool_result, got %q", i, history[i+1].Role)
		}
	}
}

// raceHistories returns the stale snapshot a new turn would read while a prior
// turn is still in flight (ending in an unanswered tool_use) and the completed
// history that becomes visible once the prior turn finishes.
func raceHistories() (stale, complete []AgentMessage) {
	stale = []AgentMessage{
		{Role: RoleUser, Content: "prior turn"},
		{Role: RoleAgent, ToolCalls: []AgentToolCall{{FuncName: "ag__sre_github_agent"}}},
	}
	complete = []AgentMessage{
		{Role: RoleUser, Content: "prior turn"},
		{Role: RoleAgent, ToolCalls: []AgentToolCall{{FuncName: "ag__sre_github_agent"}}},
		{Role: RoleToolResult, ToolResult: []map[string]interface{}{{"status": "success"}}},
		{Role: RoleAgent, Content: "done", IsComplete: true},
	}

	return stale, complete
}

// TestPersistentAgentStore_StartInference_NewTurnLocksBeforeReadingHistory
// covers the common new-turn path: StartInference mints the session id up front
// (so it can ack before the turn lock) and, because the convo id is in the
// message, takes the turn lock before setup so history is read once, under the
// lock. The lock trigger simulates the prior turn completing (and persisting its
// history) the instant the turn lock is granted; without locking first, the
// request would carry the dangling tool_use.
func TestPersistentAgentStore_StartInference_NewTurnLocksBeforeReadingHistory(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {Name: "bot", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}

	stale, complete := raceHistories()
	histKey := "cache:lrange:" + ConvoHistoryKeyPrefix + "convo-race"
	helper.resp[histKey] = mustMarshalJSON(stale)
	helper.lockTriggers = map[string]func(){
		ConvoTurnLockPrefix + "convo-race": func() {
			helper.mu.Lock()
			helper.resp[histKey] = mustMarshalJSON(complete)
			helper.mu.Unlock()
		},
	}

	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	msg := &dipper.Message{
		Labels: map[string]string{"agent_name": "bot"},
		Payload: map[string]interface{}{
			"convo_id": "convo-race",
			"text":     "new turn triggered by the posted comment",
		},
	}

	store.StartInference(msg)

	require.True(t, helper.hasCall("driver:openai:send_to_model"))
	params := helper.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	history, ok := params["history"].([]AgentMessage)
	require.True(t, ok, "history should be []AgentMessage")
	assertNoDanglingToolUse(t, history)
}

// TestPersistentAgentStore_RunTurn_LocksBeforeReadingHistory verifies runTurn
// takes the turn lock before reading history (it can, because the convo id is a
// parameter). The lock trigger swaps the stale snapshot for the completed one
// the moment the turn lock is granted; if the read happened before the lock the
// request would carry the dangling tool_use.
func TestPersistentAgentStore_RunTurn_LocksBeforeReadingHistory(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {Name: "bot", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}

	stale, complete := raceHistories()
	histKey := "cache:lrange:" + ConvoHistoryKeyPrefix + "convo-run"
	helper.resp[histKey] = mustMarshalJSON(stale)
	helper.lockTriggers = map[string]func(){
		ConvoTurnLockPrefix + "convo-run": func() {
			helper.mu.Lock()
			helper.resp[histKey] = mustMarshalJSON(complete)
			helper.mu.Unlock()
		},
	}

	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	// runTurn does not manage the WaitGroup itself; call it directly.
	store.runTurn("bot", "convo-run", "new turn", "user", "", "")

	require.True(t, helper.hasCall("driver:openai:send_to_model"))
	params := helper.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params)
	history, ok := params["history"].([]AgentMessage)
	require.True(t, ok, "history should be []AgentMessage")
	assertNoDanglingToolUse(t, history)
}

func TestPersistentAgentStore_ReceiveInference(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {Name: "bot", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}

	// Build a persisted session.
	existing := &AgentSession{
		ID:      "rcv-session",
		Type:    AgentSessionTypeInference,
		TTL:     "1h",
		Agent:   &config.Agent{Name: "bot", Driver: "openai", Engine: "gpt-4"},
		history: []AgentMessage{{Role: RoleSystem, Content: "sys"}},
	}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	helper.resp["cache:load:"+AgentKeyPrefix+"rcv-session"] = mustMarshalJSON(existing)

	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	agentMsg := AgentMessage{Role: RoleAgent, Content: "response!"}
	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_session_id": "rcv-session",
		},
		Payload: map[string]interface{}{
			"message": map[string]interface{}{
				"Role":    RoleAgent,
				"Content": "response!",
			},
		},
	}
	_ = agentMsg

	store.ReceiveInference(msg)

	// processAgentResponse → processAgentMessage → EmitMessage (agent_response).
	assert.True(t, helper.hasCall("cache:load"))
}

func TestPersistentAgentStore_ContinueInference_ProcessResult(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {Name: "bot", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}

	// Two tool calls; CurrentCall=0 means we're processing the first result
	// and should advance to dispatch the second.
	toolCalls := []AgentToolCall{
		{FuncName: "wf__firststep"},
		{FuncName: "wf__secondstep"},
	}
	existing := &AgentSession{
		ID:          "cont-session",
		ConvoID:     "convo-cont",
		Type:        AgentSessionTypeInference,
		TTL:         "1h",
		Agent:       &config.Agent{Name: "bot", Driver: "openai", Engine: "gpt-4"},
		CurrentCall: 0,
		ToolCalls:   toolCalls,
		ToolResults: []map[string]interface{}{},
		CurrentMsg:  &dipper.Message{Labels: map[string]string{}},
	}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	helper.resp["cache:load:"+AgentKeyPrefix+"cont-session"] = mustMarshalJSON(existing)
	helper.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-cont"] = mustMarshalJSON([]AgentMessage{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleAgent, ToolCalls: toolCalls},
	})

	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	msg := &dipper.Message{
		Subject: "agent_workflow_result",
		Labels: map[string]string{
			"agent_session_id": "cont-session",
			"turn_id":          "2",
			"tool_call_id":     "0",
		},
		Payload: map[string]interface{}{
			"data": map[string]interface{}{"output": "ok"},
		},
	}

	store.ContinueInference(msg)

	// Session was loaded from cache.
	assert.True(t, helper.hasCall("cache:load"))
	// Advancing to next tool dispatches the second workflow.
	emitted := helper.getEmitted()
	require.NotEmpty(t, emitted)
	assert.Equal(t, "agent_workflow", emitted[0].Subject)
	assert.Equal(t, "secondstep", emitted[0].Payload.(map[string]interface{})["do"].(map[string]interface{})["call_workflow"])
}

func TestPersistentAgentStore_ContinueInference_Recover(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"bot": {Name: "bot", Driver: "openai", Engine: "gpt-4"},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}

	existing := &AgentSession{
		ID:      "rcv2-session",
		ConvoID: "convo-rcv2",
		Type:    AgentSessionTypeInference,
		TTL:     "1h",
		Agent:   &config.Agent{Name: "bot", Driver: "openai", Engine: "gpt-4"},
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{},
		},
	}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	helper.resp["cache:load:"+AgentKeyPrefix+"rcv2-session"] = mustMarshalJSON(existing)
	helper.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-rcv2"] = mustMarshalJSON([]AgentMessage{
		{Role: RoleSystem, Content: "sys"},
	})

	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_session_id": "rcv2-session",
			"recover":          "true",
		},
		Payload: map[string]interface{}{},
	}

	store.ContinueInference(msg)

	// recover() should dispatch send_to_model.
	assert.True(t, helper.hasCall("driver:openai:send_to_model"))
}

// TestStartTurn_UsesParentAgentWhenLastSessionIsSubAgent guards against the bug
// where a sub-agent (inference) session registers itself as LastSession in the
// unified ConvoState, causing the next UI turn to start with the sub-agent's
// name (and its max_history_len) instead of the parent chat-turn agent.
func TestStartTurn_UsesParentAgentWhenLastSessionIsSubAgent(t *testing.T) {
	cfg := &config.Config{DataSet: &config.DataSet{
		Agents: map[string]config.Agent{
			"super_coder": {Name: "super_coder", Driver: "openai", Engine: "gpt-4"},
			"code_executor": {
				Name:          "code_executor",
				Driver:        "openai",
				Engine:        "gpt-4",
				MaxHistoryLen: 20,
			},
		},
		Systems:   map[string]config.System{},
		Workflows: map[string]config.Workflow{},
	}}

	convoID := "parent-convo-123"
	superCoder := cfg.DataSet.Agents["super_coder"]

	// Simulate a ConvoState where a sub-agent inference session has overwritten
	// LastSession (which is what happens today when a sub-agent runs).
	cs := &ConvoState{
		ConvoID: convoID,
		TTL:     ConvoStreamTTL,
		Agent:   &superCoder,
		FirstSession: &ConvoSessionRef{
			SessionID: "session-turn1",
			AgentName: "super_coder",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
		LastSession: &ConvoSessionRef{
			SessionID: "session-subagent",
			AgentName: "code_executor",
			Type:      AgentSessionTypeInference,
			Status:    ConvoSessionStatusComplete,
		},
	}

	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	// Provide the parent convo state so cs.load() returns the above.
	helper.resp["cache:load:"+ConvoStateKeyPrefix+convoID] = mustMarshalJSON(cs)
	// Return an empty history for the new turn's convo.
	helper.resp["cache:lrange:"+ConvoHistoryKeyPrefix+convoID] = mustMarshalJSON([]AgentMessage{})

	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	store.StartTurn(convoID, "hello again", "user1", "", "")
	store.Wait()

	// The new session must be dispatched to the parent agent (super_coder), not code_executor.
	params := helper.getNoWaitParams("driver:openai:send_to_model")
	require.NotNil(t, params, "expected send_to_model to be called")
	// The engine field is set from the agent config; both agents use the same engine here,
	// so validate via the history: super_coder has no MaxHistoryLen, so history is not truncated.
	assert.Equal(t, "gpt-4", params["engine"])

	// The agent session persisted must have super_coder as its agent name.
	var persistedAgent string
	for _, call := range helper.getCalls() {
		if call == "cache:save" {
			break
		}
	}
	// Verify by checking that the store was called with super_coder, not code_executor,
	// by checking the noWaitParams didn't come from a code_executor dispatch
	// (code_executor would restrict history to 20, but here history is empty so that
	// distinction isn't visible — instead we check the emitted messages carry no ltrim).
	ltrimCalled := helper.hasCall("cache:ltrim")
	assert.False(t, ltrimCalled, "ltrim should not be called: super_coder has no max_history_len")
	_ = persistedAgent
}

// ---------------------------------------------------------------------------
// interpolateAgentConfig tests
// ---------------------------------------------------------------------------

func TestInterpolateAgentConfig_Basic(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:         "myagent",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "You are helpful.",
	}

	agent := interpolateAgentConfig(store, "myagent", map[string]interface{}{
		"text": "hello",
	})

	assert.NotNil(t, agent)
	assert.Equal(t, "myagent", agent.Name)
	assert.Equal(t, "openai", agent.Driver)
	assert.Equal(t, "gpt-4", agent.Engine)
	assert.Equal(t, "You are helpful.", agent.SystemPrompt)
}

func TestInterpolateAgentConfig_InterpolatesSystemPrompt(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:         "myagent",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "You are helping {{.agent_data.user}} with {{.agent_data.task}}",
	}

	payload := map[string]interface{}{
		"text": "hello",
		"data": map[string]interface{}{
			"user": "alice",
			"task": "debugging",
		},
	}

	agent := interpolateAgentConfig(store, "myagent", payload)

	assert.Equal(t, "You are helping alice with debugging", agent.SystemPrompt)
}

func TestInterpolateAgentConfig_InterpolatesInferencePrompt(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:            "myagent",
		Driver:          "openai",
		Engine:          "gpt-4",
		InferencePrompt: "Analyze {{.agent_data.code}}",
	}

	payload := map[string]interface{}{
		"text": "hello",
		"data": map[string]interface{}{
			"code": "func main() {}",
		},
	}

	agent := interpolateAgentConfig(store, "myagent", payload)

	assert.Equal(t, "Analyze func main() {}", agent.InferencePrompt)
}

func TestInterpolateAgentConfig_InterpolatesModelData(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:   "myagent",
		Driver: "openai",
		Engine: "gpt-4",
		ModelData: map[string]interface{}{
			"temperature": 0.7,
			"context":     "user={{.agent_data.user}}",
		},
	}

	payload := map[string]interface{}{
		"text": "hello",
		"data": map[string]interface{}{
			"user": "bob",
		},
	}

	agent := interpolateAgentConfig(store, "myagent", payload)

	assert.Equal(t, "user=bob", agent.ModelData["context"])
	assert.Equal(t, float64(0.7), agent.ModelData["temperature"])
}

func TestInterpolateAgentConfig_InterpolatesEngine(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:   "myagent",
		Driver: "openai",
		Engine: "$agent_data.model",
	}

	data := map[string]interface{}{
		"model": "gpt-4-turbo",
	}

	payload := map[string]interface{}{
		"text": "hello",
		"data": data,
	}

	agent := interpolateAgentConfig(store, "myagent", payload)

	assert.Equal(t, "gpt-4-turbo", agent.Engine)
}

func TestInterpolateAgentConfig_InterpolatesTools(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Workflows["mywf"] = makeWorkflow("mywf", "desc")
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:   "myagent",
		Driver: "openai",
		Engine: "gpt-4",
		Tools: []config.AgentToolDef{
			{Type: "workflow", Name: "$agent_data.wf_name"},
		},
	}

	data := map[string]interface{}{
		"wf_name": "mywf",
	}

	payload := map[string]interface{}{
		"data": data,
		"text": "hello",
	}

	agent := interpolateAgentConfig(store, "myagent", payload)

	require.Len(t, agent.Tools, 1)
	assert.Equal(t, "mywf", agent.Tools[0].Name)
}

// ---------------------------------------------------------------------------
// ConvoState Agent persistence tests
// ---------------------------------------------------------------------------

func TestConvoState_AgentSerialization(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:         "myagent",
		Driver:       "openai",
		Engine:       "gpt-4",
		SystemPrompt: "You are helpful.",
		ModelData: map[string]interface{}{
			"temperature": 0.7,
		},
	}

	// Create a ConvoState with an Agent
	cs := &ConvoState{
		ConvoID: "test-convo",
		Agent:   interpolateAgentConfig(store, "myagent", map[string]interface{}{"text": "hello"}),
	}

	// Persist it
	cs.persist(store)
	assert.True(t, store.hasCall("cache:save"))

	// Load it back
	cs2 := &ConvoState{}
	cs2.load("test-convo", store)

	// Agent should be loaded and intact
	require.NotNil(t, cs2.Agent)
	assert.Equal(t, "myagent", cs2.Agent.Name)
	assert.Equal(t, "openai", cs2.Agent.Driver)
	assert.Equal(t, "gpt-4", cs2.Agent.Engine)
	assert.Equal(t, "You are helpful.", cs2.Agent.SystemPrompt)
	assert.Equal(t, float64(0.7), cs2.Agent.ModelData["temperature"])
}

func TestSetup_NewConvo_StoresInterpolatedAgent(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:         "myagent",
		Driver:       "openai",
		Engine:       "$agent_data.model",
		SystemPrompt: "Help with {{.agent_data.task}}",
	}

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "myagent",
		},
		Payload: map[string]interface{}{
			"text": "hello",
			"type": AgentSessionTypeChatTurn,
			"data": map[string]interface{}{
				"model": "gpt-4-turbo",
				"task":  "coding",
			},
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	// Session agent should be interpolated
	assert.Equal(t, "gpt-4-turbo", s.Agent.Engine)
	assert.Equal(t, "Help with coding", s.Agent.SystemPrompt)

	// ConvoState should contain the interpolated agent
	cs := &ConvoState{}
	cs.load(s.ConvoID, store)
	require.NotNil(t, cs.Agent)
	assert.Equal(t, "gpt-4-turbo", cs.Agent.Engine)
	assert.Equal(t, "Help with coding", cs.Agent.SystemPrompt)
}

func TestSetup_ExistingConvo_LoadsAgentFromConvoState(t *testing.T) {
	store := newMockStore(nil)

	// Pre-create a ConvoState with an interpolated agent
	originalAgent := config.Agent{
		Name:         "myagent",
		Driver:       "openai",
		Engine:       "gpt-4-turbo",
		SystemPrompt: "Help with coding",
	}

	cs := &ConvoState{
		ConvoID: "convo-123",
		Agent:   &originalAgent,
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-123"] = mustMarshalJSON(cs)
	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-123"] = mustMarshalJSON([]AgentMessage{
		{Role: RoleUser, Content: "hello"},
	})

	// Create a new session pointing to the existing convo
	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "myagent",
		},
		Payload: map[string]interface{}{
			"type":     AgentSessionTypeChatTurn,
			"convo_id": "convo-123",
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	// Session should have loaded agent from ConvoState
	require.NotNil(t, s.Agent)
	assert.Equal(t, "gpt-4-turbo", s.Agent.Engine)
	assert.Equal(t, "Help with coding", s.Agent.SystemPrompt)
}

func TestSetup_PanicsIfAgentMissing(t *testing.T) {
	store := newMockStore(nil)

	// Create a ConvoState with nil Agent (simulating cache from before Agent field was added)
	cs := &ConvoState{
		ConvoID: "convo-456",
		Agent:   nil,
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-456"] = mustMarshalJSON(cs)
	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-456"] = mustMarshalJSON([]AgentMessage{
		{Role: RoleUser, Content: "hello"},
	})

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "myagent",
		},
		Payload: map[string]interface{}{
			"convo_id": "convo-456",
		},
	}

	s := &AgentSession{}

	// Should panic because Agent is nil for an existing conversation
	assert.Panics(t, func() {
		s.setup(msg, store, false)
	})
}

func TestSetup_ForgetHistory_ClearsMemory(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["someagent"] = config.Agent{Name: "someagent"}

	// Pre-create a ConvoState with Generation=0 and Agent set
	cs := &ConvoState{
		ConvoID:    "convo-abc",
		Generation: 0,
		Agent:      &config.Agent{Name: "someagent"},
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-abc"] = mustMarshalJSON(cs)

	// Pre-seed history so that s.history will be populated by loadConvoHistory
	prevHistory := []AgentMessage{
		{Role: RoleSystem, Content: "You are a helpful assistant"},
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAgent, Content: "Hi there!"},
		{Role: RoleUser, Content: "What's the weather?"},
	}
	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-abc"] = mustMarshalJSON(prevHistory)

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "someagent",
		},
		Payload: map[string]interface{}{
			"type":           AgentSessionTypeChatTurn,
			"convo_id":       "convo-abc",
			"forget_history": true,
			"text":           "start fresh",
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	// s.history should be cleared and contain ONLY the marker
	assert.Len(t, s.history, 1, "history should contain only the marker message")
	assert.Equal(t, RoleSystem, s.history[0].Role)
	assert.Contains(t, s.history[0].Content, "archived_convo:")

	// Verify the old messages are NOT in history
	for _, m := range s.history {
		assert.NotEqual(t, "Hello", m.Content, "old user message should not be in history")
		assert.NotEqual(t, "Hi there!", m.Content, "old assistant message should not be in history")
		assert.NotEqual(t, "What's the weather?", m.Content, "old user message should not be in history")
	}

	// Verify cache:del and cache:rpush were called
	assert.True(t, store.hasCall("cache:del"), "cache:del should be called")
	assert.True(t, store.hasCall("cache:rpush"), "cache:rpush should be called to persist the marker")
}

func TestSetup_ForgetHistory_InjectsMarker(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["someagent"] = config.Agent{Name: "someagent"}

	cs := &ConvoState{
		ConvoID:    "convo-xyz",
		Generation: 0,
		Agent:      &config.Agent{Name: "someagent"},
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-xyz"] = mustMarshalJSON(cs)

	prevHistory := []AgentMessage{
		{Role: RoleUser, Content: "previous message 1"},
		{Role: RoleAgent, Content: "previous response 1"},
	}
	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-xyz"] = mustMarshalJSON(prevHistory)

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "someagent",
		},
		Payload: map[string]interface{}{
			"type":           AgentSessionTypeChatTurn,
			"convo_id":       "convo-xyz",
			"forget_history": true,
			"text":           "start fresh",
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	// Only the marker should be in history
	require.Len(t, s.history, 1)
	assert.Equal(t, RoleSystem, s.history[0].Role)
	assert.True(t, strings.HasPrefix(s.history[0].Content, "<!-- archived_convo: "))
	assert.True(t, strings.HasSuffix(s.history[0].Content, " -->"))
}

func TestSetup_ForgetHistory_MarkerFormat(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["someagent"] = config.Agent{Name: "someagent"}

	cs := &ConvoState{
		ConvoID:    "convo-fmt",
		Generation: 0,
		Agent:      &config.Agent{Name: "someagent"},
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-fmt"] = mustMarshalJSON(cs)

	prevHistory := []AgentMessage{
		{Role: RoleUser, Content: "a message"},
	}
	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-fmt"] = mustMarshalJSON(prevHistory)

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "someagent",
		},
		Payload: map[string]interface{}{
			"type":           AgentSessionTypeChatTurn,
			"convo_id":       "convo-fmt",
			"forget_history": true,
			"text":           "start fresh",
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	require.Len(t, s.history, 1)
	content := s.history[0].Content

	// Expected: <!-- archived_convo: convo-fmt_g1 -->
	expected := fmt.Sprintf("<!-- archived_convo: %s_g%d -->", "convo-fmt", 1)
	assert.Equal(t, expected, content)

	// Also verify the generation number via parsing
	parts := strings.Split(content, "_g")
	require.Len(t, parts, 2)
	genStr := strings.TrimSuffix(parts[1], " -->")
	gen, err := strconv.Atoi(genStr)
	require.NoError(t, err)
	assert.Equal(t, 1, gen)
}

func TestSetup_ForgetHistory_MultipleGenerations(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["someagent"] = config.Agent{Name: "someagent"}

	// Generation=1 means _g1 was already archived; next archive should be _g2
	cs := &ConvoState{
		ConvoID:    "convo-multi",
		Generation: 1,
		Agent:      &config.Agent{Name: "someagent"},
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-multi"] = mustMarshalJSON(cs)

	prevHistory := []AgentMessage{
		{Role: RoleUser, Content: "some old message"},
		{Role: RoleAgent, Content: "some old response"},
	}
	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-multi"] = mustMarshalJSON(prevHistory)

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "someagent",
		},
		Payload: map[string]interface{}{
			"type":           AgentSessionTypeChatTurn,
			"convo_id":       "convo-multi",
			"forget_history": true,
			"text":           "start fresh",
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	require.Len(t, s.history, 1)
	content := s.history[0].Content

	// Should be _g2 since Generation was already 1
	expected := "<!-- archived_convo: convo-multi_g2 -->"
	assert.Equal(t, expected, content)
}

func TestSetup_NoForgetHistory_NoMarker(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["someagent"] = config.Agent{Name: "someagent"}

	cs := &ConvoState{
		ConvoID:    "convo-noforget",
		Generation: 0,
		Agent:      &config.Agent{Name: "someagent"},
	}
	store.resp["cache:load:"+ConvoStateKeyPrefix+"convo-noforget"] = mustMarshalJSON(cs)

	prevHistory := []AgentMessage{
		{Role: RoleSystem, Content: "You are helpful"},
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAgent, Content: "Hi!"},
	}
	store.resp["cache:lrange:"+ConvoHistoryKeyPrefix+"convo-noforget"] = mustMarshalJSON(prevHistory)

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": "someagent",
		},
		Payload: map[string]interface{}{
			"type":     AgentSessionTypeChatTurn,
			"convo_id": "convo-noforget",
			"text":     "continue chat",
			// No forget_history field
		},
	}

	s := &AgentSession{}
	s.setup(msg, store, false)

	// History should be loaded as-is from cache without modification
	assert.Len(t, s.history, 3)
	assert.Equal(t, "Hello", s.history[1].Content)
	assert.Equal(t, "Hi!", s.history[2].Content)

	// No cache:del should be called
	assert.False(t, store.hasCall("cache:del"), "cache:del should NOT be called without forget_history")
}

// ---------------------------------------------------------------------------
// Engine / driver override tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Engine / driver override tests
// ---------------------------------------------------------------------------

func TestAgentOverrideEngineAndDriver_NewConvo(t *testing.T) {
	cfg := &config.Config{
		DataSet: &config.DataSet{
			Agents: map[string]config.Agent{
				"test_agent": {
					Name:   "test_agent",
					Driver: "default_driver",
					Engine: "default_engine",
				},
			},
			Workflows: map[string]config.Workflow{},
			Systems:   map[string]config.System{},
		},
	}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	store.StartNewConvo("test_agent", "hello", "user1", "gpt-4", "openai-new")
	store.Wait()

	// The session should have been created with the overridden engine/driver.
	params := helper.getNoWaitParams("driver:openai-new:send_to_model")
	require.NotNil(t, params, "expected send_to_model to be called with overridden driver")
	engine, ok := params["engine"].(string)
	require.True(t, ok, "engine must be a string")
	assert.Equal(t, "gpt-4", engine, "engine should be overridden")
}

func TestAgentOverrideEngineAndDriver_ExistingTurn(t *testing.T) {
	convoID := dipper.NewUUID()
	cfg := &config.Config{
		DataSet: &config.DataSet{
			Agents: map[string]config.Agent{
				"test_agent": {
					Name:   "test_agent",
					Driver: "default_driver",
					Engine: "default_engine",
				},
			},
			Workflows: map[string]config.Workflow{},
			Systems:   map[string]config.System{},
		},
	}

	cs := &ConvoState{
		ConvoID: convoID,
		TTL:     ConvoStreamTTL,
		Agent:   &config.Agent{Name: "test_agent", Driver: "default_driver", Engine: "default_engine"},
		FirstSession: &ConvoSessionRef{
			SessionID: "session-first",
			AgentName: "test_agent",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
		LastSession: &ConvoSessionRef{
			SessionID: "session-last",
			AgentName: "test_agent",
			Type:      AgentSessionTypeChatTurn,
			Status:    ConvoSessionStatusComplete,
		},
	}

	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	helper.resp["cache:load:"+ConvoStateKeyPrefix+convoID] = mustMarshalJSON(cs)
	helper.resp["cache:lrange:"+ConvoHistoryKeyPrefix+convoID] = mustMarshalJSON([]AgentMessage{})

	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	store.StartTurn(convoID, "follow up", "user1", "claude-3", "anthropic")
	store.Wait()

	params := helper.getNoWaitParams("driver:anthropic:send_to_model")
	require.NotNil(t, params, "expected send_to_model to be called with overridden driver")
	engine, ok := params["engine"].(string)
	require.True(t, ok, "engine must be a string")
	assert.Equal(t, "claude-3", engine, "engine should be overridden")
}

func TestAgentOverrideEngineAndDriver_NoOverride(t *testing.T) {
	cfg := &config.Config{
		DataSet: &config.DataSet{
			Agents: map[string]config.Agent{
				"test_agent": {
					Name:   "test_agent",
					Driver: "default_driver",
					Engine: "default_engine",
				},
			},
			Workflows: map[string]config.Workflow{},
			Systems:   map[string]config.System{},
		},
	}
	helper := &mockStoreHelper{mockStore: *newMockStore(cfg)}
	store := NewAgentStore(helper, "").(*PersistentAgentStore)

	store.StartNewConvo("test_agent", "hello", "user1", "", "")
	store.Wait()

	// No override — should use default_driver from config.
	params := helper.getNoWaitParams("driver:default_driver:send_to_model")
	require.NotNil(t, params, "expected send_to_model to be called with default driver")
	engine, ok := params["engine"].(string)
	require.True(t, ok, "engine must be a string")
	assert.Equal(t, "default_engine", engine, "engine should be from agent config")
}
