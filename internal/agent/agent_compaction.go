package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const (
	DefaultCompactionPreserve = 10
	DefaultCompactionPrompt   = "Summarize the above conversation history, preserving key decisions, " +
		"context, and any critical information that will be needed to continue the conversation. " +
		"Be concise but thorough. Explain what is happening currently at the end."
)

// shouldCompact returns true when the agent's compaction policy is configured
// and the configured threshold has been reached.
func (s *AgentSession) shouldCompact() bool {
	if s.Agent == nil || s.Agent.CompactionPolicy == nil {
		return false
	}
	if len(s.history) == 0 || s.history[len(s.history)-1].Role != RoleUser {
		// only trigger compaction on user messages.
		return false
	}
	switch s.Agent.CompactionPolicy.ThresholdType {
	case "history_len":
		return len(s.history) >= s.Agent.CompactionPolicy.Threshold
	case "total_tokens":
		return s.PrevContextSize >= s.Agent.CompactionPolicy.Threshold
	}

	return false
}

// handleCompactionResult performs the archive-and-replace flow when a compaction
// tool-call returns. It returns true when it handled the compaction result and
// the caller should stop normal tool-result processing.
func (s *AgentSession) handleCompactionResult(c AgentToolCall, toolResults []map[string]interface{}) bool {
	if c.Params == nil {
		return false
	}

	// Only handle tool-calls that were marked for compaction.
	compactID, ok := c.Params["compaction_id"]
	if !ok || compactID == "" {
		return false
	}

	// Extract the summary output from the collected ToolResults.
	var summaryText string
	if len(toolResults) > 0 {
		out := toolResults[len(toolResults)-1]["data"]
		switch v := out.(type) {
		case string:
			summaryText = v
		default:
			if b, err := json.Marshal(v); err == nil {
				summaryText = string(b)
			}
		}
	}

	if summaryText == "" {
		if log := s.log(); log != nil {
			log.Errorf("[agent] session [%s] compaction produced empty summary", s.ID)
		}

		return true
	}

	// Determine preserve window from params.
	preserve := DefaultCompactionPreserve
	if pv2, ok := c.Params["preserve"]; ok {
		switch v := pv2.(type) {
		case int:
			preserve = v
		case int64:
			preserve = int(v)
		case float64:
			preserve = int(v)
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				preserve = n
			}
		}
	}

	// Append archived conversation marker to summary text for UI navigation.
	// This allows the UI to provide a direct link to the archived conversation.
	if compactID != "" {
		summaryText += fmt.Sprintf("\n<!-- archived_convo: %s -->", compactID)
	}

	// Build the new history: summary as a system message, then the preserved tail messages.
	total := len(s.history)
	if total == 0 {
		return true
	}
	toolIndex := total - 1
	tailStart := toolIndex - preserve
	if tailStart < 0 {
		tailStart = 0
	}
	tail := s.history[tailStart:toolIndex]

	// Persist only the summary as a system message. The active system prompt
	// is intentionally NOT persisted and will be prepended in sendToDriver.
	newHistory := make([]AgentMessage, 0, 1+len(tail))
	newHistory = append(newHistory, AgentMessage{Role: RoleSystem, Content: "Here is a summary of the conversation so far:\n" + summaryText})
	newHistory = append(newHistory, tail...)

	// Replace persisted convo history in Redis with the new compacted history.
	_, _ = s.store.Call("cache", "del", map[string]interface{}{"key": ConvoHistoryKeyPrefix + s.ConvoID})
	convoTTL, _ := time.ParseDuration(ConvoStreamTTL)
	fullKey := ConvoHistoryKeyPrefix + s.ConvoID
	for _, m := range newHistory {
		_, _ = s.store.Call("cache", "rpush", map[string]interface{}{
			"key":   fullKey,
			"value": string(dipper.SerializeContent(m)),
			"ttl":   float64(convoTTL),
		})
	}

	// Update in-memory history
	s.history = newHistory
	s.PrevContextSize = 0 // reset previous context size since we're starting fresh with the summary as context

	// Recalculate ContextTokens from the new compacted history.
	// Since appendConvoHistory counts tokens on append, and compaction replaces
	// history entirely, we need to recount all tokens in the new history.
	if s.TokenCounter != nil {
		lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
			cs.ContextTokens = s.countSystemPromptTokens()
			for _, msg := range s.history {
				cs.ContextTokens += s.countMessageTokens(msg)
			}
		})
	}
	s.CurrentCall = 0
	s.ToolResults = nil

	s.persist(false)

	// After successful compaction, resume the conversation by sending the
	// updated history to the configured driver so the model can continue.
	// This keeps the compaction flow inline with mid-conversation triggers.
	s.sendToDriver()
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] compaction complete; new_history_len=%d", s.ID, len(s.history))
	}

	return true
}

// compactHistory runs the configured summarization agent to produce a
// condensed summary of older conversation turns, archives the previous
// history under a generation-suffixed key, and replaces the persisted
// history with the summary plus the most-recent turns to preserve.
func (s *AgentSession) compactHistory() bool {
	if s.Agent == nil || s.Agent.CompactionPolicy == nil {
		return false
	}
	cp := s.Agent.CompactionPolicy

	preserve := cp.PreserveRecent
	if preserve == 0 {
		preserve = DefaultCompactionPreserve
	}

	// Nothing to compact if history is shorter than the preserve window.
	if len(s.history) <= preserve {
		return false
	}

	// Build the summarization prompt.
	prompt := cp.SummarizationPrompt
	if prompt == "" {
		prompt = DefaultCompactionPrompt
	}

	// Resolve the summarization agent config.
	if cp.SummarizationAgent == "" {
		if log := s.log(); log != nil {
			log.Infof("[agent] session [%s] compaction configured but no summarization_agent set", s.ID)
		}

		return false
	}
	summAgent := s.store.GetAgent(cp.SummarizationAgent)
	if summAgent == nil {
		if log := s.log(); log != nil {
			log.Errorf("[agent] session [%s] summarization agent %q not found", s.ID, cp.SummarizationAgent)
		}

		return false
	}

	var compactID string
	// Archive the current conversation and capture the archived key.
	// archiveConvo returns an archived key like "<ConvoID>_g<N>" which the
	// summarization sub-agent will load when started with `compaction_id`.
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		compactID = dipper.Must(cs.archiveConvo(s.store)).(string)
	})

	// Invoke the summarization agent as a sub-agent tool call so the
	// result is delivered via eventbus:agent_continue and handled through
	// the normal tool-call result path. Mark the call with a "compaction"
	// flag so the continuation handler can perform the archive/replace.
	toolCall := AgentToolCall{
		FuncName: "ag__" + summAgent.Name,
		Params: map[string]interface{}{
			"input":         prompt,
			"compaction_id": compactID,
			"preserve":      preserve,
		},
	}

	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] running compaction via agent=%s engine=%s history_len=%d preserve=%d",
			s.ID, summAgent.Name, summAgent.Engine, len(s.history), preserve)
	}

	// Append a tool-call entry to the conversation history and dispatch it
	// using the existing tool-call mechanism so the summarizer runs as a
	// sub-agent and returns via eventbus:agent_continue.
	agentMsg := AgentMessage{Role: RoleAgent, Content: "", ToolCalls: []AgentToolCall{toolCall}}
	s.appendConvoHistory(agentMsg)

	// Kick off the tool call from this session.
	s.CurrentCall = 0
	s.ToolCalls = agentMsg.ToolCalls
	s.nextToolCall()

	return true
}
