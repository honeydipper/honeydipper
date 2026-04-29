// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/agentruntime"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

var (
	agent                   *Service
	agentMatchCounter       int64
	agentPendingTurns       int64
	errAgentNotInitialized  = errors.New("agent service is not initialized")
	errAgentEventbusMissing = errors.New("eventbus driver not loaded")
)

var (
	persistActivationFn = persistAgentActivation
	resolveContextFn    = resolveTurnContext
	enqueueProviderFn   = enqueueProviderCommand
	agentStateCallerFn  = func() dipper.RPCCaller { return agent }
)

// StartAgent starts the agent service.
func StartAgent(cfg *config.Config) {
	agent = NewService(cfg, "agent")

	agent.EmitMetrics = agentMetrics
	agent.addResponder("eventbus:activate", createActivations)
	agent.addResponder("eventbus:agent_return", continueProviderTurn)

	agent.start()
}

func createActivations(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] continue processing activate rules")
	<-agent.Ready()
	msg = dipper.DeserializePayload(msg)

	agentName, ok := dipper.GetMapDataStr(msg.Payload, "agent")
	if !ok || agentName == "" {
		return
	}

	result, err := persistActivationFn(agent, msg, agentName)
	if err != nil {
		dipper.Logger.Warningf("[agent] failed to persist activation: %+v", err)

		return
	}
	if err := resolveContextFn(agent, result.SessionID, result.TurnID); err != nil {
		dipper.Logger.Warningf("[agent] failed resolving context for session %s turn %s: %+v", result.SessionID, result.TurnID, err)

		return
	}

	atomic.AddInt64(&agentMatchCounter, 1)
	dipper.Logger.Infof("[agent] activation resolved for agent %s session %s turn %s", agentName, result.SessionID, result.TurnID)
}

func agentMetrics() {
	agent.GaugeSet("honey.honeydipper.agent.activations", strconv.FormatInt(atomic.LoadInt64(&agentMatchCounter), 10), []string{})
	agent.GaugeSet("honey.honeydipper.agent.pending_turns", strconv.FormatInt(atomic.LoadInt64(&agentPendingTurns), 10), []string{})
}

func persistAgentActivation(caller dipper.RPCCaller, msg *dipper.Message, agentName string) (*agentruntime.ActivationPersistResult, error) {
	result, err := agentruntime.PersistActivation(caller, msg, agentName)
	if err != nil {
		return nil, fmt.Errorf("persist activation runtime flow: %w", err)
	}
	atomic.AddInt64(&agentPendingTurns, 1)

	return result, nil
}

func resolveTurnContext(caller dipper.RPCCaller, sessionID string, turnID string) error {
	if err := agentruntime.ResolveTurnContext(caller, sessionID, turnID, getAgentDefinition, enqueueProviderFn); err != nil {
		return fmt.Errorf("resolve turn runtime flow: %w", err)
	}

	return nil
}

func enqueueProviderCommand(msg *dipper.Message) error {
	if agent == nil {
		return errAgentNotInitialized
	}
	worker := agent.getDriverRuntime(dipper.ChannelEventbus)
	if worker == nil {
		return errAgentEventbusMissing
	}
	worker.SendMessage(msg)

	return nil
}

func continueProviderTurn(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] continue processing provider return")
	<-agent.Ready()
	msg = dipper.DeserializePayload(msg)

	caller := agentStateCallerFn()
	if caller == nil {
		return
	}
	if err := agentruntime.ContinueProviderTurn(caller, msg, enqueueProviderFn); err != nil {
		dipper.Logger.Warningf("[agent] failed continue provider turn: %+v", err)
	}
}

func getAgentDefinition(agentName string) (config.Agent, bool) {
	if agent == nil || agent.config == nil || agent.config.DataSet == nil {
		return config.Agent{}, false
	}
	def, ok := agent.config.DataSet.Agents[agentName]
	if !ok {
		return config.Agent{}, false
	}

	return def, true
}
