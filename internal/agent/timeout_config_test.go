// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agent

import (
	"bytes"
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterpolateAgentConfig_TurnLockTimeout_EmptyUsesDefault(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:            "myagent",
		Driver:          "openai",
		Engine:          "gpt-4",
		TurnLockTimeout: "",
	}

	agent := interpolateAgentConfig(store, "myagent", map[string]interface{}{
		"text": "hello",
	})

	assert.Equal(t, "", agent.TurnLockTimeout, "empty turn_lock_timeout should remain empty (default used in lockTurn)")
}

func TestInterpolateAgentConfig_TurnLockTimeout_ValidDuration(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:            "myagent",
		Driver:          "openai",
		Engine:          "gpt-4",
		TurnLockTimeout: "30m",
	}

	agent := interpolateAgentConfig(store, "myagent", map[string]interface{}{
		"text": "hello",
	})

	assert.Equal(t, "30m", agent.TurnLockTimeout)
}

func TestInterpolateAgentConfig_TurnLockTimeout_InvalidDurationLogsWarning(t *testing.T) {
	var logBuf bytes.Buffer
	logger := logging.MustGetLogger("test")
	backend := logging.NewLogBackend(&logBuf, "", 0)
	backendLeveled := logging.AddModuleLevel(backend)
	backendLeveled.SetLevel(logging.WARNING, "test")
	logger.SetBackend(backendLeveled)

	store := newMockStore(nil)
	store.logger = logger
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:            "myagent",
		Driver:          "openai",
		Engine:          "gpt-4",
		TurnLockTimeout: "invalid-duration",
	}

	agent := interpolateAgentConfig(store, "myagent", map[string]interface{}{
		"text": "hello",
	})

	// Invalid duration should be reset to empty (default will be used)
	assert.Equal(t, "", agent.TurnLockTimeout)
	// Warning should be logged
	assert.Contains(t, logBuf.String(), "invalid turn_lock_timeout duration")
	assert.Contains(t, logBuf.String(), "using default")
}

func TestInterpolateAgentConfig_DriverCallTimeout_EmptyUsesDefault(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:              "myagent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "",
	}

	agent := interpolateAgentConfig(store, "myagent", map[string]interface{}{
		"text": "hello",
	})

	assert.Equal(t, "", agent.DriverCallTimeout, "empty driver_call_timeout should remain empty (default used in sendToDriver)")
}

func TestInterpolateAgentConfig_DriverCallTimeout_ValidDuration(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:              "myagent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "120s",
	}

	agent := interpolateAgentConfig(store, "myagent", map[string]interface{}{
		"text": "hello",
	})

	assert.Equal(t, "120s", agent.DriverCallTimeout)
}

func TestInterpolateAgentConfig_DriverCallTimeout_InvalidDurationLogsWarning(t *testing.T) {
	var logBuf bytes.Buffer
	logger := logging.MustGetLogger("test")
	backend := logging.NewLogBackend(&logBuf, "", 0)
	backendLeveled := logging.AddModuleLevel(backend)
	backendLeveled.SetLevel(logging.WARNING, "test")
	logger.SetBackend(backendLeveled)

	store := newMockStore(nil)
	store.logger = logger
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:              "myagent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "not-a-duration",
	}

	agent := interpolateAgentConfig(store, "myagent", map[string]interface{}{
		"text": "hello",
	})

	// Invalid duration should be reset to empty (default will be used)
	assert.Equal(t, "", agent.DriverCallTimeout)
	// Warning should be logged
	assert.Contains(t, logBuf.String(), "invalid driver_call_timeout duration")
	assert.Contains(t, logBuf.String(), "using default")
}

func TestInterpolateAgentConfig_TurnLockTimeout_Interpolation(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:            "myagent",
		Driver:          "openai",
		Engine:          "gpt-4",
		TurnLockTimeout: "$agent_data.lock_timeout",
	}

	agent := interpolateAgentConfig(store, "myagent", map[string]interface{}{
		"text": "hello",
		"data": map[string]interface{}{
			"lock_timeout": "45m",
		},
	})

	assert.Equal(t, "45m", agent.TurnLockTimeout)
}

func TestInterpolateAgentConfig_DriverCallTimeout_Interpolation(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["myagent"] = config.Agent{
		Name:              "myagent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "$agent_data.call_timeout",
	}

	agent := interpolateAgentConfig(store, "myagent", map[string]interface{}{
		"text": "hello",
		"data": map[string]interface{}{
			"call_timeout": "200s",
		},
	})

	assert.Equal(t, "200s", agent.DriverCallTimeout)
}

func TestLockTurn_UsesAgentTurnLockTimeout(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["test-agent"] = config.Agent{
		Name:            "test-agent",
		Driver:          "openai",
		Engine:          "gpt-4",
		TurnLockTimeout: "45m",
	}

	agent := store.cfg.DataSet.Agents["test-agent"]

	s := &AgentSession{
		Agent:   &agent,
		store:   store,
		ID:      "test-session",
		ConvoID: "test-convo",
	}

	s.lockTurn(store, "test-convo")

	// Verify the lock call was made with the custom timeout
	params := store.getCallParams("locker:lock")

	require.NotNil(t, params)
	assert.Equal(t, "45m", params["expire"])
}

func TestLockTurn_UsesDefaultWhenAgentTurnLockTimeoutEmpty(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["test-agent"] = config.Agent{
		Name:            "test-agent",
		Driver:          "openai",
		Engine:          "gpt-4",
		TurnLockTimeout: "",
	}

	agent := store.cfg.DataSet.Agents["test-agent"]

	s := &AgentSession{
		Agent:   &agent,
		store:   store,
		ID:      "test-session",
		ConvoID: "test-convo",
	}

	s.lockTurn(store, "test-convo")

	params := store.getCallParams("locker:lock")
	require.NotNil(t, params)
	assert.Equal(t, AgentSessionDefaultTurnLockExpire, params["expire"])
}

func TestLockTurn_MessageLabelTimeoutNotUsed(t *testing.T) {
	// The turn lock timeout should NOT be overridden by message label "timeout"
	// It only uses Agent.TurnLockTimeout or default
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["test-agent"] = config.Agent{
		Name:            "test-agent",
		Driver:          "openai",
		Engine:          "gpt-4",
		TurnLockTimeout: "30m",
	}

	agent := store.cfg.DataSet.Agents["test-agent"]

	s := &AgentSession{
		Agent:   &agent,
		store:   store,
		ID:      "test-session",
		ConvoID: "test-convo",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"timeout": "10m", // This should be IGNORED for turn lock
			},
		},
	}

	s.lockTurn(store, "test-convo")

	params := store.getCallParams("locker:lock")
	require.NotNil(t, params)
	// Should use Agent.TurnLockTimeout (30m), NOT message label timeout (10m)
	assert.Equal(t, "30m", params["expire"])
}

func TestLockTurn_WithNilAgentUsesDefault(t *testing.T) {
	store := newMockStore(nil)

	s := &AgentSession{
		Agent:   nil, // No agent config
		store:   store,
		ID:      "test-session",
		ConvoID: "test-convo",
	}

	s.lockTurn(store, "test-convo")

	params := store.getCallParams("locker:lock")
	require.NotNil(t, params)
	assert.Equal(t, AgentSessionDefaultTurnLockExpire, params["expire"])
}

func TestSendToDriver_UsesMessageLabelTimeoutFirst(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["test-agent"] = config.Agent{
		Name:              "test-agent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "120s", // Agent config timeout
	}

	agent := store.cfg.DataSet.Agents["test-agent"]

	s := &AgentSession{
		Agent: &agent,
		store: store,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"timeout": "60s", // Message label timeout (highest priority)
			},
		},
		Type: AgentSessionTypeChatTurn,
		history: []AgentMessage{
			{Role: RoleSystem, Content: "sys"},
		},
	}

	s.sendToDriver()

	// Check the driver call was made
	assert.True(t, store.hasCall("driver:openai:send_to_model"))

	// Check the timeout label passed to the driver call
	labels := store.getNoWaitLabels("driver:openai:send_to_model")
	require.NotNil(t, labels)
	// labelsKV format: "agent_session_id", s.ID, "timeout", timeout
	// Find the timeout value
	for i := 0; i < len(labels)-1; i++ {
		if labels[i] == "timeout" {
			assert.Equal(t, "60s", labels[i+1], "message label timeout should be used")

			return
		}
	}
	t.Fatal("timeout label not found in driver call")
}

func TestSendToDriver_UsesAgentDriverCallTimeoutWhenNoMessageLabel(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["test-agent"] = config.Agent{
		Name:              "test-agent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "120s", // Agent config timeout
	}

	agent := store.cfg.DataSet.Agents["test-agent"]

	s := &AgentSession{
		Agent: &agent,
		store: store,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{}, // No timeout label
		},
		Type: AgentSessionTypeChatTurn,
		history: []AgentMessage{
			{Role: RoleSystem, Content: "sys"},
		},
	}

	s.sendToDriver()

	assert.True(t, store.hasCall("driver:openai:send_to_model"))

	// Check the timeout label passed to the driver call
	labels := store.getNoWaitLabels("driver:openai:send_to_model")
	require.NotNil(t, labels)
	for i := 0; i < len(labels)-1; i++ {
		if labels[i] == "timeout" {
			assert.Equal(t, "120s", labels[i+1], "agent config timeout should be used when no message label")

			return
		}
	}
	t.Fatal("timeout label not found in driver call")
}

func TestSendToDriver_UsesDefaultWhenNoMessageLabelAndNoAgentTimeout(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["test-agent"] = config.Agent{
		Name:              "test-agent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "", // No agent timeout
	}

	agent := store.cfg.DataSet.Agents["test-agent"]

	s := &AgentSession{
		Agent: &agent,
		store: store,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{}, // No timeout label
		},
		Type: AgentSessionTypeChatTurn,
		history: []AgentMessage{
			{Role: RoleSystem, Content: "sys"},
		},
	}

	s.sendToDriver()

	assert.True(t, store.hasCall("driver:openai:send_to_model"))

	// Check the timeout label passed to the driver call
	labels := store.getNoWaitLabels("driver:openai:send_to_model")
	require.NotNil(t, labels)
	for i := 0; i < len(labels)-1; i++ {
		if labels[i] == "timeout" {
			assert.Equal(t, AgentSessionDefaultDriverCallTimeout, labels[i+1], "default timeout should be used when neither message label nor agent config set")

			return
		}
	}
	t.Fatal("timeout label not found in driver call")
}

func TestSendToDriver_TimeoutPriority_MessageLabelOverAgentConfig(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["test-agent"] = config.Agent{
		Name:              "test-agent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "300s", // Agent config timeout (lower priority)
	}

	agent := store.cfg.DataSet.Agents["test-agent"]

	s := &AgentSession{
		Agent: &agent,
		store: store,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{
				"timeout": "60s", // Message label timeout (higher priority)
			},
		},
		Type: AgentSessionTypeChatTurn,
		history: []AgentMessage{
			{Role: RoleSystem, Content: "sys"},
		},
	}

	s.sendToDriver()

	assert.True(t, store.hasCall("driver:openai:send_to_model"))

	labels := store.getNoWaitLabels("driver:openai:send_to_model")
	require.NotNil(t, labels)
	for i := 0; i < len(labels)-1; i++ {
		if labels[i] == "timeout" {
			assert.Equal(t, "60s", labels[i+1], "message label timeout should override agent config timeout")

			return
		}
	}
	t.Fatal("timeout label not found in driver call")
}

func TestSendToDriver_TimeoutPriority_AgentConfigOverDefault(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["test-agent"] = config.Agent{
		Name:              "test-agent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "180s", // Agent config timeout
	}

	agent := store.cfg.DataSet.Agents["test-agent"]

	s := &AgentSession{
		Agent: &agent,
		store: store,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{}, // No message label timeout
		},
		Type: AgentSessionTypeChatTurn,
		history: []AgentMessage{
			{Role: RoleSystem, Content: "sys"},
		},
	}

	s.sendToDriver()

	assert.True(t, store.hasCall("driver:openai:send_to_model"))

	labels := store.getNoWaitLabels("driver:openai:send_to_model")
	require.NotNil(t, labels)
	for i := 0; i < len(labels)-1; i++ {
		if labels[i] == "timeout" {
			assert.Equal(t, "180s", labels[i+1], "agent config timeout should override default")

			return
		}
	}
	t.Fatal("timeout label not found in driver call")
}

func TestSendToDriver_TimeoutPriority_DefaultWhenNeitherSet(t *testing.T) {
	store := newMockStore(nil)
	store.cfg.DataSet.Agents["test-agent"] = config.Agent{
		Name:              "test-agent",
		Driver:            "openai",
		Engine:            "gpt-4",
		DriverCallTimeout: "", // No agent timeout
	}

	agent := store.cfg.DataSet.Agents["test-agent"]

	s := &AgentSession{
		Agent: &agent,
		store: store,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{}, // No message label timeout
		},
		Type: AgentSessionTypeChatTurn,
		history: []AgentMessage{
			{Role: RoleSystem, Content: "sys"},
		},
	}

	s.sendToDriver()

	assert.True(t, store.hasCall("driver:openai:send_to_model"))

	labels := store.getNoWaitLabels("driver:openai:send_to_model")
	require.NotNil(t, labels)
	for i := 0; i < len(labels)-1; i++ {
		if labels[i] == "timeout" {
			assert.Equal(t, AgentSessionDefaultDriverCallTimeout, labels[i+1], "default timeout should be used when neither message label nor agent config set")

			return
		}
	}
	t.Fatal("timeout label not found in driver call")
}

func TestSendToDriver_WithMinimalAgentUsesDefault(t *testing.T) {
	store := newMockStore(nil)

	// Use a minimal agent with no timeout configs set
	s := &AgentSession{
		Agent: &config.Agent{Name: "test", Driver: "openai", Engine: "gpt-4"}, // Minimal agent with no timeouts
		store: store,
		ID:    "test-session",
		CurrentMsg: &dipper.Message{
			Labels: map[string]string{},
		},
		Type: AgentSessionTypeChatTurn,
		history: []AgentMessage{
			{Role: RoleSystem, Content: "sys"},
		},
	}

	s.sendToDriver()

	assert.True(t, store.hasCall("driver:openai:send_to_model"))
}
