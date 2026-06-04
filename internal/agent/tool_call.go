package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

var ErrToolCall = errors.New("tool call error")

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

	if strings.Contains(s.Agent.SystemPrompt, SkillsHeader) {
		tools["hd_load_skill"] = AgentTool{
			Name:        "hd_load_skill",
			Description: "Load a skill's content into the agent session.",
			Params: map[string]interface{}{
				"skill_name": map[string]interface{}{
					"name":        "skill_name",
					"type":        "string",
					"description": "The name of the skill to load, e.g. file_ops/read_file.",
				},
			},
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

		schema, _ := def["input_schema"].(map[string]interface{})
		props, _ := schema["properties"].(map[string]interface{})

		tools[fname] = AgentTool{
			Name:        fname,
			Description: desc,
			Params:      props,
		}
	}
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
	case c.FuncName == "hd_load_skill":
		s.handleLoadSkillToolCall(c, unifiedConvoID)
	default:
		panic(fmt.Errorf("%w unknown tool call prefix: %s", ErrToolCall, c.FuncName))
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
	c := s.ToolCalls[s.CurrentCall]
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

	case strings.HasPrefix(c.FuncName, "wf__") || c.FuncName == "hd_load_skill":
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

		if s.handlePreContextAndSkillsResult(c, s.ToolResults) {
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
