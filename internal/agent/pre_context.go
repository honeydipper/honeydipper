package agent

import (
	"bytes"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const PreContextHeader = "Read below first to understand the user's request:\n"

func (s *AgentSession) loadPreContext() bool {
	if len(s.history) > 0 || len(s.Agent.FileTool) == 0 {
		return false
	}

	preContext := []string{}
	for _, pc := range s.Agent.PreContext {
		if pc != "" {
			preContext = append(preContext, pc)
		}
	}

	if len(preContext) == 0 {
		return false
	}

	toolCall := AgentToolCall{
		FuncName: s.Agent.FileTool,
		Params: map[string]interface{}{
			"file_specs":  preContext,
			"pre_context": true,
		},
	}

	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] loading pre-context via tool=%s len=%d", s.ID, toolCall.FuncName, len(preContext))
	}

	// Kick off the tool call from this session.
	s.CurrentCall = 0
	s.ToolCalls = []AgentToolCall{toolCall}
	s.nextToolCall()

	return true
}

func (s *AgentSession) handlePreContextResult(c AgentToolCall, results []map[string]interface{}) bool {
	if isPreContext, _ := dipper.GetMapDataBool(c.Params, "pre_context"); !isPreContext {
		return false
	}

	status, _ := dipper.GetMapDataStr(results, "0.status")
	// so what there is a nested if.
	if status != "success" && status != "" { //nolint:nestif
		if log := s.log(); log != nil {
			log.Warningf("[agent] session [%s] failed to load pre-context, tool call result=%+v", s.ID, results)
		}
	} else {
		log := s.log()
		buf := bytes.Buffer{}
		if content, ok := dipper.GetMapData(results, "0.data.files"); ok {
			if contentList, isList := content.([]interface{}); isList {
				for i, v := range contentList {
					fname, _ := dipper.GetMapDataStr(v, "file_spec")
					content, _ := dipper.GetMapDataStr(v, "file_content")
					if log != nil {
						log.Infof("[agent] session [%s] loaded pre-context file=%d file_name=%s content_len=%d", s.ID, i, fname, len(content))
					}
					if content != "" {
						dipper.Must(buf.WriteString(content + "\n\n"))
					}
				}
			}
		}

		text := buf.String()
		if len(text) > 0 {
			s.Agent.SystemPrompt += "\n\n" + PreContextHeader + text
			s.Agent.InferencePrompt += "\n\n" + PreContextHeader + text
		}
	}
	s.Agent.PreContext = nil // prevent retrying on next session run
	s.Agent.FileTool = ""
	s.CurrentCall = 0
	s.ToolCalls = nil
	s.ToolResults = nil
	s.persist(false)
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.Agent = s.Agent
	})

	s.run()

	return true
}
