// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"github.com/honeydipper/honeydipper/v4/internal/agent"
	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/daemon"
	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// Channel and subject constants for the agent service.
const (
	ChannelAgentbus        = "agentbus"
	SubjectAgentbusReceive = "receive"

	SubjectEventbusAgentStart    = "agent_start"
	SubjectEventbusAgentContinue = dipper.EventbusAgentContinue
	SubjectEventbusAgentRecover  = "agent_recover"
)

var (
	agentSvc   *Service
	agentStore agent.AgentStore
)

// StartAgent starts the agent service.
//
// The service:
//   - loads all AI-model drivers referenced by the agent configurations;
//   - receives new inference requests on eventbus:agent_start;
//   - receives tool-call results from the operator on eventbus:agent_continue;
//   - receives model responses from AI drivers on agentbus:receive and routes them
//     to the agent session store for further processing.
func StartAgent(cfg *config.Config) {
	agentSvc = NewService(cfg, "agent")
	agentSvc.Route = agentRoute
	agentSvc.DiscoverFeatures = AgentFeatures

	// Register responders before start() so no messages are missed.
	agentSvc.addResponder(dipper.ChannelEventbus+":"+SubjectEventbusAgentStart, handleAgentStart)
	agentSvc.addResponder(dipper.ChannelEventbus+":"+SubjectEventbusAgentContinue, handleAgentContinue)
	agentSvc.addResponder(ChannelAgentbus+":"+SubjectAgentbusReceive, handleAgentReceive)
	agentSvc.addResponder(dipper.ChannelEventbus+":"+dipper.EventbusRPCInterrupted, handleRPCInterrupted)
	agentSvc.addResponder(dipper.ChannelEventbus+":"+SubjectEventbusAgentRecover, handleAgentRecover)

	// Build the store before start() so the package-level var is set
	// before any goroutine spawned by a responder can reference it.
	agentStore = agent.NewAgentStore(agentSvc)
	agentSvc.Drain = func() {
		agentStore.Stop()
		agentStore.Wait()
	}

	agentSvc.start()
}

// AgentFeatures discovers all AI-model drivers referenced by agent definitions and
// returns them as dynamic features so the service loads each one.
func AgentFeatures(c *config.DataSet) map[string]interface{} {
	dynamicData := map[string]interface{}{}
	for _, ag := range c.Agents {
		if ag.Driver != "" {
			dynamicData["driver:"+ag.Driver] = nil
		}
	}

	return dynamicData
}

// agentRoute forwards eventbus messages (tool-call dispatches emitted by agent sessions)
// to the eventbus driver so the operator service can pick them up.
// Agentbus messages from AI drivers are handled entirely by responders and do not need routing.
func agentRoute(msg *dipper.Message) []RoutedMessage {
	if msg.Channel == dipper.ChannelEventbus {
		if bus := agentSvc.getDriverRuntime(dipper.ChannelEventbus); bus != nil {
			return []RoutedMessage{{driverRuntime: bus, message: msg}}
		}
	}

	return nil
}

// handleAgentStart receives an agentbus:start message and calls StartInference on the store.
func handleAgentStart(_ *driver.Runtime, msg *dipper.Message) {
	callerID := msg.Labels["caller_id"]
	callerType := msg.Labels["caller_type"]
	defer dipper.SafeExitOnError("[agent] error in handleAgentStart", func(r error) {
		if callerID == "" {
			return
		}
		agentStore.EmitMessage(dipper.Message{
			Channel: dipper.ChannelEventbus,
			Subject: "agent_response",
			Labels: map[string]string{
				"caller_id":   callerID,
				"caller_type": callerType,
				"status":      "error",
				"reason":      r.Error(),
			},
		})
	})
	<-agentSvc.Ready()
	msg = dipper.DeserializePayload(msg)
	go agentStore.StartInference(msg)
}

// handleAgentContinue receives an agentbus:continue message (tool-call result from the
// operator) and calls ContinueInference on the store.
func handleAgentContinue(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] error in handleAgentContinue")
	<-agentSvc.Ready()
	msg = dipper.DeserializePayload(msg)
	go agentStore.ContinueInference(msg)
}

// handleAgentReceive receives an agentbus:receive message emitted by an AI-model driver
// when model inference completes and calls ReceiveInference on the store.
func handleAgentReceive(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] error in handleAgentReceive")
	<-agentSvc.Ready()
	msg = dipper.DeserializePayload(msg)
	go agentStore.ReceiveInference(msg)
}

// handleRPCInterrupted handles an eventbus:rpc/interrupted notification from an AI-model
// driver whose context was cancelled mid-inference.  If the message carries an
// agent_session_id label the session is queued for recovery via the durable
// agent_recover redis topic.
func handleRPCInterrupted(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] error in handleRPCInterrupted")
	if msg.Labels["agent_session_id"] == "" {
		return
	}
	daemon.Go(func() {
		if agentSvc.drainingGroup != nil {
			// Wait until all drivers have acknowledged draining so the
			// redisqueue driver is still running but has stopped reading
			// its input queues — writes to it will land in redis.
			agentSvc.drainingGroup.Wait()
		}
		agentSvc.getDriverRuntime(dipper.ChannelEventbus).SendMessage(&dipper.Message{
			Channel: dipper.ChannelEventbus,
			Subject: SubjectEventbusAgentRecover,
			Labels:  msg.Labels,
		})
	})
}

// handleAgentRecover handles a durable agent_recover message delivered from redis
// and retries the interrupted inference session.
func handleAgentRecover(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] error in handleAgentRecover")
	<-agentSvc.Ready()
	msg = dipper.DeserializePayload(msg)
	msg.Labels["recover"] = "true"
	go agentStore.ContinueInference(msg)
}
