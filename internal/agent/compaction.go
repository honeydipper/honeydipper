// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package agent

import (
	"encoding/json"
	"fmt"

	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

// CompactionService handles compaction of agent conversation history.
type CompactionService struct {
	store AgentStore
}

// NeedsCompaction checks if the session's history exceeds the compaction threshold.
// It supports two threshold types: "history_len" (number of messages) and "total_tokens" (cumulative tokens).
func (cs *CompactionService) NeedsCompaction(history []agentpkg.Message, policy *agentpkg.CompactionPolicy) bool {
	if policy == nil {
		return false
	}
	switch policy.ThresholdType {
	case "history_len":
		return len(history) >= policy.Threshold
	case "total_tokens":
		var totalTokens int
		for _, msg := range history {
			totalTokens += msg.InputTokens + msg.OutputTokens
		}
		return totalTokens >= policy.Threshold
	default:
		// Unknown threshold type, disable compaction
		return false
	}
}

// Compact performs compaction on the session's conversation history.
// It archives the current history, summarizes the archived portion (excluding the most recent PreserveRecent messages),
// and replaces the history with the summary followed by the preserved recent messages.
// The caller is responsible for ensuring no concurrent modifications to the history.
func (cs *CompactionService) Compact(s *AgentSession) error {
	if s.Agent == nil || s.Agent.CompactionPolicy == nil {
		return nil
	}
	policy := s.Agent.CompactionPolicy
	if policy.PreserveRecent < 0 {
		return fmt.Errorf("invalid PreserveRecent: %d", policy.PreserveRecent)
	}

	// Load the current ConvoState to get the Generation and to archive.
	csState := &ConvoState{}
	csState.load(s.ConvoID, s.store)
	// Archive the current conversation history.
	archivedKey, err := csState.archiveConvo(s.store)
	if err != nil {
		return fmt.Errorf("archiveConvo failed: %w", err)
	}
	// Persist the updated ConvoState (with incremented Generation).
	csState.persist(s.store)

	// Read the current conversation history from the cache.
	historyKey := ConvoHistoryKeyPrefix + s.ConvoID
	ret, err := s.store.Call("cache", "lrange", map[string]interface{}{"key": historyKey})
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}
	var historyMessages []agentpkg.Message
	if err := json.Unmarshal(ret, &historyMessages); err != nil {
		return fmt.Errorf("failed to unmarshal history: %w", err)
	}

	// If there are no messages or not enough to preserve, nothing to compact.
	if len(historyMessages) == 0 {
		return nil
	}
	if len(historyMessages) <= policy.PreserveRecent {
		// Not enough messages to compact; just persist the ConvoState (already done) and return.
		return nil
	}

	// Split history into oldMessages (to summarize) and recentMessages (to keep verbatim).
	var recentMessages []agentpkg.Message
	if policy.PreserveRecent > 0 {
		recentMessages = historyMessages[len(historyMessages)-policy.PreserveRecent:]
	}
	oldMessages := historyMessages[:len(historyMessages)-len(recentMessages)]

	// Summarize the oldMessages using the summarization agent.
	summary, err := cs.summarizeHistory(s, oldMessages, policy)
	if err != nil {
		return fmt.Errorf("summarization failed: %w", err)
	}

	// Create a summary message.
	summaryMsg := agentpkg.Message{
		Role:     agentpkg.RoleSystem,
		Content:  summary,
		IsComplete: true,
	}

	// Serialize the summary message.
	summaryMsgBytes, err := json.Marshal(summaryMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal summary message: %w", err)
	}

	// Update the cache: keep only the recent messages, then push the summary to the front.
	// First, trim the list to keep only the recent messages.
	if policy.PreserveRecent > 0 {
		if _, err := s.store.Call("cache", "ltrim", map[string]interface{}{
			"key":   historyKey,
			"start": -policy.PreserveRecent,
			"stop":  -1,
		}); err != nil {
			return fmt.Errorf("failed to trim history: %w", err)
		}
	} else {
		// If PreserveRecent is 0, we want to delete the entire list.
		if _, err := s.store.Call("cache", "del", map[string]interface{}{"key": historyKey}); err != nil {
			return fmt.Errorf("failed to delete history: %w", err)
		}
	}
	// Now push the summary message to the front (left) of the list.
	if _, err := s.store.Call("cache", "lpush", map[string]interface{}{
		"key":   historyKey,
		"value": string(summaryMsgBytes),
	}); err != nil {
		return fmt.Errorf("failed to push summary: %w", err)
	}
	// Then push the recent messages back (in order) so they follow the summary.
	// We need to push them in reverse order because lpush prepends.
	for i := len(recentMessages) - 1; i >= 0; i-- {
		msgBytes, err := json.Marshal(recentMessages[i])
		if err != nil {
			return fmt.Errorf("failed to marshal recent message %d: %w", i, err)
		}
		if _, err := s.store.Call("cache", "rpush", map[string]interface{}{
			"key":   historyKey,
			"value": string(msgBytes),
		}); err != nil {
			return fmt.Errorf("failed to push recent message %d: %w", i, err)
		}
	}

	// Update the in-memory history to match the new cache state.
	var newHistory []agentpkg.Message
	newHistory = append(newHistory, summaryMsg)
	newHistory = append(newHistory, recentMessages...)
	s.history = newHistory

	// Log the compaction.
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] compacted conversation: archived=%s, summary_len=%d, recent=%d",
			s.ID, archivedKey, len(summary), len(recentMessages))
	}

	return nil
}

// summarizeHistory creates a temporary inference session for the summarization agent
// and returns the summary string.
func (cs *CompactionService) summarizeHistory(s *AgentSession, oldMessages []agentpkg.Message, policy *agentpkg.CompactionPolicy) (string, error) {
	// Get the summarization agent configuration.
	summarizationAgent := s.store.GetAgent(policy.SummarizationAgent)
	if summarizationAgent == nil {
		return "", fmt.Errorf("summarization agent %s not found", policy.SummarizationAgent)
	}

	// Build the payload for the send_to_model RPC.
	payload := map[string]interface{}{
		"engine":        summarizationAgent.Engine,
		"history":       oldMessages,
		"model_data":    summarizationAgent.ModelData,
		"should_stream": false, // we don't need streaming for summarization
	}
	if policy.SummarizationPrompt != "" {
		payload["summarization_prompt"] = policy.SummarizationPrompt
	}
	// The type field is required by the driver to know this is an inference request.
	payload["type"] = agentpkg.SessionTypeInference

	// Call the summarization agent's send_to_model method.
	// We pass the payload as the params and set a timeout label.
	// We do not set agent_session_id label to avoid creating a session record.
	resp, err := s.store.Call("driver:"+summarizationAgent.Driver, "send_to_model", payload,
		"timeout", "5m")
	if err != nil {
		return "", fmt.Errorf("failed to call summarization agent: %w", err)
	}

	// The response is a dipper.Message (as []byte) where the Payload contains a "message" field.
	var respMsg dipper.Message
	if err := json.Unmarshal(resp, &respMsg); err != nil {
		return "", fmt.Errorf("failed to unmarshal summarization response: %w", err)
	}
	msg, ok := dipper.GetMapData(respMsg.Payload, "message")
	if !ok {
		return "", fmt.Errorf("summarization response missing 'message' field")
	}
	m := msg.(map[string]interface{})
	var summaryMsg agentpkg.Message
	if err := mapstructure.Decode(m, &summaryMsg); err != nil {
		return "", fmt.Errorf("failed to decode summary message: %w", err)
	}
	return summaryMsg.Content, nil
}
