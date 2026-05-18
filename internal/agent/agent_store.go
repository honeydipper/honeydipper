package agent

import (
	"sync"
	"sync/atomic"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/op/go-logging"
)

// AgentStore is the interface used by agent sessions to interact with the store.
type AgentStore interface {
	StartInference(msg *dipper.Message)
	ContinueInference(msg *dipper.Message)
	ReceiveInference(msg *dipper.Message)

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
func (p *PersistentAgentStore) emitErrorResponse(callerID, callerType, reason string) {
	if callerID == "" {
		return
	}
	p.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "agent_response",
		Labels: map[string]string{
			"caller_id":   callerID,
			"caller_type": callerType,
			"status":      "error",
			"reason":      reason,
		},
	})
}

// StartInference handles an incoming inference request by creating and running a new agent session.
func (p *PersistentAgentStore) StartInference(msg *dipper.Message) {
	p.wg.Add(1)
	defer p.wg.Done()
	callerID := msg.Labels["caller_id"]
	callerType := msg.Labels["caller_type"]
	defer dipper.SafeExitOnError("[agent] error in StartInference", func(r error) {
		p.emitErrorResponse(callerID, callerType, r.Error())
	})
	p.Infof("[agent] StartInference agent=%s", msg.Labels["agent_name"])
	s := &AgentSession{}
	s.setup(msg, p)
	s.run()
}

// ContinueInference handles an incoming inference continuation request by creating and running a new agent session.
func (p *PersistentAgentStore) ContinueInference(msg *dipper.Message) {
	p.wg.Add(1)
	defer p.wg.Done()
	p.Infof("[agent] ContinueInference session=%s subject=%s", msg.Labels["agent_session_id"], msg.Subject)
	s := &AgentSession{}
	defer dipper.SafeExitOnError("[agent] error in ContinueInference", func(r error) {
		p.emitErrorResponse(s.CallerID, s.CallerType, r.Error())
	})
	s.setup(msg, p)
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
	s.setup(msg, p)
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
