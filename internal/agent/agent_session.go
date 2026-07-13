package agent

import (
	"context"
	"encoding/json"
	"fmt"
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
	AgentSessionTypeInference            = agentpkg.SessionTypeInference
	AgentSessionTypeChatTurn             = agentpkg.SessionTypeChatTurn
	AgentKeyPrefix                       = "agent_session:"
	ConvoHistoryKeyPrefix                = "convo_history:"
	MCPToolsCachePrefix                  = "mcp_tools:"
	AgentSessionDefaultTTL               = "336h"
	AgentSessionDefaultTimeout           = "1h"
	MCPToolsCacheDefaultTTL              = "6h"
	AgentSessionDefaultTurnLockExpire    = "1h"
	AgentSessionDefaultDriverCallTimeout = "300s"
	AgentSessionDefaultPollTimeout       = time.Second * 9
	MinPollInterval                      = time.Second * 2

	RoleSystem     = agentpkg.RoleSystem
	RoleUser       = agentpkg.RoleUser
	RoleAgent      = agentpkg.RoleAgent
	RoleTool       = agentpkg.RoleTool
	RoleToolResult = agentpkg.RoleToolResult
)

// AgentSession holds the runtime state of a single agent inference or chat-turn session.
type AgentSession struct {
	ID             string
	ConvoID        string
	UnifiedConvoID string
	Agent          *config.Agent

	history               []AgentMessage
	CurrentMsg            *dipper.Message
	Type                  string
	TTL                   string
	CurrentCall           int
	ToolCalls             []AgentToolCall
	ToolResults           []map[string]interface{}
	LastPoll              int
	LastPollTime          time.Time
	PendingContent        string
	PendingThoughts       string
	NewPendingContent     bool
	PendingThoughtsOffset int
	ErrorReason           string
	PrevContextSize       int
	TotalTokens           int
	InputTokens           int
	OutputTokens          int
	TokenCounter          agentpkg.TokenCounter `json:"-"`
	ParentSessionID       string
	ParentTurnID          string
	ParentToolCallID      string
	// TurnLockKey is the distributed lock key for this conversation's turn.
	// It is set when the turn lock is acquired and cleared when released.
	// The lock prevents concurrent sessions from modifying the same conversation.
	TurnLockKey string

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

// lockTurn acquires the per-conversation turn lock for convoID and records the
// key on the session so syncConvoStateStatus/releaseTurnLock can release it when
// the turn reaches a terminal state. The lock serialises turns within a
// conversation and is held for the whole turn lifecycle. Take it before reading
// the conversation history so a turn that waits for an in-flight prior turn
// resumes from that prior turn's completed history rather than a stale snapshot.
// store is passed explicitly because this may run before setup() sets s.store.
func (s *AgentSession) lockTurn(store AgentStore, convoID string) {
	s.TurnLockKey = ConvoTurnLockPrefix + convoID
	expire := AgentSessionDefaultTurnLockExpire
	if s.Agent != nil && s.Agent.TurnLockTimeout != "" {
		expire = s.Agent.TurnLockTimeout
	}
	dipper.Must(store.Call("locker", "lock", map[string]interface{}{
		"name":   s.TurnLockKey,
		"expire": expire,
	}, "timeout", "30m"))
}

// releaseTurnLock releases the distributed turn lock for this session's conversation.
// It is idempotent: calling it with an empty TurnLockKey is a no-op.
func (s *AgentSession) releaseTurnLock() {
	if s.TurnLockKey == "" {
		return
	}
	if log := s.log(); log != nil {
		log.Debugf("[agent] session [%s] releasing turn lock %q", s.ID, s.TurnLockKey)
	}
	dipper.Must(s.store.Call("locker", "unlock", map[string]interface{}{
		"name": s.TurnLockKey,
	}, "timeout", "30s"))
	s.TurnLockKey = ""
}

// buildConvoURLs constructs the conversation page URL and focus page URL for the
// current conversation. It returns an error if the UI URL is not configured.
func (s *AgentSession) buildConvoURLs() (string, string, error) {
	base := strings.TrimRight(s.store.GetUIURL(), "/")
	if base == "" {
		return "", "", fmt.Errorf("%w: HD_UI_URL env var is not set, cannot construct conversation URL", ErrToolCall)
	}
	convoURL := fmt.Sprintf("%s/conversations/%s", base, s.ConvoID)
	focusURL := fmt.Sprintf("%s/focus/%s", base, s.ConvoID)

	return convoURL, focusURL, nil
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
// When a terminal state is detected, it also releases the distributed turn lock.
func (s *AgentSession) syncConvoStateStatus() {
	var status string
	switch {
	case s.ErrorReason != "":
		status = ConvoSessionStatusFailed
	case len(s.history) > 0 && s.history[len(s.history)-1].IsComplete &&
		len(s.history[len(s.history)-1].ToolCalls) == 0:
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

		cs.updateSessionStatus(s.ID, status, s.ErrorReason, s.InputTokens, s.OutputTokens, s.TotalTokens)
	})

	s.releaseTurnLock()
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
	// A non-empty agent_session_id label means "restore this session". Otherwise
	// adopt a session id the caller minted before setup (StartInference does this
	// so it can ack the id and take the turn lock before reading history), and
	// fall back to generating one.
	labelID := msg.Labels["agent_session_id"]
	id := labelID
	if id == "" {
		id = s.ID
	}
	if id == "" {
		id = dipper.NewUUID()
	}

	if locking {
		dipper.Must(store.Call("locker", "lock", map[string]interface{}{
			"name":   AgentKeyPrefix + id,
			"expire": "600s",
		}))
	}

	if labelID != "" {
		s.load(id, store)
		s.store = store
		// History is loaded explicitly by the caller after setup() returns.
	} else {
		s.initNewSession(id, msg, store)
	}

	// Ensure Agent is set. For restored sessions, load from ConvoState.
	if s.Agent == nil {
		cs := &ConvoState{}
		cs.load(s.ConvoID, store)
		s.Agent = cs.Agent
		if s.Agent == nil {
			panic(fmt.Sprintf("agent config not found for session %s: ConvoState.Agent=nil", s.ID))
		}
	}

	// Initialize custom token counter only when explicitly set to "simple".
	// When TokenCounter is nil, the session uses driver-reported token counts instead
	// and skips all custom counting logic (ContextTokens).
	if s.TokenCounter == nil && s.Agent != nil && s.Agent.TokenCounter == "simple" {
		s.TokenCounter = &SimpleTokenCounter{}
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
				archivedKey := dipper.Must(cs.archiveConvo(store)).(string)
				s.history = nil
				dipper.Must(s.store.Call("cache", "del", map[string]interface{}{
					"key": ConvoHistoryKeyPrefix + s.ConvoID,
				}))
				markerMsg := AgentMessage{Role: RoleSystem, Content: fmt.Sprintf("<!-- archived_convo: %s -->", archivedKey)}
				s.history = append(s.history, markerMsg)
				convoTTL, _ := time.ParseDuration(ConvoStreamTTL)
				dipper.Must(s.store.Call("cache", "rpush", map[string]interface{}{
					"key":   ConvoHistoryKeyPrefix + s.ConvoID,
					"value": string(dipper.SerializeContent(markerMsg)),
					"ttl":   float64(convoTTL),
				}))
			}
		}
		if cs.LastSession != nil {
			s.PrevContextSize = cs.LastSession.InputTokens + cs.LastSession.OutputTokens
		}

		if cs.Agent == nil {
			cs.Agent = interpolateAgentConfig(s.store, msg.Labels["agent_name"], msg.Payload)
		}
		// Apply optional engine/driver overrides from the API request payload.
		// These override the agent config values when non-empty.
		if engine, ok := dipper.GetMapDataStr(msg.Payload, "engine"); ok && engine != "" {
			cs.Agent.Engine = engine
		}
		if driver, ok := dipper.GetMapDataStr(msg.Payload, "driver"); ok && driver != "" {
			cs.Agent.Driver = driver
		}
		s.Agent = cs.Agent
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

// interpolateAgentConfig applies template interpolation to the agent's config fields using the current session data.
func interpolateAgentConfig(store AgentStore, agentName string, payload any) *config.Agent {
	data, _ := dipper.GetMapData(payload, "data")
	user, _ := dipper.GetMapDataStr(payload, "user")
	text := dipper.MustGetMapDataStr(payload, "text")

	agent := *store.GetAgent(agentName)

	envData := map[string]any{
		"agent_name": agent.Name,
		"agent_data": data,
		"model_data": agent.ModelData,
		"user":       user,
		"text":       text,
	}
	agent.ModelData = dipper.Interpolate("agent_model_data", agent.ModelData, envData).(map[string]interface{})
	envData["model_data"] = agent.ModelData

	agent.SystemPrompt = dipper.InterpolateStr("agent_system_prompt", agent.SystemPrompt, envData)
	agent.InferencePrompt = dipper.InterpolateStr("agent_inference_prompt", agent.InferencePrompt, envData)
	agent.Driver = dipper.InterpolateStr("agent_driver", agent.Driver, envData)
	agent.Engine = dipper.InterpolateStr("agent_engine", agent.Engine, envData)
	agent.PreContext = dipper.Interpolate("agent_pre_context", agent.PreContext, envData).([]string)
	agent.FileTool = dipper.InterpolateStr("agent_file_tool", agent.FileTool, envData)
	agent.TurnLockTimeout = dipper.InterpolateStr("agent_turn_lock_timeout", agent.TurnLockTimeout, envData)
	agent.DriverCallTimeout = dipper.InterpolateStr("agent_driver_call_timeout", agent.DriverCallTimeout, envData)
	// Validate duration format for timeout fields, fall back to defaults if invalid
	if agent.TurnLockTimeout != "" {
		if _, err := time.ParseDuration(agent.TurnLockTimeout); err != nil {
			if log := store.GetLogger(); log != nil {
				log.Warningf(
					"[agent] invalid turn_lock_timeout duration %q, using default %s",
					agent.TurnLockTimeout, AgentSessionDefaultTurnLockExpire,
				)
			}
			agent.TurnLockTimeout = ""
		}
	}
	if agent.DriverCallTimeout != "" {
		if _, err := time.ParseDuration(agent.DriverCallTimeout); err != nil {
			if log := store.GetLogger(); log != nil {
				log.Warningf(
					"[agent] invalid driver_call_timeout duration %q, using default %s",
					agent.DriverCallTimeout, AgentSessionDefaultDriverCallTimeout,
				)
			}
			agent.DriverCallTimeout = ""
		}
	}
	agent.AgentSettings = dipper.Interpolate("agent_", agent.AgentSettings, envData)
	tools := make([]config.AgentToolDef, len(agent.Tools))
	for i, tool := range agent.Tools {
		nt := tool
		nt.Name = dipper.InterpolateStr("agent_tool_name", tool.Name, envData)
		nt.Type = dipper.InterpolateStr("agent_tool_type", tool.Type, envData)
		tools[i] = nt
	}
	agent.Tools = tools
	if agent.CompactionPolicy != nil {
		cp := *agent.CompactionPolicy
		cp.SummarizationAgent = dipper.InterpolateStr("agent_compaction_summarization_agent", cp.SummarizationAgent, envData)
		cp.SummarizationPrompt = dipper.InterpolateStr("agent_compaction_summarization_prompt", cp.SummarizationPrompt, envData)
		agent.CompactionPolicy = &cp
	}

	return &agent
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
func (s *AgentSession) appendConvoHistory(msg *AgentMessage) {
	s.history = append(s.history, *msg)

	// Count message tokens and add to ContextTokens when TokenCounter is active.
	// This ensures ContextTokens = sum of all history message tokens by construction.
	if s.TokenCounter != nil {
		lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
			msg.InputTokens = cs.ContextTokens
			msg.OutputTokens = s.countMessageTokens(*msg)
			cs.ContextTokens += msg.OutputTokens
		})
	}

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
	if s.loadPreContextAndSkills() {
		return
	}
	text := dipper.MustGetMapDataStr(s.CurrentMsg.Payload, "text")
	user, _ := dipper.GetMapDataStr(s.CurrentMsg.Payload, "user")

	s.appendConvoHistory(&AgentMessage{
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

// countMessageTokens counts all tokens in a message including content, tool calls, and tool results.
// It uses the session's TokenCounter. This method should only be called when s.TokenCounter is not nil.
func (s *AgentSession) countMessageTokens(msg AgentMessage) int {
	total := 0

	// Count content tokens
	total += s.TokenCounter.CountTokens(msg.Content)

	// Count thoughts tokens
	total += s.TokenCounter.CountTokens(msg.Thoughts)

	// Count tool call tokens (function names and parameters)
	for _, tc := range msg.ToolCalls {
		total += s.TokenCounter.CountTokens(tc.FuncName)
		// Convert params to string for counting
		if tc.Params != nil {
			paramsStr := fmt.Sprintf("%v", tc.Params)
			total += s.TokenCounter.CountTokens(paramsStr)
		}
	}

	// Count tool result tokens
	for _, tr := range msg.ToolResult {
		resultStr := fmt.Sprintf("%v", tr)
		total += s.TokenCounter.CountTokens(resultStr)
	}

	return total
}

// countSystemPromptTokens counts tokens in the system prompt.
// It uses the session's TokenCounter. This method should only be called when s.TokenCounter is not nil.
func (s *AgentSession) countSystemPromptTokens() int {
	systemPrompt := s.Agent.SystemPrompt
	if s.Type == AgentSessionTypeInference && len(s.Agent.InferencePrompt) > 0 {
		systemPrompt = s.Agent.InferencePrompt
	}

	return s.TokenCounter.CountTokens(systemPrompt)
}

// getConvoContextTokens safely retrieves the current ContextTokens from ConvoState.
// It should only be called when s.TokenCounter is not nil.
func (s *AgentSession) getConvoContextTokens() int {
	cs := &ConvoState{}
	cs.load(s.ConvoID, s.store)

	return cs.ContextTokens
}

func (s *AgentSession) sendToDriver() {
	tools := s.BuildTools()
	timeout := s.CurrentMsg.Labels["timeout"]
	if timeout == "" {
		timeout = AgentSessionDefaultDriverCallTimeout
		if s.Agent != nil && s.Agent.DriverCallTimeout != "" {
			timeout = s.Agent.DriverCallTimeout
		}
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
	history = append(history, s.history...)

	if log := s.log(); log != nil {
		log.Infof("[agent] session [%s] sending to driver=%s engine=%s history_len=%d tools=%d",
			s.ID, s.Agent.Driver, s.Agent.Engine, len(history), len(tools))
	}
	s.InputTokens = 0
	dipper.Must(s.store.CallNoWait("driver:"+s.Agent.Driver, "send_to_model", map[string]interface{}{
		"engine":         s.Agent.Engine,
		"history":        history,
		"tools":          tools,
		"type":           s.Type,
		"model_data":     s.Agent.ModelData,
		"should_stream":  s.Agent.ShouldStream,
		"agent_settings": s.Agent.AgentSettings,
	}, "agent_session_id", s.ID, "timeout", timeout))
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
	if s.TokenCounter == nil {
		// handle streaming chunk, currently expect input/output tokens to be counted and returned.
		s.InputTokens += agentMsg.InputTokens
		s.OutputTokens += agentMsg.OutputTokens
		s.TotalTokens = s.InputTokens + s.OutputTokens
	}

	// Streaming chunk: non-complete agent content with no tool calls and not a thinking token.
	// Accumulate in PendingContent to avoid one Redis rpush per chunk.
	if agentMsg.Role == RoleAgent && !agentMsg.IsComplete {
		s.PendingContent += agentMsg.Content
		s.PendingThoughts += agentMsg.Thoughts

		s.NewPendingContent = s.NewPendingContent || len(agentMsg.Content) > 0
		s.NewPendingContent = s.NewPendingContent || s.Agent.ShouldEmitThoughts && len(agentMsg.Thoughts) > 0

		return
	}

	// Populate ConvoID on agent tool calls so the UI can render a
	// "View Sub-Agent Conversation" link immediately when the tool call
	// card appears, without waiting for the result.
	for i, tc := range agentMsg.ToolCalls {
		if strings.HasPrefix(tc.FuncName, "ag__") {
			oneShot, _ := dipper.GetMapDataBool(tc.Params, "one_shot")
			if oneShot {
				agentMsg.ToolCalls[i].ConvoID = dipper.NewUUID()
			} else {
				agentMsg.ToolCalls[i].ConvoID = fmt.Sprintf("%s-%s", s.ConvoID, tc.FuncName[len("ag__"):])
			}
		}
	}

	s.appendConvoHistory(agentMsg)
	if s.TokenCounter != nil {
		s.InputTokens += agentMsg.InputTokens
		s.OutputTokens += agentMsg.OutputTokens
		s.TotalTokens = s.InputTokens + s.OutputTokens
	}

	// Final agent message: the complete message added to the
	// history, reset the pending content and thoughts.
	if agentMsg.Role == RoleAgent {
		s.PendingContent = ""
		s.PendingThoughts = ""
		s.NewPendingContent = false
	}

	// Everything else: tool calls, non-agent messages.
	if len(agentMsg.ToolCalls) > 0 {
		s.CurrentCall = 0
		s.ToolCalls = agentMsg.ToolCalls
		s.nextToolCall()

		return
	}

	if agentMsg.Role != RoleAgent || !agentMsg.IsComplete {
		s.persist(false)
		s.sendToDriver()

		return
	}

	if s.ParentSessionID != "" {
		s.notifyParent(*agentMsg)
	}
}

func (s *AgentSession) processAgentPoll(msg *dipper.Message) {
	log := s.log()
	if log == nil {
		log = dipper.GetLogger("agent", "INFO")
	}

	log.Infof("[agent] session [%s] poll received lastpoll=%d resume_key %s", s.ID, s.LastPoll, msg.Labels["resume_key"])

	timeout := AgentSessionDefaultPollTimeout
	if t, ok := msg.Labels["timeout"]; ok {
		timeout = dipper.Must(time.ParseDuration(t)).(time.Duration)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		rateLimited := !s.LastPollTime.IsZero() && time.Since(s.LastPollTime) < MinPollInterval

		for (!s.NewPendingContent && s.LastPoll == len(s.history) && s.ErrorReason == "") || rateLimited {
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
			rateLimited = !s.LastPollTime.IsZero() && time.Since(s.LastPollTime) < MinPollInterval
		}

		if s.emitPollResponse(msg) {
			return
		}
	}
}

func (s *AgentSession) emitPollResponse(msg *dipper.Message) bool {
	fullMessages := []map[string]string{}
	for ; s.LastPoll < len(s.history); s.LastPoll++ {
		am := s.history[s.LastPoll]
		if am.Role != RoleAgent {
			continue
		}
		if am.IsThinking && !s.Agent.ShouldEmitThoughts {
			continue
		}
		currentMessage := map[string]string{
			"content":     am.Content,
			"is_thinking": strconv.FormatBool(am.IsThinking),
		}
		if s.Agent.ShouldEmitThoughts && len(am.Thoughts) > 0 {
			currentMessage["thoughts"] = am.Thoughts
		}
		if len(currentMessage["content"]) > 0 || len(currentMessage["thoughts"]) > 0 {
			fullMessages = append(fullMessages, currentMessage)
		}
	}

	ret := dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "agent_response",
		Payload: map[string]interface{}{
			"full_messages": fullMessages,
			"state": AgentState{
				HistoryLen:  len(s.history),
				TotalTokens: s.TotalTokens,
				ConvoID:     s.ConvoID,
			},
		},
	}

	last := s.history[len(s.history)-1]
	live := !last.IsComplete || last.Role != RoleAgent || len(last.ToolCalls) > 0
	live = live && len(s.ErrorReason) == 0

	if live && s.NewPendingContent {
		liveMessage := map[string]string{
			"content": s.PendingContent,
		}
		if s.Agent.ShouldEmitThoughts && len(s.PendingThoughts) > 0 {
			liveMessage["thoughts"] = s.PendingThoughts
		}
		ret.Payload.(map[string]interface{})["live_message"] = liveMessage
	}

	labels := msg.Labels
	labels["last_poll"] = strconv.Itoa(s.LastPoll)
	if s.ErrorReason != "" {
		labels["status"] = "error"
		labels["reason"] = s.ErrorReason
	} else {
		labels["status"] = "success"
	}

	if labels["status"] == "success" &&
		len(fullMessages) == 0 &&
		!s.NewPendingContent {
		return false
	}

	s.LastPollTime = time.Now()
	s.NewPendingContent = false
	s.store.EmitMessage(ret)

	return true
}
