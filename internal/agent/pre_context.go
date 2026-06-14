package agent

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"gopkg.in/yaml.v2"
)

const (
	PreContextHeader = "Read below first to understand the user's request:\n"
	SkillsHeader     = "The following skills may be useful to accomplish the user's " +
		"request, you can use hd_load_skill tool to load them when needed:\n"
)

var ErrSkillLoadFailed = errors.New("failed to load skill content")

func (s *AgentSession) loadPreContextAndSkills() bool {
	if len(s.history) > 0 || (len(s.Agent.SkillsPaths) == 0 && len(s.Agent.PreContext) == 0) {
		return false
	}

	fileSpecs := []string{}
	fileSets := map[string]struct{}{} // using a set to avoid duplicates.
	for _, pc := range s.Agent.PreContext {
		_, added := fileSets[pc]
		if pc != "" && !added {
			fileSpecs = append(fileSpecs, pc)
			fileSets[pc] = struct{}{}
		}
	}

	for _, skill := range s.Agent.SkillsPaths {
		_, added := fileSets[skill]
		if skill != "" && !added {
			fileSpecs = append(fileSpecs, strings.TrimSuffix(skill, "/SKILL.md")+"/SKILL.md")
			fileSets[skill] = struct{}{}
		}
	}

	if len(fileSpecs) == 0 {
		return false
	}

	toolCall := AgentToolCall{
		FuncName: s.Agent.FileTool,
		Params: map[string]interface{}{
			"file_specs":  fileSpecs,
			"pre_context": true,
		},
	}

	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] loading pre-context and skills via tool=%s len=%d", s.ID, toolCall.FuncName, len(fileSpecs))
	}

	// Kick off the tool call from this session.
	s.CurrentCall = 0
	s.ToolCalls = []AgentToolCall{toolCall}
	s.nextToolCall()

	return true
}

func (s *AgentSession) handlePreContextAndSkillsResult(c AgentToolCall, results []map[string]interface{}) bool {
	if isPreContext, _ := dipper.GetMapDataBool(c.Params, "pre_context"); !isPreContext {
		return false
	}

	status, _ := dipper.GetMapDataStr(results, "0.status")
	if status != "success" && status != "" {
		if log := s.log(); log != nil {
			log.Warningf("[agent] session [%s] failed to load pre-context, tool call result=%+v", s.ID, results)
		}
		s.resumeWithSkills(nil)

		return true
	}

	content, ok := dipper.GetMapData(results, "0.data.files")
	if !ok {
		s.resumeWithSkills(nil)

		return true
	}
	contentList, isList := content.([]interface{})
	if !isList {
		s.resumeWithSkills(nil)

		return true
	}

	log := s.log()
	contentMap := map[string]string{}
	// convert to a map for easy lookup
	for i, v := range contentList {
		fname, _ := dipper.GetMapDataStr(v, "file_spec")
		content, _ := dipper.GetMapDataStr(v, "file_content")
		if log != nil {
			log.Infof("[agent] session [%s] loaded pre-context file=%d file_name=%s content_len=%d", s.ID, i, fname, len(content))
		}
		if fname != "" && content != "" {
			contentMap[fname] = content
		}
	}

	buf := bytes.Buffer{}
	for _, pc := range s.Agent.PreContext {
		if content, ok := contentMap[pc]; ok {
			buf.WriteString(content)
			buf.WriteString("\n\n")
		}
	}
	text := buf.String()
	if len(text) > 0 {
		s.Agent.SystemPrompt += "\n\n" + PreContextHeader + text
		s.Agent.InferencePrompt += "\n\n" + PreContextHeader + text
	}

	skillMap := map[string]string{}
	skillbuf := bytes.Buffer{}
	for k, v := range contentMap {
		if !strings.HasSuffix(k, "/SKILL.md") {
			continue
		}

		yp := strings.SplitN(v, "---", 3)
		if len(yp) < 2 {
			continue
		}
		ytext := yp[len(yp)-2]

		skill := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(ytext), &skill); err != nil {
			if log := s.log(); log != nil {
				log.Warningf("[agent] session [%s] failed to parse skill content as yaml, skill=%s error=%v", s.ID, k, err)
			}

			continue
		}
		name, _ := dipper.GetMapDataStr(skill, "name")
		description, _ := dipper.GetMapDataStr(skill, "description")
		if name == "" || description == "" {
			continue
		}
		skillbuf.WriteString("* ")
		skillbuf.WriteString(name)
		skillbuf.WriteString(": ")
		skillbuf.WriteString(description)
		skillbuf.WriteString("\n")
		skillMap[name] = k
	}

	skillParagraph := skillbuf.String()
	if len(skillParagraph) > 0 {
		s.Agent.SystemPrompt += "\n\n" + SkillsHeader + skillParagraph
		s.Agent.InferencePrompt += "\n\n" + SkillsHeader + skillParagraph
	}

	s.resumeWithSkills(skillMap)

	return true
}

func (s *AgentSession) resumeWithSkills(skillMap map[string]string) {
	s.Agent.PreContext = nil // prevent retrying on next session run
	s.Agent.SkillsPaths = nil
	s.CurrentCall = 0
	s.ToolCalls = nil
	s.ToolResults = nil
	s.persist(false)
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.Agent = s.Agent
		if len(skillMap) > 0 {
			cs.Skills = skillMap
		}
	})

	s.run()
}

func (s *AgentSession) handleLoadSkillToolCall(c AgentToolCall, unifiedConvoID string) {
	skillName, _ := dipper.GetMapDataStr(c.Params, "skill_name")
	path, _ := dipper.GetMapDataStr(c.Params, "path")
	if path == "" {
		path = "/SKILL.md"
	}

	path = "/" + strings.TrimPrefix(path, "/")

	if skillName == "" {
		if log := s.log(); log != nil {
			log.Warningf("[agent] session [%s] hd_load_skill call missing skill_name param", s.ID)
		}
		panic(fmt.Errorf("%w: missing skill_name parameter", ErrSkillLoadFailed))
	}

	var skillMap map[string]string
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		skillMap = cs.Skills
	})
	if skillMap == nil {
		if log := s.log(); log != nil {
			log.Warningf("[agent] session [%s] hd_load_skill call but no skills found in convo state", s.ID)
		}
		panic(fmt.Errorf("%w: no skills found in convo state", ErrSkillLoadFailed))
	}

	skillPath := skillMap[skillName]
	if skillPath == "" {
		if log := s.log(); log != nil {
			log.Warningf("[agent] session [%s] hd_load_skill call with skill_name=%s but no matching skill path found", s.ID, skillName)
		}
		panic(fmt.Errorf("%w: skill '%s' not found in convo state", ErrSkillLoadFailed, skillName))
	}

	skillPath = strings.TrimSuffix(skillPath, "/SKILL.md") + path
	toolCall := AgentToolCall{
		FuncName: s.Agent.FileTool,
		Params: map[string]interface{}{
			"file_specs": []string{skillPath},
		},
	}
	s.handleWorkflowToolCall(toolCall, unifiedConvoID)
}
