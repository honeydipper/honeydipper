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
	ID           string
	ConvoID      string
	Agent        *config.Agent
	history      []AgentMessage
	CurrentMsg   *dipper.Message
	Type         string
	TTL          string
	CurrentCall  int
	ToolCalls    []AgentToolCall
	ToolResults  []map[string]interface{}
	LastPoll     int
	LastPollTime time.Time
	ErrorReason  string

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
		dipper.Must(s.store.Call("locker", "unlock", map[string]interface{}{
			"name": AgentKeyPrefix + s.ID,
		}))
	}
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
func (s *AgentSession) appendConvoHistory(msg AgentMessage) {
	s.history = append(s.history, msg)

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

	if len(s.history) == 0 {
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
	timeout := s.CurrentMsg.Labels["timeout"]
	if timeout == "" {
		timeout = AgentSessionDefaultTimeout
	}
	s.appendConvoHistory(AgentMessage{
		Role:    RoleUser,
		User:    user,
		Content: text,
	})
	s.LastPoll = len(s.history)

	s.persist(true)
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] sending to driver=%s engine=%s history_len=%d tools=%d",
			s.ID, s.Agent.Driver, s.Agent.Engine, len(s.history), len(tools))
	}
	dipper.Must(s.store.CallNoWait("driver:"+s.Agent.Driver, "send_to_model", map[string]interface{}{
		"engine":        s.Agent.Engine,
		"history":       s.history,
		"tools":         tools,
		"type":          s.Type,
		"model_data":    s.Agent.ModelData,
		"should_stream": s.Agent.ShouldStream,
	}, "agent_session_id", s.ID, "timeout", timeout))
}

func (s *AgentSession) recover() {
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] recovering from cache after restart", s.ID)
	}

	tools := s.BuildTools()
	var timeout string
	if s.CurrentMsg != nil {
		timeout = s.CurrentMsg.Labels["timeout"]
	}
	if timeout == "" {
		timeout = AgentSessionDefaultTimeout
	}
	dipper.Must(s.store.CallNoWait("driver:"+s.Agent.Driver, "send_to_model", map[string]interface{}{
		"engine":        s.Agent.Engine,
		"history":       s.history,
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
	var agentMsg AgentMessage
	dipper.Must(mapstructure.Decode(m, &agentMsg))
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] response received role=%s thinking=%v tool_calls=%d",
			s.ID, agentMsg.Role, agentMsg.IsThinking, len(agentMsg.ToolCalls))
	}
	s.processAgentMessage(&agentMsg)
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

	if agentMsg.Role != RoleAgent {
		s.run()
	} else {
		s.persist(true)
	}
}

// nextToolCall dispatches the next pending tool call to the appropriate system or workflow.
func (s *AgentSession) nextToolCall() {
	if s.CurrentCall >= len(s.ToolCalls) {
		return
	}

	s.persist(true)

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
				"turn_id":          strconv.Itoa(len(s.history)),
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
						"turn_id":          strconv.Itoa(len(s.history)),
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
		for (s.LastPoll == len(s.history) && s.ErrorReason == "") || (!s.LastPollTime.IsZero() && time.Since(s.LastPollTime) < MinPollInterval) {
			select {
			case <-ctx.Done():
				log.Warningf("[agent] session [%s] poll timeout after %s", s.ID, timeout)
				labels := msg.Labels
				labels["status"] = "failure"
				labels["reason"] = "poll timeout after " + timeout.String()
				dipper.Must(s.store.Call("locker", "unlock", map[string]interface{}{
					"name": AgentKeyPrefix + s.ID,
				}))
				s.store.EmitMessage(dipper.Message{
					Channel: dipper.ChannelEventbus,
					Subject: "agent_response",
					Labels:  labels,
				})

				return
			default:
			}
			dipper.Must(s.store.Call("locker", "unlock", map[string]interface{}{
				"name": AgentKeyPrefix + s.ID,
			}))

			time.Sleep(time.Second)
			s.setup(msg, s.store, true)
		}

		if s.emitPollResponse(msg) {
			return
		}
	}
}

func (s *AgentSession) emitPollResponse(msg *dipper.Message) bool {
	buf := strings.Builder{}
	if marker, ok := msg.Labels["last_poll"]; ok {
		s.LastPoll = dipper.Must(strconv.Atoi(marker)).(int)
	}
	for i := s.LastPoll; i < len(s.history); i++ {
		am := s.history[i]
		if am.Role != RoleAgent {
			continue
		}

		if am.IsThinking && !s.Agent.ShouldStream {
			continue
		}

		buf.WriteString(am.Content)
	}

	labels := msg.Labels
	am := AgentMessage{
		Role:    RoleAgent,
		Content: buf.String(),
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
	labels["last_poll"] = strconv.Itoa(s.LastPoll)
	if am.Content == "" && s.ErrorReason == "" {
		s.persist(false)

		return false
	}

	s.store.EmitMessage(dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "agent_response",
		Labels:  labels,
		Payload: map[string]interface{}{
			"message": am,
		},
	})
	s.LastPollTime = time.Now()
	s.persist(true)

	return true
}
