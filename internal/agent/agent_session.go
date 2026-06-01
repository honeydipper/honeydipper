package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/op/go-logging"
)

// Session type constants, cache key prefixes, default TTL, and role labels.
const (
	AgentSessionTypeInference      = agentpkg.SessionTypeInference
	AgentSessionTypeChatTurn       = agentpkg.SessionTypeChatTurn
	AgentKeyPrefix                 = "agent_session:"
	ConvoHistoryKeyPrefix          = "convo_history:"
	AgentSessionDefaultTTL         = "72h"
	AgentSessionDefaultTimeout     = "1h"
	AgentSessionDefaultPollTimeout = time.Second * 9
	MinPollInterval                = time.Second * 2

	RoleSystem     = agentpkg.RoleSystem
	RoleUser       = agentpkg.RoleUser
	RoleAgent      = agentpkg.RoleAgent
	RoleTool       = agentpkg.RoleTool
	RoleToolResult = agentpkg.RoleToolResult
)

// AgentSession holds the runtime state of a single agent inference or chat-turn session.
type AgentSession struct {
	ID               string
	ConvoID          string
	UnifiedConvoID   string
	Agent            *config.Agent
	history          []AgentMessage
	CurrentMsg       *dipper.Message
	Type             string
	TTL              string
	CurrentCall      int
	ToolCalls        []AgentToolCall
	ToolResults      []map[string]interface{}
	LastPoll         int
	LastPollTime     time.Time
	PendingContent   string
	PendingOffset    int
	ErrorReason      string
	PrevContextSize  int
	TotalTokens      int
	InputTokens      int
	OutputTokens     int
	ParentSessionID  string
	ParentTurnID     string
	ParentToolCallID string

	store AgentStore
}

// Type aliases so internal code can use the short names unchanged.
type (
	AgentMessage  = agentpkg.Message
	AgentTool     = agentpkg.Tool
	AgentToolCall = agentpkg.ToolCall
	AgentState    = agentpkg.State
)

// log returns the logger from the store, or nil if unavailable.
func (s *AgentSession) log() *logging.Logger {
	return s.store.GetLogger()
}

// unlock releases the session lock in the store.
func (s *AgentSession) unlock() {
	dipper.Must(s.store.Call("locker", "unlock", map[string]interface{}{
		"name": AgentKeyPrefix + s.ID,
	}))
}

// persist serialises the session and writes it to the cache.
// When unlocking is true the session lock is released and, if the session
// has reached a terminal state, the shared ConvoState is updated too.
func (s *AgentSession) persist(unlocking bool) {
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] persisting to cache", s.ID)
	}
	// add locking when parallel access is expected
	dipper.Must(s.store.Call("cache", "save", map[string]interface{}{
		"key":   AgentKeyPrefix + s.ID,
		"value": string(dipper.SerializeContent(s)),
		"ttl":   s.TTL,
	}))

	if unlocking {
		s.unlock()
		s.syncConvoStateStatus()
	}
}

// syncConvoStateStatus updates the session's status in the shared ConvoState(s) when
// the session has reached a terminal state (complete or failed).
// It is called after the session lock has been released to avoid nested locking.
func (s *AgentSession) syncConvoStateStatus() {
	var status string
	switch {
	case s.ErrorReason != "":
		status = ConvoSessionStatusFailed
	case len(s.history) > 0 && s.history[len(s.history)-1].IsComplete:
		status = ConvoSessionStatusComplete
	default:
		return // not yet terminal
	}

	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		if log := s.log(); log != nil {
			lastIsComplete := false
			if len(s.history) > 0 {
				lastIsComplete = s.history[len(s.history)-1].IsComplete
			}
			fs, ls := "<nil>", "<nil>"
			if cs.FirstSession != nil {
				fs = cs.FirstSession.SessionID
			}
			if cs.LastSession != nil {
				ls = cs.LastSession.SessionID
			}
			log.Debugf("[agent] session [%s] syncConvoStateStatus convo=%s status=%s lastIsComplete=%v error=%q firstSession=%s lastSession=%s",
				s.ID, s.ConvoID, status, lastIsComplete, s.ErrorReason, fs, ls)
		}

		cs.updateSessionStatus(s.ID, status, s.InputTokens, s.OutputTokens, s.TotalTokens)
	})
}

// checkCancelled returns true when this session has been marked as cancelled
// in its own ConvoState or in the shared unified ConvoState.
// Cancellation is checked per-session so that only the active turn is aborted;
// future turns (new sessions) start fresh with an active status and are unaffected.
func (s *AgentSession) checkCancelled() bool {
	if s.ConvoID == "" || s.ID == "" {
		return false
	}
	cs := &ConvoState{}
	cs.load(s.ConvoID, s.store)
	if cs.isSessionCancelled(s.ID) {
		return true
	}
	if s.UnifiedConvoID != "" && s.UnifiedConvoID != s.ConvoID {
		ucs := &ConvoState{}
		ucs.load(s.UnifiedConvoID, s.store)

		return ucs.isSessionCancelled(s.ID)
	}

	return false
}

// load reads and deserialises a previously persisted session from the cache.
func (s *AgentSession) load(id string, store AgentStore) {
	if log := store.GetLogger(); log != nil {
		log.Debugf("[agent] session [%s] loading from cache", id)
	}
	ret := dipper.Must(store.Call("cache", "load", map[string]interface{}{
		"key": AgentKeyPrefix + id,
	})).([]byte)

	dipper.Must(json.Unmarshal(ret, s))
}

// setup initialises a new session or restores an existing one from cache.
// Returns the conversation ID.
func (s *AgentSession) setup(msg *dipper.Message, store AgentStore, locking bool) {
	id, ok := msg.Labels["agent_session_id"]
	if !ok || id == "" {
		id = dipper.NewUUID()
	}

	if locking {
		dipper.Must(store.Call("locker", "lock", map[string]interface{}{
			"name":   AgentKeyPrefix + id,
			"expire": "600s",
		}))
	}

	if ok && id != "" {
		s.load(id, store)
		s.store = store
		s.loadConvoHistory()
	} else {
		s.initNewSession(id, msg, store)
	}

	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] created type=%s agent=%s", s.ID, s.Type, msg.Labels["agent_name"])
	}
}

// initNewSession populates a freshly created session from the incoming message.
func (s *AgentSession) initNewSession(id string, msg *dipper.Message, store AgentStore) {
	s.store = store
	s.CurrentMsg = msg
	s.ID = id
	s.Type, _ = dipper.GetMapDataStr(msg.Payload, "type")
	if s.Type == "" {
		s.Type = agentpkg.SessionTypeChatTurn
	}

	s.Agent = s.store.GetAgent(msg.Labels["agent_name"])
	s.TTL = AgentSessionDefaultTTL
	if ttl, ok := dipper.GetMapDataStr(msg.Payload, "ttl"); ok && ttl != "" {
		s.TTL = ttl
	}

	if convoID, ok := dipper.GetMapDataStr(msg.Payload, "convo_id"); ok && convoID != "" {
		s.ConvoID = convoID
		s.loadConvoHistory()
	} else {
		s.ConvoID = dipper.NewUUID()
	}

	// Capture the unified conversation ID; fall back to the session's own convo ID.
	s.UnifiedConvoID = msg.Labels["unified_convo_id"]
	if s.UnifiedConvoID == "" {
		if u, ok := dipper.GetMapDataStr(msg.Payload, "unified_convo_id"); ok && u != "" {
			s.UnifiedConvoID = u
		}
	}
	if s.UnifiedConvoID == "" {
		s.UnifiedConvoID = s.ConvoID
	}

	// Register this session in the shared conversation state so that it can be
	// queried for display and subject to controls such as cancellation.
	agentName := msg.Labels["agent_name"]
	firstTurn, _ := dipper.GetMapDataStr(msg.Payload, "text")
	if len(firstTurn) > 200 {
		firstTurn = firstTurn[:200]
	}
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		if cs.TTL == "" {
			cs.TTL = ConvoStreamTTL
		}
		cs.UnifiedConvoID = s.UnifiedConvoID
		if cs.FirstTurn == "" && s.Type == AgentSessionTypeChatTurn && firstTurn != "" {
			cs.FirstTurn = firstTurn
		}
		if len(s.history) > 0 {
			forgetHistory, _ := dipper.GetMapDataBool(msg.Payload, "forget_history")
			if forgetHistory {
				dipper.Must(cs.archiveConvo(store))
				dipper.Must(s.store.Call("cache", "del", map[string]interface{}{
					"key": ConvoHistoryKeyPrefix + s.ConvoID,
				}))
			}
		}
		if cs.LastSession != nil {
			s.PrevContextSize = cs.LastSession.InputTokens + cs.LastSession.OutputTokens
		}
		cs.registerSession(s.ID, agentName, s.Type, true)
	})
	// Also register in the unified convo state when it spans multiple convo IDs
	// (i.e. sub-agent sessions whose convo_id differs from unified_convo_id).
	if s.UnifiedConvoID != s.ConvoID {
		lockedConvoStateUpdate(s.UnifiedConvoID, s.store, func(cs *ConvoState) {
			if cs.TTL == "" {
				cs.TTL = ConvoStreamTTL
			}
			if cs.UnifiedConvoID == "" {
				cs.UnifiedConvoID = s.UnifiedConvoID
			}
			cs.registerSession(s.ID, agentName, s.Type, false)
		})
	}
}

// loadConvoHistory fetches the multi-turn conversation history from the cache.
func (s *AgentSession) loadConvoHistory() {
	if s.ConvoID == "" {
		return
	}

	ret := dipper.Must(s.store.Call("cache", "lrange", map[string]interface{}{
		"key": ConvoHistoryKeyPrefix + s.ConvoID,
	})).([]byte)

	var history []AgentMessage
	dipper.Must(json.Unmarshal(ret, &history))
	s.history = history
}

// appendConvoHistory appends a message to the in-memory history and the cache.
// If the agent has MaxHistoryLen set, older entries beyond that limit are trimmed.
func (s *AgentSession) appendConvoHistory(msg AgentMessage) {
	s.history = append(s.history, msg)

	if s.ConvoID != "" {
		convoTTL, _ := time.ParseDuration(ConvoStreamTTL)
		dipper.Must(s.store.Call("cache", "rpush", map[string]interface{}{
			"key":   ConvoHistoryKeyPrefix + s.ConvoID,
			"value": string(dipper.SerializeContent(msg)),
			"ttl":   float64(convoTTL), // nanoseconds; compatible with old and new rpush handler
		}))

		if s.Agent != nil && s.Agent.MaxHistoryLen > 0 && len(s.history) > s.Agent.MaxHistoryLen {
			s.history = s.history[len(s.history)-s.Agent.MaxHistoryLen:]
			dipper.Must(s.store.Call("cache", "ltrim", map[string]interface{}{
				"key":   ConvoHistoryKeyPrefix + s.ConvoID,
				"start": -s.Agent.MaxHistoryLen,
				"stop":  -1,
			}))
		}
	}
}

// run appends the current user message and dispatches the conversation to the AI driver.
func (s *AgentSession) run() {
	text := dipper.MustGetMapDataStr(s.CurrentMsg.Payload, "text")
	user, _ := dipper.GetMapDataStr(s.CurrentMsg.Payload, "user")
	s.appendConvoHistory(AgentMessage{
		Role:    RoleUser,
		User:    user,
		Content: text,
	})
	s.LastPoll = len(s.history)

	s.sendToDriver()
}

func (s *AgentSession) recover() {
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] recovering from cache after restart", s.ID)
	}

	s.sendToDriver()
}

func (s *AgentSession) sendToDriver() {
	tools := s.BuildTools()
	timeout := s.CurrentMsg.Labels["timeout"]
	if timeout == "" {
		timeout = AgentSessionDefaultTimeout
	}

	// Build the system prompt from the current agent config (inference vs chat).
	systemPrompt := s.Agent.SystemPrompt
	if s.Type == AgentSessionTypeInference && len(s.Agent.InferencePrompt) > 0 {
		systemPrompt = s.Agent.InferencePrompt
	}

	// Run compaction if the agent's policy indicates it's due. If compaction
	// dispatched a summarizer sub-agent, quit the send so the session waits
	// for the compaction result delivered via agent_continue.
	if s.shouldCompact() {
		if s.compactHistory() {
			if log := s.log(); log != nil {
				log.Infof("[agent] session [%s] compaction dispatched; waiting for summarizer", s.ID)
			}

			return
		}
	}

	// Prepend the system prompt ephemerally; filter any legacy persisted system
	// messages so the driver always sees exactly one, up-to-date system entry.
	history := make([]AgentMessage, 0, len(s.history)+1)
	history = append(history, AgentMessage{Role: RoleSystem, Content: systemPrompt})
	for _, m := range s.history {
		if m.Role != RoleSystem {
			history = append(history, m)
		}
	}

	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] sending to driver=%s engine=%s history_len=%d tools=%d",
			s.ID, s.Agent.Driver, s.Agent.Engine, len(history), len(tools))
	}
	s.InputTokens = 0
	s.OutputTokens = 0
	dipper.Must(s.store.CallNoWait("driver:"+s.Agent.Driver, "send_to_model", map[string]interface{}{
		"engine":        s.Agent.Engine,
		"history":       history,
		"tools":         tools,
		"type":          s.Type,
		"model_data":    s.Agent.ModelData,
		"should_stream": s.Agent.ShouldStream,
	}, "agent_session_id", s.ID, "timeout", timeout))
}

// BuildTools assembles the tool map from the agent's configured system and workflow tools.
func (s *AgentSession) BuildTools() map[string]AgentTool {
	tools := map[string]AgentTool{}

	for _, toolDef := range s.Agent.Tools {
		switch toolDef.Type {
		case "system":
			s.addSystemTool(tools, toolDef)
		case "workflow":
			s.addWorkflowTool(tools, toolDef)
		case "agent":
			s.addAgentTool(tools, toolDef)
		case "mcp":
			s.addMCPTool(tools, toolDef)
		}
	}

	return tools
}

// addSystemTool exposes each function of a named system as a callable tool.
func (s *AgentSession) addSystemTool(tools map[string]AgentTool, toolDef config.AgentToolDef) {
	prefix := "sys_" + toolDef.Name + "__"

	sys := s.store.GetSystem(toolDef.Name)
	for k, f := range sys.Functions {
		fname := prefix + k
		if _, exists := tools[fname]; exists {
			continue
		}

		if f.Meta == nil {
			continue
		}
		meta := f.Meta.(map[string]interface{})
		if skip, ok := dipper.GetMapData(meta, "skip_agent"); ok && dipper.IsTruthy(skip) {
			continue
		}

		desc := ""
		if d, ok := meta["description"]; ok {
			desc = d.(string)
		}

		params := map[string]interface{}{}
		inputs := meta["inputs"].([]interface{})
		for _, v := range inputs {
			def := v.(map[string]interface{})

			pname := def["name"].(string)
			ptype := "string"
			if t, ok := def["type"]; ok {
				ptype = t.(string)
			}

			params[pname] = map[string]interface{}{
				"name":        pname,
				"type":        ptype,
				"description": def["description"],
			}
		}

		tools[fname] = AgentTool{
			Name:        fname,
			Description: desc,
			Params:      params,
		}
	}
}

// addWorkflowTool registers a named workflow as a callable tool.
func (s *AgentSession) addWorkflowTool(tools map[string]AgentTool, toolDef config.AgentToolDef) {
	fname := "wf__" + toolDef.Name
	if _, exists := tools[fname]; exists {
		return
	}

	wf := s.store.GetWorkflow(toolDef.Name)
	meta := wf.Meta.(map[string]interface{})
	desc := wf.Description
	if desc == "" {
		if d, ok := meta["description"]; ok {
			desc = d.(string)
		}
	}
	params := map[string]interface{}{}
	inputs := meta["inputs"].([]interface{})
	for _, v := range inputs {
		def := v.(map[string]interface{})

		pname := def["name"].(string)
		ptype := "string"
		if t, ok := def["type"]; ok {
			ptype = t.(string)
		}

		params[pname] = map[string]interface{}{
			"name":        pname,
			"type":        ptype,
			"description": def["description"],
		}
	}

	tools[fname] = AgentTool{
		Name:        fname,
		Description: desc,
		Params:      params,
	}
}

// addAgentTool registers a named agent as a callable tool.
func (s *AgentSession) addAgentTool(tools map[string]AgentTool, toolDef config.AgentToolDef) {
	fname := "ag__" + toolDef.Name
	if _, exists := tools[fname]; exists {
		return
	}

	ag := s.store.GetAgent(toolDef.Name)
	desc := ag.Description
	if desc == "" {
		desc = "Call the " + toolDef.Name + " agent"
	}

	tools[fname] = AgentTool{
		Name:        fname,
		Description: desc,
		Params: map[string]interface{}{
			"input": map[string]interface{}{
				"name":        "input",
				"type":        "string",
				"description": "The input text to send to the agent",
			},
			"forget_history": map[string]interface{}{
				"name": "forget_history",
				"type": "boolean",
				"description": "Whether to forget the agent's previous conversation history, " +
					"if you have been using sticky conversation(one_shot: false).",
			},
			"one_shot": map[string]interface{}{
				"name": "one_shot",
				"type": "boolean",
				"description": "Whether to run the agent in one-shot mode, forgetting " +
					"this call afterwards.",
			},
		},
	}
}

// addMCPTool registers all tools exposed by a named remote MCP server as callable tools.
// It calls the MCP driver's list_tools RPC synchronously and creates one AgentTool per
// remote tool, named mcp__<serverName>__<toolName>.
func (s *AgentSession) addMCPTool(tools map[string]AgentTool, toolDef config.AgentToolDef) {
	prefix := "mcp__" + toolDef.Name + "__"

	raw, err := s.store.Call("driver:mcp", "list_tools", map[string]interface{}{
		"server": toolDef.Name,
	})
	if err != nil {
		if log := s.log(); log != nil {
			log.Errorf("[agent] session [%s] failed to list tools for MCP server %q: %v",
				s.ID, toolDef.Name, err)
		}

		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		if log := s.log(); log != nil {
			log.Errorf("[agent] session [%s] failed to decode list_tools response for %q: %v",
				s.ID, toolDef.Name, err)
		}

		return
	}

	toolList, _ := payload["tools"].([]interface{})
	for _, item := range toolList {
		def, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := def["name"].(string)
		desc, _ := def["description"].(string)
		if name == "" {
			continue
		}

		fname := prefix + name
		if _, exists := tools[fname]; exists {
			continue
		}

		params := map[string]interface{}{}
		schema, _ := def["input_schema"].(map[string]interface{})
		props, _ := schema["properties"].(map[string]interface{})
		for pname, pval := range props {
			prop, ok := pval.(map[string]interface{})
			if !ok {
				continue
			}

			ptype, _ := prop["type"].(string)
			if ptype == "" {
				ptype = "string"
			}
			pdesc, _ := prop["description"].(string)
			params[pname] = map[string]interface{}{
				"name":        pname,
				"type":        ptype,
				"description": pdesc,
			}
		}

		tools[fname] = AgentTool{
			Name:        fname,
			Description: desc,
			Params:      params,
		}
	}
}

// notifyParentFailure emits an agent_continue failure message to the parent session.
// It is a no-op when ParentSessionID is empty.
func (s *AgentSession) notifyParentFailure(reason string) {
	if s.ParentSessionID == "" {
		return
	}
	s.store.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: dipper.EventbusAgentContinue,
		Labels: map[string]string{
			"agent_session_id": s.ParentSessionID,
			"turn_id":          s.ParentTurnID,
			"tool_call_id":     s.ParentToolCallID,
			"status":           "failure",
			"reason":           reason,
		},
	})
}

// notifyParent emits an agent_continue message to the parent session when a sub-agent
// session completes as a tool call.
func (s *AgentSession) notifyParent(agentMsg AgentMessage) {
	s.store.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: dipper.EventbusAgentContinue,
		Labels: map[string]string{
			"agent_session_id": s.ParentSessionID,
			"turn_id":          s.ParentTurnID,
			"tool_call_id":     s.ParentToolCallID,
			"status":           "success",
		},
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"output": agentMsg.Content,
			},
		},
	})
}

// coerceToolCallParams fixes parameter values where the declared type is "object"
// or "array" but the LLM sent them as a JSON-encoded string. Only parameters with
// a clear type mismatch are coerced; everything else is left unchanged.
func (s *AgentSession) coerceToolCallParams(toolCalls []AgentToolCall) {
	tools := s.BuildTools()
	for i := range toolCalls {
		tool, ok := tools[toolCalls[i].FuncName]
		if !ok {
			continue
		}
		for pname, pval := range toolCalls[i].Params {
			pdef, ok := tool.Params[pname]
			if !ok {
				continue
			}
			def, ok := pdef.(map[string]interface{})
			if !ok {
				continue
			}
			ptype, _ := def["type"].(string)
			if ptype != "object" && ptype != "array" {
				continue
			}
			strVal, ok := pval.(string)
			if !ok {
				continue
			}
			var parsed interface{}
			if err := json.Unmarshal([]byte(strVal), &parsed); err == nil {
				toolCalls[i].Params[pname] = parsed //nolint:gosec // i is always a valid index from range
			}
		}
	}
}

// processAgentResponse decodes the model's response message and hands it to processAgentMessage.
func (s *AgentSession) processAgentResponse(msg *dipper.Message) {
	// Honour a cancellation that arrived while the model was running.
	if s.checkCancelled() {
		s.ErrorReason = "conversation cancelled"
		s.notifyParentFailure(s.ErrorReason)

		return
	}
	m := dipper.MustGetMapData(msg.Payload, "message").(map[string]interface{})
	var agentMsg AgentMessage
	dipper.Must(mapstructure.Decode(m, &agentMsg))
	s.coerceToolCallParams(agentMsg.ToolCalls)
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] response received role=%s thinking=%v tool_calls=%d",
			s.ID, agentMsg.Role, agentMsg.IsThinking, len(agentMsg.ToolCalls))
	}

	if msg.Labels["status"] != "success" && msg.Labels["status"] != "" {
		s.ErrorReason = msg.Labels["reason"]
		s.notifyParentFailure(s.ErrorReason)

		return
	}
	s.processAgentMessage(&agentMsg)
}

// processAgentMessage routes a decoded model message: triggers tool calls, handles thinking
// tokens, or re-runs the session when the model produces a final user-facing reply.
func (s *AgentSession) processAgentMessage(agentMsg *AgentMessage) {
	// Streaming chunk: non-complete agent content with no tool calls and not a thinking token.
	// Accumulate in PendingContent to avoid one Redis rpush per chunk.
	s.InputTokens += agentMsg.InputTokens
	s.OutputTokens += agentMsg.OutputTokens
	s.TotalTokens += agentMsg.InputTokens + agentMsg.OutputTokens

	if agentMsg.Role == RoleAgent && !agentMsg.IsComplete && !agentMsg.IsThinking && len(agentMsg.ToolCalls) == 0 {
		s.PendingContent += agentMsg.Content

		return
	}

	// Final agent message: merge PendingContent into a single history entry.
	// PendingOffset is intentionally NOT reset here so that emitPollResponse
	// does not re-send content that was already streamed to the engine.
	if agentMsg.Role == RoleAgent && agentMsg.IsComplete {
		agentMsg.Content = s.PendingContent + agentMsg.Content
		s.PendingContent = ""
		s.appendConvoHistory(*agentMsg)
		if s.ParentSessionID != "" {
			s.notifyParent(*agentMsg)
		}

		return
	}

	// Everything else: thinking tokens, tool calls, non-agent messages.
	s.appendConvoHistory(*agentMsg)
	if len(agentMsg.ToolCalls) > 0 {
		s.CurrentCall = 0
		s.ToolCalls = agentMsg.ToolCalls
		s.nextToolCall()

		return
	}

	if agentMsg.Role != RoleAgent {
		s.persist(false)
		s.sendToDriver()
	}
}

// nextToolCall dispatches the next pending tool call to the appropriate system or workflow.
func (s *AgentSession) nextToolCall() {
	if s.CurrentCall >= len(s.ToolCalls) {
		return
	}

	c := s.ToolCalls[s.CurrentCall]
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] dispatching tool call %d/%d: %s",
			s.ID, s.CurrentCall+1, len(s.ToolCalls), c.FuncName)
	}

	unifiedConvoID := s.CurrentMsg.Labels["unified_convo_id"]
	if unifiedConvoID == "" {
		unifiedConvoID, _ = dipper.GetMapDataStr(s.CurrentMsg.Payload, "convo_id")
	}
	if unifiedConvoID == "" {
		unifiedConvoID = s.ConvoID
	}

	switch {
	case strings.HasPrefix(c.FuncName, "sys_"):
		s.handleSysToolCall(c, unifiedConvoID)
	case strings.HasPrefix(c.FuncName, "wf__"):
		s.handleWorkflowToolCall(c, unifiedConvoID)
	case strings.HasPrefix(c.FuncName, "ag__"):
		s.handleAgentToolCall(c, unifiedConvoID)
	case strings.HasPrefix(c.FuncName, "mcp__"):
		s.handleMCPToolCall(c, unifiedConvoID)
	}
}

func (s *AgentSession) handleSysToolCall(c AgentToolCall, unifiedConvoID string) {
	sysName := c.FuncName[len("sys_"):strings.LastIndex(c.FuncName, "__")]
	fName := c.FuncName[strings.LastIndex(c.FuncName, "__")+2:]

	s.store.EmitMessage(dipper.Message{
		Channel: "eventbus",
		Subject: "agent_command",
		Labels: map[string]string{
			"agent_session_id": s.ID,
			"turn_id":          strconv.Itoa(len(s.history)),
			"tool_call_id":     strconv.Itoa(s.CurrentCall),
			"unified_convo_id": unifiedConvoID,
			"agent_name":       s.Agent.Name,
		},
		Payload: map[string]interface{}{
			"ctx": c.Params,
			"function": map[string]interface{}{
				"target": map[string]interface{}{
					"system":   sysName,
					"function": fName,
				},
			},
		},
	})
}

func (s *AgentSession) handleWorkflowToolCall(c AgentToolCall, unifiedConvoID string) {
	wfName := c.FuncName[len("wf__"):]

	s.store.EmitMessage(dipper.Message{
		Channel: "eventbus",
		Subject: "agent_workflow",
		Payload: map[string]interface{}{
			"data": c.Params,
			"do": map[string]interface{}{
				"call_workflow": wfName,
				"context":       "_agent_tool_call",
				"with": map[string]interface{}{
					"agent_session_id": s.ID,
					"turn_id":          strconv.Itoa(len(s.history)),
					"convo_id":         s.ConvoID,
					"unified_convo_id": unifiedConvoID,
				},
			},
			"message": map[string]interface{}{
				"labels": map[string]string{
					"agent_session_id": s.ID,
					"turn_id":          strconv.Itoa(len(s.history)),
					"tool_call_id":     strconv.Itoa(s.CurrentCall),
					"unified_convo_id": unifiedConvoID,
					"agent_name":       s.Agent.Name,
				},
			},
		},
	})
}

func (s *AgentSession) handleAgentToolCall(c AgentToolCall, unifiedConvoID string) {
	subAgentName := c.FuncName[len("ag__"):]
	input := c.Params["input"]
	oneShot, _ := dipper.GetMapDataBool(c.Params, "one_shot")
	forgetHistory, _ := dipper.GetMapDataBool(c.Params, "forget_history")
	m := dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "agent_call",
		Labels: map[string]string{
			"agent_session_id": s.ID,
			"turn_id":          strconv.Itoa(len(s.history)),
			"tool_call_id":     strconv.Itoa(s.CurrentCall),
			"sub_agent_name":   subAgentName,
			"convo_id":         s.ConvoID,
			"unified_convo_id": unifiedConvoID,
			"agent_name":       s.Agent.Name,
		},
		Payload: map[string]interface{}{
			"input":          input,
			"one_shot":       oneShot,
			"forget_history": forgetHistory,
		},
	}
	if compactID, ok := dipper.GetMapDataStr(c.Params, "compaction_id"); ok && compactID != "" {
		m.Payload.(map[string]interface{})["compaction_id"] = compactID
	}

	s.store.EmitMessage(m)
}

func (s *AgentSession) handleMCPToolCall(c AgentToolCall, unifiedConvoID string) {
	rest := c.FuncName[len("mcp__"):]
	sep := strings.Index(rest, "__")
	var mcpServer, mcpTool string
	if sep >= 0 {
		mcpServer = rest[:sep]
		mcpTool = rest[sep+2:]
	} else {
		mcpServer = rest
	}

	s.store.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "mcp_call",
		Labels: map[string]string{
			"agent_session_id": s.ID,
			"turn_id":          strconv.Itoa(len(s.history)),
			"tool_call_id":     strconv.Itoa(s.CurrentCall),
			"unified_convo_id": unifiedConvoID,
			"agent_name":       s.Agent.Name,
		},
		Payload: map[string]interface{}{
			"server": mcpServer,
			"tool":   mcpTool,
			"args":   c.Params,
		},
	})
}

// processToolResult collects the result of a completed tool call and either advances to the
// next pending call or feeds all results back to the model.
func (s *AgentSession) processToolResult(msg *dipper.Message) {
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] tool result received call=%d subject=%s",
			s.ID, s.CurrentCall, msg.Subject)
	}

	turn_id, _ := strconv.Atoi(msg.Labels["turn_id"])

	if turn_id != len(s.history) {
		if log := s.log(); log != nil {
			log.Warningf("[agent] session [%s] received out-of-order tool result for turn %d (current turn is %d)",
				s.ID, turn_id, len(s.history))
		}

		return
	}

	tool_call_id, _ := strconv.Atoi(msg.Labels["tool_call_id"])
	if tool_call_id != s.CurrentCall {
		if log := s.log(); log != nil {
			log.Warningf("[agent] session [%s] received out-of-order tool result for call %d (current call is %d)",
				s.ID, tool_call_id, s.CurrentCall)
		}

		return
	}

	var data interface{}
	c := s.history[len(s.history)-1].ToolCalls[s.CurrentCall]
	switch {
	case strings.HasPrefix(c.FuncName, "sys_"):
		sysName := c.FuncName[len("sys_"):strings.LastIndex(c.FuncName, "__")]
		fName := c.FuncName[strings.LastIndex(c.FuncName, "__")+2:]
		sys := s.store.GetSystem(sysName)
		f := sys.Functions[fName]

		data = config.ExportFunctionContext(&f, map[string]interface{}{
			"data":   msg.Payload,
			"ctx":    c.Params,
			"labels": msg.Labels,
		}, s.store.GetConfig())

	case strings.HasPrefix(c.FuncName, "wf__"):
		data, _ = dipper.GetMapData(msg.Payload, "data.output")

	case strings.HasPrefix(c.FuncName, "ag__"):
		data, _ = dipper.GetMapData(msg.Payload, "data.output")
		lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
			cs.ActiveSession = cs.LastSession
		})
	case strings.HasPrefix(c.FuncName, "mcp__"):
		data, _ = dipper.GetMapData(msg.Payload, "data.output")
	}

	result := map[string]interface{}{
		"status":    msg.Labels["status"],
		"reason":    msg.Labels["reason"],
		"data":      data,
		"func_name": c.FuncName,
	}

	s.ToolResults = append(s.ToolResults, result)
	s.CurrentCall++

	if s.CurrentCall < len(s.ToolCalls) {
		s.nextToolCall()
	} else {
		// Delegate compaction-specific handling to the compaction helper.
		if s.handleCompactionResult(c, s.ToolResults) {
			return
		}

		agentMsg := AgentMessage{
			Role:       RoleToolResult,
			ToolResult: s.ToolResults,
		}
		s.ToolResults = nil

		s.processAgentMessage(&agentMsg)
	}
}

func (s *AgentSession) processAgentPoll(msg *dipper.Message) {
	log := s.log()
	if log == nil {
		log = dipper.GetLogger("agent", "INFO")
	}

	if s.LastPoll == len(s.history) {
		if s.LastPoll > 0 && s.history[s.LastPoll-1].IsComplete {
			log.Panicf("[agent] session [%s] poll after completion", s.ID)
		}
	}
	log.Infof("[agent] session [%s] poll received lastpoll=%d resume_key %s", s.ID, s.LastPoll, msg.Labels["resume_key"])

	timeout := AgentSessionDefaultPollTimeout
	if t, ok := msg.Labels["timeout"]; ok {
		timeout = dipper.Must(time.ParseDuration(t)).(time.Duration)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		noNewContent := s.LastPoll == len(s.history) && len(s.PendingContent) <= s.PendingOffset
		rateLimited := !s.LastPollTime.IsZero() && time.Since(s.LastPollTime) < MinPollInterval
		for (noNewContent && s.ErrorReason == "") || rateLimited {
			select {
			case <-ctx.Done():
				log.Warningf("[agent] session [%s] poll timeout after %s", s.ID, timeout)
				labels := msg.Labels
				labels["status"] = "failure"
				labels["reason"] = "poll timeout after " + timeout.String()
				s.store.EmitMessage(dipper.Message{
					Channel: dipper.ChannelEventbus,
					Subject: "agent_response",
					Labels:  labels,
				})

				return
			default:
			}
			s.unlock()

			time.Sleep(time.Second)
			s.setup(msg, s.store, true)

			// Check whether the conversation was cancelled while waiting.
			if s.checkCancelled() {
				s.ErrorReason = "conversation cancelled"

				break
			}

			noNewContent = s.LastPoll == len(s.history) && len(s.PendingContent) <= s.PendingOffset
			rateLimited = !s.LastPollTime.IsZero() && time.Since(s.LastPollTime) < MinPollInterval
		}

		if s.emitPollResponse(msg) {
			return
		}
	}
}

func (s *AgentSession) emitPollResponse(msg *dipper.Message) bool {
	// Build the full content available since LastPoll:
	// PendingContent (not-yet-committed streaming chunks) plus any agent entries
	// that have already been committed to history.
	fullContent := s.PendingContent
	for i := s.LastPoll; i < len(s.history); i++ {
		am := s.history[i]
		if am.Role != RoleAgent {
			continue
		}

		if am.IsThinking && !s.Agent.ShouldStream {
			continue
		}

		fullContent += am.Content
	}

	// PendingOffset tracks how much of fullContent has already been sent to
	// the engine in previous poll responses.
	newContent := ""
	if len(fullContent) > s.PendingOffset {
		newContent = fullContent[s.PendingOffset:]
	}

	labels := msg.Labels
	am := AgentMessage{
		Role:    RoleAgent,
		Content: newContent,
	}

	if h := len(s.history); h > 0 {
		am.IsComplete = s.history[h-1].IsComplete
	}

	if s.ErrorReason != "" {
		am.IsComplete = true
		labels["status"] = "error"
		labels["reason"] = s.ErrorReason
	} else {
		labels["status"] = "ok"
	}

	s.LastPoll = len(s.history)
	if newContent == "" && s.ErrorReason == "" && !am.IsComplete {
		s.persist(false)

		return false
	}

	s.PendingOffset += len(newContent)
	if am.IsComplete || s.ErrorReason != "" {
		// Final poll response: reset tracking state.
		s.PendingOffset = 0
		s.PendingContent = ""
	}

	labels["last_poll"] = strconv.Itoa(s.LastPoll)
	s.store.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "agent_response",
		Labels:  labels,
		Payload: map[string]interface{}{
			"message": am,
			"state": AgentState{
				HistoryLen:  len(s.history),
				TotalTokens: s.TotalTokens,
				ConvoID:     s.ConvoID,
			},
		},
	})
	s.LastPollTime = time.Now()

	return true
}
