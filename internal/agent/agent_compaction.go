package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const (
	DefaultCompactionPreserve = 10
	DefaultCompactionPrompt   = "Summarize the above conversation history, preserving key decisions, " +
		"context, and any critical information that will be needed to continue the conversation. " +
		"Be concise but thorough. Explain what is happening currently at the end."

	// compactionSummarizeReminder is always injected (prepended) into the
	// summarizer prompt — default or custom — so the summarizer reliably produces
	// a summary instead of answering the triggering user's question. The
	// triggering user message stays in the preserved tail and the _gN archive,
	// but is excluded from the summarizer's loaded history via the
	// summarize_upto boundary; the reminder reinforces that the model must not
	// attempt to answer it.
	compactionSummarizeReminder = "You are summarizing conversation history; do NOT answer the user's question; produce a summary only."

	// compactionFailurePlaceholder is injected as a RoleSystem message whenever a
	// summarization attempt fails, so the failure and the archive location are
	// visible in history without truncating anything. The %s is the _g<N>
	// archive key holding the preserved full history.
	compactionFailurePlaceholder = "Summary unavailable (compaction failed); the full conversation history " +
		"has been preserved and archived at `%s`."
)

// shouldCompact returns true when the agent's compaction policy is configured
// and the configured threshold has been reached.
func (s *AgentSession) shouldCompact() bool {
	if s.Agent == nil || s.Agent.CompactionPolicy == nil {
		return false
	}
	cp := s.Agent.CompactionPolicy
	// Only trigger compaction on user messages. A trailing marked slash
	// command (IsSlash) should never trigger compaction, so find the last
	// non-slash message and use its role.
	last := s.lastNonSlashMessage()
	if last == -1 || s.history[last].Role != RoleUser {
		if log := s.log(); log != nil {
			log.Debugf("[agent] session [%s] compaction skipped: last non-slash message is not a user message", s.ID)
		}

		return false
	}
	// Once-per-user-turn guard: compaction fires at most once per real user
	// message. The post-compaction model call appends a fresh agent reply
	// carrying the compacted context's driver-reported tokens (self-healing
	// baseline); this guard keeps that fresh reply from re-triggering
	// compaction again within the same turn.
	if s.CompactedThisTurn {
		if log := s.log(); log != nil {
			log.Debugf("[agent] session [%s] compaction skipped: once-per-turn guard already fired", s.ID)
		}

		return false
	}
	switch cp.ThresholdType {
	case "history_len":
		triggered := len(s.history) >= cp.Threshold
		if log := s.log(); log != nil {
			log.Debugf("[agent] session [%s] compaction check threshold_type=history_len history_len=%d threshold=%d trigger=%t",
				s.ID, len(s.history), cp.Threshold, triggered)
		}

		return triggered
	case "total_tokens":
		triggered := s.PrevContextSize >= cp.Threshold
		if log := s.log(); log != nil {
			log.Debugf("[agent] session [%s] compaction check threshold_type=total_tokens context_size=%d threshold=%d trigger=%t",
				s.ID, s.PrevContextSize, cp.Threshold, triggered)
		}

		return triggered
	}
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] compaction skipped: unknown threshold_type=%q", s.ID, cp.ThresholdType)
	}

	return false
}

// lastNonSlashMessage returns the index of the last history message whose
// IsSlash marker is false, or -1 if every message is slash-origin. Marked slash
// messages (command text and replies) are never part of the model context, so
// they must not influence decisions (such as compaction triggering) that depend
// on the shape of the real conversation.
func (s *AgentSession) lastNonSlashMessage() int {
	for i := len(s.history) - 1; i >= 0; i-- {
		if !s.history[i].IsSlash {
			return i
		}
	}

	return -1
}

// lastNonSlashUserMessage returns the index of the last non-slash user message
// in the session history, or -1 if none exists. Compaction triggers on the last
// non-slash user message (shouldCompact), so this identifies the triggering
// user message that must be excluded from the summarizer's input via the
// summarize_upto boundary. Slash-origin messages are never part of the model
// context, so they are skipped.
func (s *AgentSession) lastNonSlashUserMessage() int {
	for i := len(s.history) - 1; i >= 0; i-- {
		if !s.history[i].IsSlash && s.history[i].Role == RoleUser {
			return i
		}
	}

	return -1
}

// handleCompactionResult performs the archive-and-replace flow when a compaction
// tool-call returns. It returns true when it handled the compaction result and
// the caller should stop normal tool-result processing.
//
// Validation is performed FIRST, before any destructive action: a failed,
// errored, or empty summarization result must never delete or truncate the live
// conversation history. Only a genuinely non-empty, successful summary may
// proceed to archive/delete/replace. Every failure path degrades gracefully via
// compactionFailed: it rolls back from the clean _gN archive (or keeps the live
// history), injects a RoleSystem placeholder, and resumes the conversation so
// the turn is never left stuck.
func (s *AgentSession) handleCompactionResult(c AgentToolCall, toolResults []map[string]interface{}) bool {
	if c.Params == nil {
		return false
	}

	// Only handle tool-calls that were marked for compaction.
	compactIDRaw, ok := c.Params["compaction_id"]
	if !ok {
		return false
	}
	compactID, ok := compactIDRaw.(string)
	if !ok || compactID == "" {
		return false
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

	// --- Validation-first: nothing destructive happens (no archive/delete/
	// replace/marker) until the summary is verified to be a genuine, non-empty
	// success. ---
	var lastResult map[string]interface{}
	if len(toolResults) > 0 {
		lastResult = toolResults[len(toolResults)-1]
	}
	if lastResult == nil {
		return s.compactionFailed(compactID, "no summarization result was returned")
	}

	// 1. Consult the status label FIRST. A failed summarizer (via
	// StartAgentCall's SafeExitOnError) emits status=failure with no
	// data.output; it must never proceed to archive/delete/replace.
	if status, _ := lastResult["status"].(string); status != "success" {
		reason := fmt.Sprintf("summarizer returned non-success status %q", status)
		if r, ok := lastResult["reason"].(string); ok && r != "" {
			reason = r
		}

		return s.compactionFailed(compactID, reason)
	}

	// 2. Extract the summary and reject empty/whitespace-only output.
	summaryText := ""
	if out, ok := lastResult["data"]; ok {
		summaryText = summaryTextOf(out)
	}
	if strings.TrimSpace(summaryText) == "" {
		return s.compactionFailed(compactID, "summarizer produced an empty summary")
	}

	// 3. Reject degenerate JSON literals that a failed summarizer can produce
	// (e.g. json.Marshal(nil) == "null").
	switch strings.TrimSpace(summaryText) {
	case "null", "{}", "[]":
		return s.compactionFailed(compactID, fmt.Sprintf("summarizer produced a degenerate summary %q", strings.TrimSpace(summaryText)))
	}

	// Append archived conversation marker to summary text for UI navigation.
	// This allows the UI to provide a direct link to the archived conversation.
	summaryText += fmt.Sprintf("\n<!-- archived_convo: %s -->", compactID)

	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] compaction result: summary_len=%d preserve=%d history_len=%d",
			s.ID, len(summaryText), preserve, len(s.history))
	}

	// Build the new history: summary as a system message, then the preserved tail messages.
	total := len(s.history)
	if total == 0 {
		return s.compactionFailed(compactID, "compaction ran with an empty conversation history")
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

	// Replace persisted convo history in Redis with the new compacted history,
	// verifying each write (failure-atomic): a failed del is tolerated (an
	// already-absent history isn't the failure), but a failed rpush after a
	// successful del would lose live history, so any missing write triggers the
	// rollback path via compactionFailed.
	if err := s.replaceLiveHistory(newHistory); err != nil {
		return s.compactionFailed(compactID, fmt.Sprintf("failed to persist compacted history: %v", err))
	}

	// Update in-memory history.
	s.history = newHistory
	// Record the history length at the compaction boundary. Baseline
	// measurements (refreshContextSize) only consider agent messages appended
	// at or after this index, so preserved-tail messages carrying
	// pre-compaction (large) tokens cannot re-trigger compaction on the next
	// user turn when the post-compaction resume produced no new agent message.
	s.CompactionHistoryIdx = len(newHistory)
	s.PrevContextSize = 0 // reset previous context size since we're starting fresh with the summary as context

	return s.finalizeCompaction(newHistory)
}

// finalizeCompaction records the compaction boundary in ConvoState, resets the
// in-flight tool-call state, persists the compacted session, and resumes the
// conversation. It is the shared success-path continuation for
// handleCompactionResult, kept in a separate helper so the validation-first
// function stays short.
//
// The compaction marker is persisted in ConvoState so it survives across user
// turns. Every real user message creates a brand-new AgentSession that seeds
// its CompactionHistoryIdx from ConvoState.LastCompactionHistoryLen; without
// this persistence the next turn's fresh session would default the marker to 0,
// re-scan the preserved tail's pre-compaction (large) tokens, and re-trigger
// compaction every turn until a new agent message appears. ContextTokens is
// also recalculated from the new compacted history when a custom token counter
// is active: since appendConvoHistory counts tokens on append and compaction
// replaces history entirely, we recount all tokens.
func (s *AgentSession) finalizeCompaction(newHistory []AgentMessage) bool {
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.LastCompactionHistoryLen = len(newHistory)
		// Persist the reset baseline (s.PrevContextSize was just zeroed above) so
		// the API-exposed compaction metric reflects the fresh post-compaction
		// state instead of the stale pre-compaction context size.
		cs.PrevContextSize = s.PrevContextSize
		if s.TokenCounter != nil {
			cs.ContextTokens = s.countSystemPromptTokens()
			for _, msg := range s.history {
				if msg.IsSlash {
					continue
				}
				cs.ContextTokens += s.countMessageTokens(msg)
			}
		}
	})
	s.CurrentCall = 0
	s.ToolResults = nil

	// Persist the compacted session (with any pending SlashInitiatedCompaction
	// flag) before branching. This also makes the flag durable across a restore
	// in case the confirmation reply is processed after a reload.
	s.persist(false)

	// Resume the conversation only when compaction was NOT slash-initiated.
	// - Automatic compaction runs mid-turn from shouldCompact()/sendToDriver()
	//   where there is a real pending user message, so we send the compacted
	//   history to the model to continue the turn.
	// - A /compact slash command has NO pending user message; resuming the
	//   model here would produce a spurious continuation. Instead we post the
	//   confirmation as a marked IsSlash reply (persisted + returned to the
	//   workflow/UI, but never sent to the model) and leave the conversation
	//   idle for the next real user turn.
	if s.SlashInitiatedCompaction {
		s.SlashInitiatedCompaction = false
		s.reply("\u2705 Conversation compacted. The history has been summarized; older turns are archived.")
		if log := s.log(); log != nil {
			log.Infof("[agent] session [%s] slash-initiated compaction complete; new_history_len=%d", s.ID, len(s.history))
		}

		return true
	}

	// Automatic compaction: resume the conversation by sending the updated
	// history to the configured driver so the model can continue.
	s.sendToDriver()
	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] compaction complete; new_history_len=%d", s.ID, len(s.history))
	}

	return true
}

// compactionFailed handles a failed/errored/empty summarization result. It
// rolls back the live history to the clean pre-compaction _gN archive when the
// archive is complete, tolerates a partial/empty archive by keeping the live
// history, injects a RoleSystem placeholder recording the failure and archive
// location, and resumes the conversation so the turn is never left stuck.
//
// Rollback must NOT re-set CompactionHistoryIdx / LastCompactionHistoryLen:
// no compaction happened, so there is no new compaction boundary to record.
// CompactedThisTurn is deliberately left as-is (it was set by compactHistory
// when the summarizer was dispatched) so the degraded sendToDriver does not
// immediately re-fire compaction within the same turn; run() clears it on the
// next real user turn.
func (s *AgentSession) compactionFailed(compactID, reason string) bool {
	if log := s.log(); log != nil {
		log.Errorf("[agent] session [%s] compaction failed (archive=%s): %s", s.ID, compactID, reason)
	}

	// Roll back from the clean pre-compaction archive (_gN) when available;
	// otherwise keep the live history. Never truncate live history on failure.
	s.rollbackFromArchive(compactID)

	// Inject a RoleSystem placeholder so the failure and the archive location
	// are visible in history, rather than silently truncating or leaving a
	// garbage summary in place.
	s.appendConvoHistory(&AgentMessage{
		Role:    RoleSystem,
		Content: fmt.Sprintf(compactionFailurePlaceholder, compactID),
	})

	// Reset the in-flight tool-call state so the session is clean for the
	// degraded continuation. Do NOT touch CompactionHistoryIdx /
	// LastCompactionHistoryLen (no compaction happened; the old boundary stays).
	s.CurrentCall = 0
	s.ToolResults = nil
	s.ToolCalls = nil

	// Persist the degraded state so a mid-flight restore observes the rollback.
	s.persist(false)

	// Resume the conversation so the turn is never left stuck.
	if s.SlashInitiatedCompaction {
		s.SlashInitiatedCompaction = false
		s.reply("\u26a0\ufe0f Compaction failed; the full conversation history has been preserved.")
		if log := s.log(); log != nil {
			log.Infof("[agent] session [%s] slash-initiated compaction failed; history preserved (history_len=%d)", s.ID, len(s.history))
		}

		return true
	}

	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] compaction failed; resuming with full history (history_len=%d)", s.ID, len(s.history))
	}
	s.sendToDriver()

	return true
}

// rollbackFromArchive restores the live conversation history from the clean
// pre-compaction _gN archive produced by compactHistory (which runs archiveConvo
// BEFORE appending the compaction tool-call card, so the archive includes the
// triggering user message and excludes the card). It returns true when the
// restore succeeded.
//
// The archive rpush is best-effort, so a partial/empty archive is tolerated:
// when the archive is unreadable, unparseable, empty, or shorter than the
// pre-compaction live history, the live history is left untouched (keep-live).
func (s *AgentSession) rollbackFromArchive(compactID string) bool {
	archivedKey := ConvoHistoryKeyPrefix + compactID
	ret, err := s.store.Call("cache", "lrange", map[string]interface{}{"key": archivedKey})
	if err != nil {
		return false // archive unreadable -> keep live history
	}
	var archived []AgentMessage
	if err := json.Unmarshal(ret, &archived); err != nil {
		return false // archive unparseable -> keep live history
	}
	if len(archived) == 0 {
		return false // empty archive -> keep live history
	}
	// The compaction tool-call card is the last message of the live history, so
	// a complete archive holds exactly one fewer message. A shorter archive was
	// written partially (best-effort rpush) and is not a safe rollback target:
	// keep the (fuller) live history instead.
	if len(archived) < len(s.history)-1 {
		return false // partial archive -> keep live history
	}

	// Replace the persisted live history with the archived content. The archive
	// itself remains the durable safety net if this rewrite fails partway.
	if err := s.replaceLiveHistory(archived); err != nil {
		if log := s.log(); log != nil {
			log.Errorf("[agent] session [%s] compaction rollback: failed to rewrite live history from archive %s: %v", s.ID, compactID, err)
		}

		return false
	}
	s.history = archived

	return true
}

// replaceLiveHistory replaces the persisted live conversation history with the
// given messages, verifying each write. A failed del is tolerated (an
// already-absent history isn't the failure); any failed rpush is returned as an
// error so the caller can roll back rather than mark a partial write as success.
func (s *AgentSession) replaceLiveHistory(messages []AgentMessage) error {
	fullKey := ConvoHistoryKeyPrefix + s.ConvoID
	// A failed del is non-fatal: already-absent history isn't the failure —
	// missing writes are.
	_, _ = s.store.Call("cache", "del", map[string]interface{}{"key": fullKey})
	convoTTL, _ := time.ParseDuration(ConvoStreamTTL)
	for _, m := range messages {
		if _, err := s.store.Call("cache", "rpush", map[string]interface{}{
			"key":   fullKey,
			"value": string(dipper.SerializeContent(m)),
			"ttl":   float64(convoTTL),
		}); err != nil {
			return fmt.Errorf("replaceLiveHistory: %w", err)
		}
	}

	return nil
}

// summaryTextOf renders a tool result's "data" field into a summary string the
// way the compaction path expects: strings pass through unchanged, and any
// other value is JSON-marshalled (which is why a nil data field yields the
// literal "null" that the validation layer must reject).
func summaryTextOf(out interface{}) string {
	switch v := out.(type) {
	case string:
		return v
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}

	return ""
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

	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] compactHistory threshold_type=%s threshold=%d context_size=%d history_len=%d preserve=%d",
			s.ID, cp.ThresholdType, cp.Threshold, s.PrevContextSize, len(s.history), preserve)
	}

	// Nothing to compact if history is shorter than the preserve window.
	if len(s.history) <= preserve {
		if log := s.log(); log != nil {
			log.Debugf("[agent] session [%s] compaction skipped: history_len=%d not above preserve=%d", s.ID, len(s.history), preserve)
		}

		return false
	}

	// Build the summarization prompt (default or custom).
	prompt := cp.SummarizationPrompt
	if prompt == "" {
		prompt = DefaultCompactionPrompt
	}
	// ALWAYS inject the "summarize, do not answer" reminder into whichever prompt
	// is used (default or custom). The summarizer sub-agent is generic: without
	// this reminder it can drift toward answering the user's question, which
	// leads to loss of history. The reminder supplements (never replaces) the
	// summarizer agent's own system prompt.
	prompt = compactionSummarizeReminder + "\n\n" + prompt

	// Compute the summarize_upto boundary: the summarizer must receive the
	// archived history up to (but excluding) the triggering user message so it
	// summarizes instead of answering the question. The triggering user message
	// stays in the preserved tail and the full _gN archive; only the summarizer's
	// loaded input excludes it.
	summarizeUpto := s.lastNonSlashUserMessage()
	if summarizeUpto < 0 {
		// Defensive fallback: no user message to exclude, summarize the whole
		// (archived) history.
		summarizeUpto = len(s.history)
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
	// summarization sub-agent will load when started with `compaction_id`. The
	// archive is the rollback target on summarization failure, so a failed
	// archive must NOT panic (dipper.Must): compaction cannot safely run without
	// a clean recovery point, so it degrades gracefully by returning false and
	// letting the caller fall back to the full-history path.
	var archiveErr error
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		compactID, archiveErr = cs.archiveConvo(s.store)
	})
	if archiveErr != nil {
		if log := s.log(); log != nil {
			log.Errorf("[agent] session [%s] compaction skipped: failed to archive history: %v", s.ID, archiveErr)
		}

		return false
	}

	// Invoke the summarization agent as a sub-agent tool call so the
	// result is delivered via eventbus:agent_continue and handled through
	// the normal tool-call result path. Mark the call with a "compaction"
	// flag so the continuation handler can perform the archive/replace.
	toolCall := AgentToolCall{
		FuncName: "ag__" + summAgent.Name,
		Params: map[string]interface{}{
			"input":          prompt,
			"compaction_id":  compactID,
			"preserve":       preserve,
			"summarize_upto": summarizeUpto,
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
	s.appendConvoHistory(&agentMsg)

	// Mark this real user turn as compacted so compaction cannot re-fire before
	// the next real user message (once-per-turn guard). Cleared at the start of
	// each new real user turn in run().
	s.CompactedThisTurn = true

	// Kick off the tool call from this session.
	s.CurrentCall = 0
	s.ToolCalls = agentMsg.ToolCalls
	s.nextToolCall()

	return true
}
