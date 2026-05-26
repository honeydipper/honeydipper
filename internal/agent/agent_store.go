package agent

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
)

// AgentStore is the interface used by agent sessions to interact with the store.
type AgentStore interface {
	StartInference(msg *dipper.Message)
	PollInference(msg *dipper.Message)
	ContinueInference(msg *dipper.Message)
	ReceiveInference(msg *dipper.Message)
	StartAgentCall(msg *dipper.Message)

	GetAgent(name string) *config.Agent
	GetSystem(name string) *config.System
	GetWorkflow(name string) *config.Workflow
	EmitMessage(msg dipper.Message)
	GetConfig() *config.Config

	// Stop signals the store to stop accepting new sessions and returns immediately.
	// Call Wait to block until all in-flight sessions have drained.
	Stop()
	// Wait blocks until all in-flight sessions are done.
	Wait()

	// GetLogger returns the logger used by the agent store and sessions.
	GetLogger() *logging.Logger

	dipper.RPCCaller
}

// StoreHelper defines the methods PersistentAgentStore requires from the service layer.
// internal/service.Service satisfies this interface, keeping the agent package free of
// any dependency on internal/service.
type StoreHelper interface {
	dipper.RPCCaller
	GetConfig() *config.Config
	EmitMessage(msg dipper.Message)
}

// PersistentAgentStore implements AgentStore by delegating to a StoreHelper.
type PersistentAgentStore struct {
	StoreHelper
	*logging.Logger
	wg      sync.WaitGroup
	stopped atomic.Bool
}

// GetLogger returns the logger used by the agent store and sessions.
func (p *PersistentAgentStore) GetLogger() *logging.Logger {
	return p.Logger
}

// emitErrorResponse sends an agent_response with status=error back to the workflow
// session identified by callerID. It is a no-op when callerID is empty.
func (p *PersistentAgentStore) emitErrorResponse(msg *dipper.Message, reason string) {
	resumeKey := msg.Labels["resume_key"]
	if resumeKey == "" {
		p.Warningf("[agent] cannot emit error response without resume key in message %+v", msg.Labels)

		return
	}
	p.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "agent_response",
		Labels: map[string]string{
			"resume_key": resumeKey,
			"status":     "error",
			"reason":     reason,
		},
	})
}

// StartInference handles an incoming inference request by creating and running a new agent session.
func (p *PersistentAgentStore) StartInference(msg *dipper.Message) {
	p.wg.Add(1)
	defer p.wg.Done()
	resumeKey := msg.Labels["resume_key"]
	defer dipper.SafeExitOnError("[agent] error in handleAgentStart", func(r interface{}) {
		p.emitErrorResponse(msg, fmt.Sprintf("%v", r))
	})
	p.Infof("[agent] StartInference agent=%s", msg.Labels["agent_name"])
	s := &AgentSession{}
	s.setup(msg, p, true)
	p.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "agent_response",
		Labels: map[string]string{
			"resume_key":       resumeKey,
			"agent_session_id": s.ID,
		},
	})
	defer s.persist(true)
	defer dipper.SafeExitOnError("[agent] error in running inference for %s", resumeKey, func(r interface{}) {
		if s.ErrorReason == "" {
			s.ErrorReason = fmt.Sprintf("%v", r)
		}
		s.notifyParentFailure(s.ErrorReason)
	})
	s.run()
}

// ContinueInference handles an incoming inference continuation request by creating and running a new agent session.
func (p *PersistentAgentStore) ContinueInference(msg *dipper.Message) {
	p.wg.Add(1)
	defer p.wg.Done()
	defer dipper.SafeExitOnError("[agent] error in ContinueInference")
	p.Infof("[agent] ContinueInference session=%s subject=%s", msg.Labels["agent_session_id"], msg.Subject)
	s := &AgentSession{}
	s.setup(msg, p, true)
	defer s.persist(true)
	defer dipper.SafeExitOnError("[agent] error in ContinueInference", func(r interface{}) {
		if s.ErrorReason == "" {
			s.ErrorReason = fmt.Sprintf("%v", r)
		}
		s.notifyParentFailure(s.ErrorReason)
	})

	if msg.Labels["recover"] == "true" {
		s.recover()
	} else {
		s.processToolResult(msg)
	}
}

// ReceiveInference handles an incoming inference message by loading the corresponding agent session and appending
// the message to the conversation history.
func (p *PersistentAgentStore) ReceiveInference(msg *dipper.Message) {
	p.wg.Add(1)
	defer p.wg.Done()
	defer dipper.SafeExitOnError("[agent] error in ReceiveInference")
	p.Infof("[agent] ReceiveInference session=%s", msg.Labels["agent_session_id"])
	s := &AgentSession{}
	s.setup(msg, p, true)
	defer s.persist(true)
	defer dipper.SafeExitOnError("[agent] error in process agent response", func(r interface{}) {
		if s.ErrorReason == "" {
			s.ErrorReason = fmt.Sprintf("%v", r)
		}
		s.notifyParentFailure(s.ErrorReason)
	})
	s.processAgentResponse(msg)
}

// GetAgent returns the named agent from the current configuration.
func (p *PersistentAgentStore) GetAgent(name string) *config.Agent {
	agent := p.GetConfig().DataSet.Agents[name]

	return &agent
}

// GetSystem returns the named system from the current configuration.
func (p *PersistentAgentStore) GetSystem(name string) *config.System {
	system := p.GetConfig().DataSet.Systems[name]

	return &system
}

// GetWorkflow returns the named workflow from the current configuration.
func (p *PersistentAgentStore) GetWorkflow(name string) *config.Workflow {
	workflow := p.GetConfig().DataSet.Workflows[name]

	return &workflow
}

// Wait blocks until all in-flight agent sessions have completed.
func (p *PersistentAgentStore) Wait() {
	p.wg.Wait()
}

// Stop signals the store to reject new sessions and returns immediately.
// Call Wait to block until all in-flight sessions have drained.
func (p *PersistentAgentStore) Stop() {
	p.stopped.Store(true)
}

// NewAgentStore creates an AgentStore backed by the provided StoreHelper.
func NewAgentStore(helper StoreHelper) AgentStore {
	return &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      dipper.GetLogger("agent", "INFO"),
	}
}

// StartAgentCall handles an eventbus:agent_call dispatched by a parent session's tool call.
// It creates a new inference session for the named sub-agent, initiates its first driver call,
// and persists it. The sub-agent session will notify the parent via eventbus:agent_continue
// when it produces its final complete message.
func (p *PersistentAgentStore) StartAgentCall(msg *dipper.Message) {
	p.wg.Add(1)
	defer p.wg.Done()
	defer dipper.SafeExitOnError("[agent] error in StartAgentCall", func(r interface{}) {
		p.EmitMessage(dipper.Message{
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
	p.Infof("[agent] StartAgentCall sub_agent=%s parent=%s", msg.Labels["sub_agent_name"], msg.Labels["agent_session_id"])

	// Build a synthetic start message for the sub-agent session.
	subMsg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": msg.Labels["sub_agent_name"],
		},
		Payload: map[string]interface{}{
			"text": dipper.MustGetMapDataStr(msg.Payload, "input"),
			"type": agentpkg.SessionTypeInference,
		},
	}

	s := &AgentSession{}
	s.setup(subMsg, p, false)
	s.ParentSessionID = msg.Labels["agent_session_id"]
	s.ParentTurnID = msg.Labels["turn_id"]
	s.ParentToolCallID = msg.Labels["tool_call_id"]

	defer s.persist(false)
	s.run()
}

// PollInference handles an incoming poll request for an inference session.
func (p *PersistentAgentStore) PollInference(msg *dipper.Message) {
	p.wg.Add(1)
	defer p.wg.Done()
	defer dipper.SafeExitOnError("[agent] error in handleAgentPoll", func(r interface{}) {
		p.emitErrorResponse(msg, fmt.Sprintf("%v", r))
	})
	p.Infof("[agent] PollInference session=%s", msg.Labels["agent_session_id"])

	s := &AgentSession{}
	s.setup(msg, p, true)
	defer s.persist(true)
	s.processAgentPoll(msg)
}
