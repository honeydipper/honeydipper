// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org.

package service

import (
	"fmt"
	"os"

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

	SubjectEventbusAgentStart     = "agent_start"
	SubjectEventbusAgentContinue  = dipper.EventbusAgentContinue
	SubjectEventbusAgentRecover   = "agent_recover"
	SubjectAgentbusRPCInterrupted = "rpc_interrupted"
	SubjectEventbusAgentPoll      = "agent_poll"
	SubjectEventbusAgentCall      = "agent_call"
	SubjectEventbusMCPCall        = "mcp_call"
	SubjectEventbusConvoCancel    = "convo_cancel"
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
	agentSvc.addResponder(ChannelAgentbus+":"+SubjectAgentbusRPCInterrupted, handleRPCInterrupted)
	agentSvc.addResponder(dipper.ChannelEventbus+":"+SubjectEventbusAgentRecover, handleAgentRecover)
	agentSvc.addResponder(dipper.ChannelEventbus+":"+SubjectEventbusAgentPoll, handleAgentPoll)
	agentSvc.addResponder(dipper.ChannelEventbus+":"+SubjectEventbusAgentCall, handleAgentCall)
	agentSvc.addResponder(dipper.ChannelEventbus+":"+SubjectEventbusMCPCall, handleMCPCall)
	agentSvc.addResponder(dipper.ChannelEventbus+":"+SubjectEventbusConvoCancel, handleConvoCancel)

	// Build the store before start() so the package-level var is set
	// before any goroutine spawned by a responder can reference it.
	uiURL := os.Getenv("HD_UI_URL")
	if uiURL == "" {
		dipper.Logger.Errorf("agent service starting without HD_UI_URL env var; util__hd_get_convo_url will error")
	}

	agentStore = agent.NewAgentStore(agentSvc, uiURL)
	setupAgentAPIs()
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
		for _, t := range ag.Tools {
			if t.Type == "mcp" {
				dynamicData["driver:mcp"] = nil
			}
		}
	}

	return dynamicData
}

// agentRoute forwards eventbus messages (tool-call dispatches emitted by agent sessions)
// to the eventbus driver so the operator service can pick them up.
// Agentbus messages from AI drivers are handled entirely by responders and do not need routing.
func agentRoute(msg *dipper.Message) []RoutedMessage {
	if msg.Channel == dipper.ChannelEventbus {
		if msg.Subject == "agent_command" || msg.Subject == "agent_workflow" || msg.Subject == "agent_response" {
			if bus := agentSvc.getDriverRuntime(dipper.ChannelEventbus); bus != nil {
				return []RoutedMessage{{driverRuntime: bus, message: msg}}
			}
		}
	}

	return nil
}

func handleAgentStart(_ *driver.Runtime, msg *dipper.Message) {
	resumeKey := msg.Labels["resume_key"]
	if resumeKey == "" {
		dipper.Logger.Panicf("agent received start request without resume key %+v", msg.Labels)
	}
	defer dipper.SafeExitOnError("[agent] error in handleAgentStart", func(r interface{}) {
		agentStore.EmitMessage(dipper.Message{
			Channel: dipper.ChannelEventbus,
			Subject: "agent_response",
			Labels: map[string]string{
				"resume_key": resumeKey,
				"status":     "error",
				"reason":     fmt.Sprintf("%v", r),
			},
		})
	})
	<-agentSvc.Ready()
	msg = dipper.DeserializePayload(msg)
	if err := agentStore.StartInference(msg); err != nil {
		// StartInference returns ErrConvoExpiredNoAgent (and other errors) when
		// it cannot recover. The defer above will emit the error response.
		// We panic here to trigger the defer's error handler.
		panic(err)
	}
}

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
	agentStore.ReceiveInference(msg)
}

// handleRPCInterrupted handles an eventbus:rpc/interrupted notification from an AI-model
// driver whose context was cancelled mid-inference. If the message carries an
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

// handleAgentCall handles an eventbus:agent_call dispatched by an agent session
// when a model invokes a sub-agent as a tool.
func handleAgentCall(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] error in handleAgentCall", func(r interface{}) {
		agentStore.EmitMessage(dipper.Message{
			Channel: dipper.ChannelEventbus,
			Subject: dipper.EventbusAgentContinue,
			Labels: map[string]string{
				"agent_session_id": msg.Labels["agent_session_id"],
				"turn_id":          msg.Labels["turn_id"],
				"tool_call_id":     msg.Labels["tool_call_id"],
				"status":           "failure",
				"reason":           fmt.Sprintf("%v", r),
			},
		})
	})
	<-agentSvc.Ready()
	msg = dipper.DeserializePayload(msg)
	go agentStore.StartAgentCall(msg)
}

// handleMCPCall handles an eventbus:mcp_call dispatched by an agent session
// when a model invokes a remote MCP server tool.
func handleMCPCall(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] error in handleMCPCall", func(r interface{}) {
		agentStore.EmitMessage(dipper.Message{
			Channel: dipper.ChannelEventbus,
			Subject: dipper.EventbusAgentContinue,
			Labels: map[string]string{
				"agent_session_id": msg.Labels["agent_session_id"],
				"turn_id":          msg.Labels["turn_id"],
				"tool_call_id":     msg.Labels["tool_call_id"],
				"status":           "failure",
				"reason":           fmt.Sprintf("%v", r),
			},
		})
	})
	<-agentSvc.Ready()
	msg = dipper.DeserializePayload(msg)
	go agentStore.StartMCPCall(msg)
}

func handleAgentPoll(_ *driver.Runtime, msg *dipper.Message) {
	resumeKey := msg.Labels["resume_key"]
	if resumeKey == "" {
		dipper.Logger.Panicf("[agent] poll request received without resume key %+v", msg.Labels)
	}
	defer dipper.SafeExitOnError("[agent] error in handleAgentPoll", func(r interface{}) {
		agentStore.EmitMessage(dipper.Message{
			Channel: dipper.ChannelEventbus,
			Subject: "agent_response",
			Labels: map[string]string{
				"resume_key": resumeKey,
				"status":     "error",
				"reason":     fmt.Sprintf("%v", r),
			},
		})
	})
	<-agentSvc.Ready()

	msg = dipper.DeserializePayload(msg)
	go agentStore.PollInference(msg)
}

// handleConvoCancel receives an eventbus:convo_cancel message and marks the
// conversation as cancelled so that all active sessions abort on their next
// poll cycle.
func handleConvoCancel(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] error in handleConvoCancel")
	<-agentSvc.Ready()
	msg = dipper.DeserializePayload(msg)
	go agentStore.CancelConvo(msg)
}
