package agent

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/op/go-logging"
)

// Session type constants, cache key prefixes, default TTL, and role labels.
const (
	AgentSessionTypeInference = agentpkg.SessionTypeInference
	AgentSessionTypeChatTurn  = agentpkg.SessionTypeChatTurn
	AgentKeyPrefix            = "agent_session:"
	ConvoHistoryKeyPrefix     = "convo_history:"
	AgentSessionDefaultTTL    = "1h"

	RoleSystem     = agentpkg.RoleSystem
	RoleUser       = agentpkg.RoleUser
	RoleAgent      = agentpkg.RoleAgent
	RoleTool       = agentpkg.RoleTool
	RoleToolResult = agentpkg.RoleToolResult
)

// AgentSession holds the runtime state of a single agent inference or chat-turn session.
type AgentSession struct {
	ID          string
	CallerID    string
	CallerType  string
	ConvoID     string
	Agent       *config.Agent
	History     []AgentMessage
	CurrentMsg  *dipper.Message
	Type        string
	TTL         string
	CurrentCall int
	SessionSeq  int
	ChunkSeq    int
	ToolCalls   []AgentToolCall
	ToolResults []map[string]interface{}

	store AgentStore
}

// Type aliases so internal code can use the short names unchanged.
type (
	AgentMessage  = agentpkg.Message
	AgentTool     = agentpkg.Tool
	AgentToolCall = agentpkg.ToolCall
)

// log returns the logger from the store, or nil if unavailable.
func (s *AgentSession) log() *logging.Logger {
	return s.store.GetLogger()
}

// persist serialises the session and writes it to the cache.
func (s *AgentSession) persist() {
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] persisting to cache", s.ID)
	}
	// add locking when parallel access is expected
	dipper.Must(s.store.Call("cache", "save", map[string]interface{}{
		"key":   AgentKeyPrefix + s.ID,
		"value": string(dipper.SerializeContent(s)),
		"ttl":   s.TTL,
	}))
}

// load reads and deserialises a previously persisted session from the cache.
func (s *AgentSession) load(id string) {
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] loading from cache", id)
	}
	ret := dipper.Must(s.store.Call("cache", "load", map[string]interface{}{
		"key": AgentKeyPrefix + id,
	})).([]byte)

	dipper.Must(json.Unmarshal(ret, s))
}

// setup initialises a new session or restores an existing one from cache.
// Returns the conversation ID.
func (s *AgentSession) setup(msg *dipper.Message, store AgentStore) {
	s.store = store
	if id, ok := msg.Labels["agent_session_id"]; ok && id != "" {
		s.load(id)
		s.CurrentMsg = msg

		return
	}

	s.ID = dipper.NewUUID()
	s.CallerID = msg.Labels["caller_id"]
	s.CallerType = msg.Labels["caller_type"]

	s.Type, _ = dipper.GetMapDataStr(msg.Payload, "type")
	if s.Type == "" {
		s.Type = agentpkg.SessionTypeChatTurn
	}

	if convoID, ok := dipper.GetMapDataStr(msg.Payload, "convo_id"); ok && convoID != "" {
		s.ConvoID = convoID
		s.loadConvoHistory()
	} else {
		s.ConvoID = dipper.NewUUID()
	}

	s.Agent = s.store.GetAgent(msg.Labels["agent_name"])
	s.CurrentMsg = msg

	s.TTL = AgentSessionDefaultTTL
	if ttl, ok := dipper.GetMapDataStr(msg.Payload, "ttl"); ok {
		s.TTL = ttl
	}
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] created type=%s agent=%s", s.ID, s.Type, msg.Labels["agent_name"])
	}
}

// loadConvoHistory fetches the multi-turn conversation history from the cache.
func (s *AgentSession) loadConvoHistory() {
	ret := dipper.Must(s.store.Call("cache", "lrange", map[string]interface{}{
		"key": ConvoHistoryKeyPrefix + s.ConvoID,
	})).([]byte)

	var history []AgentMessage
	dipper.Must(json.Unmarshal(ret, &history))

	s.History = history
}

// appendConvoHistory appends a message to the in-memory history and the cache.
func (s *AgentSession) appendConvoHistory(msg AgentMessage) {
	s.History = append(s.History, msg)

	if s.ConvoID != "" {
		dipper.Must(s.store.Call("cache", "rpush", map[string]interface{}{
			"key":   ConvoHistoryKeyPrefix + s.ConvoID,
			"value": string(dipper.SerializeContent(msg)),
		}))
	}
}

// run appends the current user message and dispatches the conversation to the AI driver.
func (s *AgentSession) run() {
	tools := s.BuildTools()

	if len(s.History) == 0 {
		systemPrompt := s.Agent.SystemPrompt
		if s.Type == AgentSessionTypeInference && len(s.Agent.InferencePrompt) > 0 {
			systemPrompt = s.Agent.InferencePrompt
		}
		s.appendConvoHistory(AgentMessage{
			Role:    RoleSystem,
			Content: systemPrompt,
		})
	}

	text := dipper.MustGetMapDataStr(s.CurrentMsg.Payload, "text")
	user, _ := dipper.GetMapDataStr(s.CurrentMsg.Payload, "user")
	s.appendConvoHistory(AgentMessage{
		Role:    RoleUser,
		User:    user,
		Content: text,
	})

	s.persist()
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] sending to driver=%s engine=%s history_len=%d tools=%d",
			s.ID, s.Agent.Driver, s.Agent.Engine, len(s.History), len(tools))
	}
	dipper.Must(s.store.CallNoWait("driver:"+s.Agent.Driver, "send_to_model", map[string]interface{}{
		"engine":        s.Agent.Engine,
		"history":       s.History,
		"tools":         tools,
		"type":          s.Type,
		"model_data":    s.Agent.ModelData,
		"should_stream": s.Agent.ShouldStream,
	}, "agent_session_id", s.ID, "chunk_sq", strconv.Itoa(s.ChunkSeq), "timeout", s.TTL))
}

func (s *AgentSession) recover() {
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] recovering from cache after restart", s.ID)
	}

	tools := s.BuildTools()
	dipper.Must(s.store.CallNoWait("driver:"+s.Agent.Driver, "send_to_model", map[string]interface{}{
		"engine":        s.Agent.Engine,
		"history":       s.History,
		"tools":         tools,
		"type":          s.Type,
		"model_data":    s.Agent.ModelData,
		"should_stream": s.Agent.ShouldStream,
	}, "agent_session_id", s.ID, "chunk_sq", strconv.Itoa(s.ChunkSeq), "timeout", s.TTL))
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

		meta := f.Meta.(map[string]interface{})
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

// processAgentResponse decodes the model's response message and hands it to processAgentMessage.
func (s *AgentSession) processAgentResponse(msg *dipper.Message) {
	s.CurrentMsg = msg
	m := dipper.MustGetMapData(msg.Payload, "message").(map[string]interface{})

	if msg.Labels["status"] != "" && msg.Labels["status"] != "success" {
		errRet := dipper.Message{
			Channel: "eventbus",
			Subject: "agent_response",
			Labels: map[string]string{
				"agent_session_id": s.ID,
				"convo_id":         s.ConvoID,
				"caller_id":        s.CallerID,
				"caller_type":      s.CallerType,
				"session_seq":      strconv.Itoa(s.SessionSeq),
				"status":           msg.Labels["status"],
				"reason":           msg.Labels["reason"],
			},
		}
		if s.SessionSeq > 0 {
			s.SessionSeq++
			errRet.Labels["session_seq"] = strconv.Itoa(s.SessionSeq)
		}
		s.store.EmitMessage(errRet)

		return
	}

	var agentMsg AgentMessage
	dipper.Must(mapstructure.Decode(m, &agentMsg))
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] response received role=%s thinking=%v tool_calls=%d",
			s.ID, agentMsg.Role, agentMsg.IsThinking, len(agentMsg.ToolCalls))
	}
	if agentMsg.ChunkSeq > 0 {
		dipper.Must(s.store.Call("locker", "lock", map[string]interface{}{
			"name":   AgentKeyPrefix + "lock:" + s.ID,
			"expire": "30s",
			"sq":     agentMsg.ChunkSeq,
		}))
		s.setup(msg, s.store) // refresh session data in case of updates during streaming
		s.ChunkSeq = agentMsg.ChunkSeq
	}
	s.processAgentMessage(&agentMsg)
	if agentMsg.ChunkSeq > 0 {
		dipper.Must(s.store.Call("locker", "unlock", map[string]interface{}{
			"name": AgentKeyPrefix + "lock:" + s.ID,
		}))
	}
}

// processAgentMessage routes a decoded model message: triggers tool calls, handles thinking
// tokens, or re-runs the session when the model produces a final user-facing reply.
func (s *AgentSession) processAgentMessage(agentMsg *AgentMessage) {
	s.appendConvoHistory(*agentMsg)
	if len(agentMsg.ToolCalls) > 0 {
		s.CurrentCall = 0
		s.ToolCalls = agentMsg.ToolCalls
		s.nextToolCall()

		return
	}

	if !agentMsg.IsThinking || s.Agent.ShouldEmitThoughts {
		msg := dipper.Message{
			Channel: "eventbus",
			Subject: "agent_response",
			Labels: map[string]string{
				"agent_session_id": s.ID,
				"convo_id":         s.ConvoID,
				"caller_id":        s.CallerID,
				"caller_type":      s.CallerType,
			},
			Payload: map[string]interface{}{
				"message": agentMsg,
			},
		}
		if agentMsg.ChunkSeq > 0 {
			s.SessionSeq++
			msg.Labels["session_seq"] = strconv.Itoa(s.SessionSeq)
			s.persist()
		}
		s.store.EmitMessage(msg)
	}

	if agentMsg.IsThinking {
		return
	}

	if agentMsg.Role != RoleAgent {
		s.run()
	}
}

// nextToolCall dispatches the next pending tool call to the appropriate system or workflow.
func (s *AgentSession) nextToolCall() {
	if s.CurrentCall >= len(s.ToolCalls) {
		return
	}

	s.persist()

	c := s.ToolCalls[s.CurrentCall]
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] dispatching tool call %d/%d: %s",
			s.ID, s.CurrentCall+1, len(s.ToolCalls), c.FuncName)
	}
	if strings.HasPrefix(c.FuncName, "sys_") {
		sysName := c.FuncName[len("sys_"):strings.LastIndex(c.FuncName, "__")]
		fName := c.FuncName[strings.LastIndex(c.FuncName, "__")+2:]
		sys := s.store.GetSystem(sysName)
		f := sys.Functions[fName]

		s.store.EmitMessage(dipper.Message{
			Channel: "eventbus",
			Subject: "agent_command",
			Labels: map[string]string{
				"agent_session_id": s.ID,
				"turn_id":          strconv.Itoa(len(s.History)),
				"tool_call_id":     strconv.Itoa(s.CurrentCall),
			},
			Payload: map[string]interface{}{
				"ctx":      c.Params,
				"function": f,
			},
		})
	} else if strings.HasPrefix(c.FuncName, "wf__") {
		wfName := c.FuncName[len("wf__"):]

		s.store.EmitMessage(dipper.Message{
			Channel: "eventbus",
			Subject: "agent_workflow",
			Payload: map[string]interface{}{
				"data": c.Params,
				"do": map[string]interface{}{
					"call_workflow": wfName,
				},
				"message": map[string]interface{}{
					"labels": map[string]string{
						"agent_session_id": s.ID,
						"turn_id":          strconv.Itoa(len(s.History)),
						"tool_call_id":     strconv.Itoa(s.CurrentCall),
					},
				},
			},
		})
	}
}

// processToolResult collects the result of a completed tool call and either advances to the
// next pending call or feeds all results back to the model.
func (s *AgentSession) processToolResult(msg *dipper.Message) {
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] tool result received call=%d subject=%s",
			s.ID, s.CurrentCall, msg.Subject)
	}

	turn_id, _ := strconv.Atoi(msg.Labels["turn_id"])

	if turn_id != len(s.History) {
		if log := s.log(); log != nil {
			log.Warningf("[agent] session [%s] received out-of-order tool result for turn %d (current turn is %d)",
				s.ID, turn_id, len(s.History))
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
	c := s.History[len(s.History)-1].ToolCalls[s.CurrentCall]
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
	}

	result := map[string]interface{}{
		"status": msg.Labels["status"],
		"reason": msg.Labels["reason"],
		"data":   data,
	}

	s.ToolResults = append(s.ToolResults, result)
	s.CurrentCall++

	if s.CurrentCall < len(s.ToolCalls) {
		s.nextToolCall()
	} else {
		agentMsg := AgentMessage{
			Role:       RoleToolResult,
			ToolResult: s.ToolResults,
		}

		s.processAgentMessage(&agentMsg)
	}
}
