package agent

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

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
	StartMCPCall(msg *dipper.Message)
	CancelConvo(msg *dipper.Message)
	// StartTurn fires a new chat turn on an existing conversation without a
	// return path back to a workflow. It is used by the UI-initiated turn API.
	// engine and driver are optional overrides; empty strings mean "no override".
	StartTurn(convoID, text, user, engine, driver string)
	// StartNewConvo starts a brand-new conversation for the named agent and
	// returns the generated convo_id synchronously. The session runs asynchronously.
	// engine and driver are optional overrides; empty strings mean "no override".
	StartNewConvo(agentName, text, user, engine, driver string) string

	GetAgent(name string) *config.Agent
	GetSystem(name string) *config.System
	GetWorkflow(name string) *config.Workflow
	EmitMessage(msg dipper.Message)
	GetConfig() *config.Config
	GetUIURL() string

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
	uiURL   string
}

// GetLogger returns the logger used by the agent store and sessions.
func (p *PersistentAgentStore) GetLogger() *logging.Logger {
	return p.Logger
}

// GetUIURL returns the base URL for the Honeydipper UI.
func (p *PersistentAgentStore) GetUIURL() string {
	return p.uiURL
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

	// Acquire the distributed turn lock after setup so s.ConvoID is populated.
	// The lock is held for the entire turn lifecycle and released by
	// syncConvoStateStatus() when the session reaches a terminal state.
	s.TurnLockKey = ConvoTurnLockPrefix + s.ConvoID
	dipper.Must(p.Call("locker", "lock", map[string]interface{}{
		"name":   s.TurnLockKey,
		"expire": "1h",
	}, "timeout", "30m"))

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

// StartTurn starts a new chat turn on an existing conversation without a workflow
// return path. The agent name is inferred from the most recent session recorded in
// the ConvoState. The turn runs asynchronously; errors are logged, not propagated.
// engine and driver are optional overrides; empty strings mean "no override".
func (p *PersistentAgentStore) StartTurn(convoID, text, user, engine, driver string) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer dipper.SafeExitOnError("[agent] error in StartTurn")

		// Resolve the agent name from the ConvoState.
		// Sub-agent (inference) sessions register themselves into the unified
		// ConvoState and can overwrite LastSession with a sub-agent name.
		// To guard against starting a new turn with the wrong agent, prefer
		// the last ChatTurn session; fall back to FirstSession when LastSession
		// is an inference session.
		cs := &ConvoState{}
		cs.load(convoID, p)
		agentName := ""
		switch {
		case cs.LastSession != nil && cs.LastSession.Type != AgentSessionTypeInference:
			agentName = cs.LastSession.AgentName
		case cs.FirstSession != nil:
			agentName = cs.FirstSession.AgentName
		}
		if agentName == "" {
			p.Errorf("[agent] StartTurn: cannot determine agent name for convo %s", convoID)

			return
		}

		p.Infof("[agent] StartTurn convo=%s agent=%s", convoID, agentName)
		p.runTurn(agentName, convoID, text, user, engine, driver)
	}()
}

// StartNewConvo starts a brand-new conversation for the named agent and returns
// the generated convo_id synchronously. The session runs asynchronously.
// engine and driver are optional overrides; empty strings mean "no override".
func (p *PersistentAgentStore) StartNewConvo(agentName, text, user, engine, driver string) string {
	convoID := dipper.NewUUID()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer dipper.SafeExitOnError("[agent] error in StartNewConvo")

		p.Infof("[agent] StartNewConvo convo=%s agent=%s", convoID, agentName)
		p.runTurn(agentName, convoID, text, user, engine, driver)
	}()

	return convoID
}

// runTurn creates and runs a single chat-turn session for the given agent and
// conversation. It must be called from within a goroutine that has already
// incremented p.wg; it does NOT add/done the WaitGroup itself.
func (p *PersistentAgentStore) runTurn(agentName, convoID, text, user, engine, driver string) {
	payload := map[string]interface{}{
		"text":     text,
		"user":     user,
		"convo_id": convoID,
	}
	// Pass optional engine/driver overrides through the message payload
	// so initNewSession can pick them up.
	if engine != "" {
		payload["engine"] = engine
	}
	if driver != "" {
		payload["driver"] = driver
	}

	msg := &dipper.Message{
		Labels: map[string]string{
			"agent_name": agentName,
		},
		Payload: payload,
	}

	s := &AgentSession{}
	s.setup(msg, p, true)

	// Acquire the distributed turn lock after setup so s.ConvoID is populated.
	// The lock is held for the entire turn lifecycle and released automatically
	// by syncConvoStateStatus() when the session reaches a terminal state.
	s.TurnLockKey = ConvoTurnLockPrefix + s.ConvoID
	dipper.Must(p.Call("locker", "lock", map[string]interface{}{
		"name":   s.TurnLockKey,
		"expire": "1h",
	}, "timeout", "30m"))

	defer s.persist(true)
	defer dipper.SafeExitOnError("[agent] error running turn for convo "+convoID, func(r interface{}) {
		if s.ErrorReason == "" {
			s.ErrorReason = fmt.Sprintf("%v", r)
		}
	})
	s.run()
}

// NewAgentStore creates an AgentStore backed by the provided StoreHelper.
func NewAgentStore(helper StoreHelper, uiURL string) AgentStore {
	return &PersistentAgentStore{
		StoreHelper: helper,
		Logger:      dipper.GetLogger("agent", "INFO"),
		uiURL:       uiURL,
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
	forgetHistory, _ := dipper.GetMapDataBool(msg.Payload, "forget_history")
	// Check if a convo_id was pre-generated by the caller (handleAgentToolCall).
	// If present, use it directly; otherwise fall back to the existing generation logic.
	convoID, _ := dipper.GetMapDataStr(msg.Payload, "convo_id")
	if convoID == "" {
		oneShot, _ := dipper.GetMapDataBool(msg.Payload, "one_shot")
		if !oneShot {
			convoID = fmt.Sprintf("%s-%s", msg.Labels["convo_id"], msg.Labels["sub_agent_name"])
		}
	}
	compactID, _ := dipper.GetMapDataStr(msg.Payload, "compaction_id")
	if compactID != "" {
		convoID = compactID
	}
	sessionType, _ := dipper.GetMapDataStr(msg.Payload, "session_type")
	if sessionType == "" {
		sessionType = agentpkg.SessionTypeChatTurn
	}
	if sessionType != agentpkg.SessionTypeChatTurn && sessionType != agentpkg.SessionTypeInference {
		p.Warningf("[agent] StartAgentCall got invalid session_type %s, defaulting to chat_turn", sessionType)
		sessionType = agentpkg.SessionTypeChatTurn
	}
	subMsg := &dipper.Message{
		Labels: map[string]string{
			"agent_name":       msg.Labels["sub_agent_name"],
			"unified_convo_id": msg.Labels["unified_convo_id"],
		},
		Payload: map[string]interface{}{
			"text":             dipper.MustGetMapDataStr(msg.Payload, "input"),
			"type":             sessionType,
			"convo_id":         convoID,
			"unified_convo_id": msg.Labels["unified_convo_id"],
			"forget_history":   forgetHistory,
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

// StartMCPCall handles an eventbus:mcp_call dispatched by an agent session's tool call.
// It forwards the call synchronously to the MCP driver via RPC and emits the result as
// an eventbus:agent_continue message back to the originating session.
func (p *PersistentAgentStore) StartMCPCall(msg *dipper.Message) {
	p.wg.Add(1)
	defer p.wg.Done()
	defer dipper.SafeExitOnError("[agent] error in StartMCPCall", func(r interface{}) {
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

	msg = dipper.DeserializePayload(msg)
	serverName := dipper.MustGetMapDataStr(msg.Payload, "server")
	toolName := dipper.MustGetMapDataStr(msg.Payload, "tool")
	args, _ := dipper.GetMapData(msg.Payload, "args")

	p.Infof("[agent] StartMCPCall server=%s tool=%s session=%s", serverName, toolName, msg.Labels["agent_session_id"])

	result, callErr := p.Call("driver:mcp", "call_tool", map[string]interface{}{
		"server": serverName,
		"tool":   toolName,
		"args":   args,
	})

	status := "success"
	reason := ""
	var output interface{}

	if callErr != nil {
		status = "failure"
		reason = callErr.Error()
	} else {
		var resultMap map[string]interface{}
		if jsonErr := json.Unmarshal(result, &resultMap); jsonErr == nil {
			output = resultMap["output"]
		}
	}

	p.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: dipper.EventbusAgentContinue,
		Labels: map[string]string{
			"agent_session_id": msg.Labels["agent_session_id"],
			"turn_id":          msg.Labels["turn_id"],
			"tool_call_id":     msg.Labels["tool_call_id"],
			"status":           status,
			"reason":           reason,
		},
		Payload: map[string]interface{}{
			"data": map[string]interface{}{"output": output},
		},
	})
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

// CancelConvo marks the conversation identified by convo_id or unified_convo_id as cancelled.
// Active sessions belonging to the conversation will detect this flag on their
// next poll cycle and abort with a "conversation cancelled" error.
func (p *PersistentAgentStore) CancelConvo(msg *dipper.Message) {
	p.wg.Add(1)
	defer p.wg.Done()
	defer dipper.SafeExitOnError("[agent] error in CancelConvo")

	convoID := msg.Labels["convo_id"]
	if convoID == "" {
		convoID, _ = dipper.GetMapDataStr(msg.Payload, "convo_id")
	}
	unifiedConvoID := msg.Labels["unified_convo_id"]
	if unifiedConvoID == "" {
		unifiedConvoID, _ = dipper.GetMapDataStr(msg.Payload, "unified_convo_id")
	}

	if convoID == "" && unifiedConvoID == "" {
		p.Warningf("[agent] CancelConvo called without convo_id or unified_convo_id")

		return
	}

	cancelOne := func(id string) {
		lockedConvoStateUpdate(id, p, func(cs *ConvoState) {
			now := time.Now()
			for _, sr := range []*ConvoSessionRef{cs.FirstSession, cs.LastSession, cs.ActiveSession} {
				if sr != nil && sr.Status == ConvoSessionStatusActive {
					sr.Status = ConvoSessionStatusCancelled
					sr.UpdatedAt = now
				}
			}
		})
	}

	if convoID != "" {
		p.Infof("[agent] CancelConvo convo=%s", convoID)
		cancelOne(convoID)
	}
	if unifiedConvoID != "" && unifiedConvoID != convoID {
		p.Infof("[agent] CancelConvo unified_convo=%s", unifiedConvoID)
		cancelOne(unifiedConvoID)
	}
}
