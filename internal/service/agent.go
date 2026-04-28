// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/agenthistory"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const (
	agentSessionTTL      = "168h"
	agentTurnTTL         = "72h"
	agentSessionPrefix   = "agent:session:"
	agentTurnPrefix      = "agent:turn:"
	agentHistoryPrefix   = "agent:history:"
	agentSessionIndexKey = "agent:session:index:"
)

type agentSession struct {
	ID              string `json:"id"`
	Agent           string `json:"agent"`
	ConversationID  string `json:"conversation_id"`
	State           string `json:"state"`
	CurrentTurnID   string `json:"current_turn_id"`
	SourceSessionID string `json:"source_session_id,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type agentTurn struct {
	ID              string               `json:"id"`
	SessionID       string               `json:"session_id"`
	Agent           string               `json:"agent"`
	State           string               `json:"state"`
	Provider        string               `json:"provider,omitempty"`
	Prompt          string               `json:"prompt,omitempty"`
	SourceSessionID string               `json:"source_session_id,omitempty"`
	Labels          map[string]string    `json:"labels,omitempty"`
	Event           interface{}          `json:"event,omitempty"`
	Ctx             interface{}          `json:"ctx,omitempty"`
	Workflow        interface{}          `json:"workflow,omitempty"`
	Tools           interface{}          `json:"tools,omitempty"`
	ResolvedContext *resolvedTurnContext `json:"resolved_context,omitempty"`
	CreatedAt       string               `json:"created_at"`
	UpdatedAt       string               `json:"updated_at"`
}

type resolvedTurnContext struct {
	ConversationID string                           `json:"conversation_id"`
	History        []agenthistory.TurnHistoryRecord `json:"history"`
	Event          interface{}                      `json:"event,omitempty"`
	Ctx            interface{}                      `json:"ctx,omitempty"`
	Workflow       interface{}                      `json:"workflow,omitempty"`
	Tools          interface{}                      `json:"tools,omitempty"`
}

type activationPersistResult struct {
	SessionID string
	TurnID    string
}

var (
	agent                   *Service
	agentMatchCounter       int64
	agentPendingTurns       int64
	errAgentSessionNotFound = errors.New("agent session not found")
	errAgentTurnNotFound    = errors.New("agent turn not found")
)

var (
	persistActivationFn = persistAgentActivation
	resolveContextFn    = resolveTurnContext
)

// StartAgent starts the agent service.
func StartAgent(cfg *config.Config) {
	agent = NewService(cfg, "agent")

	agent.EmitMetrics = agentMetrics
	agent.addResponder("eventbus:activate", createActivations)

	agent.start()
}

func createActivations(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] continue processing activate rules")
	<-agent.Ready()
	msg = dipper.DeserializePayload(msg)

	agentName, ok := dipper.GetMapDataStr(msg.Payload, "agent")
	if !ok || agentName == "" {
		return
	}

	result, err := persistActivationFn(agent, msg, agentName)
	if err != nil {
		dipper.Logger.Warningf("[agent] failed to persist activation: %+v", err)

		return
	}
	if err := resolveContextFn(agent, result.SessionID, result.TurnID); err != nil {
		dipper.Logger.Warningf("[agent] failed resolving context for session %s turn %s: %+v", result.SessionID, result.TurnID, err)

		return
	}

	atomic.AddInt64(&agentMatchCounter, 1)
	dipper.Logger.Infof("[agent] activation resolved for agent %s session %s turn %s", agentName, result.SessionID, result.TurnID)
}

func agentMetrics() {
	agent.GaugeSet("honey.honeydipper.agent.activations", strconv.FormatInt(atomic.LoadInt64(&agentMatchCounter), 10), []string{})
	agent.GaugeSet("honey.honeydipper.agent.pending_turns", strconv.FormatInt(atomic.LoadInt64(&agentPendingTurns), 10), []string{})
}

func persistAgentActivation(caller dipper.RPCCaller, msg *dipper.Message, agentName string) (*activationPersistResult, error) {
	labels := map[string]string{}
	for k, v := range msg.Labels {
		labels[k] = v
	}

	sourceSessionID := labels["sourceSessionID"]
	conversationID := resolveConversationID(msg, sourceSessionID)
	idxKey := agentSessionIndexKey + agentName + ":" + conversationID

	sessionID := ""
	rawSessionID, err := caller.Call("cache", "load", map[string]any{"key": idxKey})
	if err != nil {
		return nil, fmt.Errorf("load session index: %w", err)
	}
	if len(rawSessionID) > 0 {
		sessionID = string(rawSessionID)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sess := &agentSession{}
	if sessionID != "" {
		loaded, lerr := loadJSON(caller, agentSessionPrefix+sessionID, sess)
		if lerr != nil {
			return nil, lerr
		}
		if !loaded {
			sessionID = ""
		}
	}
	if sessionID == "" {
		sessionID = dipper.NewUUID()
		sess = &agentSession{
			ID:              sessionID,
			Agent:           agentName,
			ConversationID:  conversationID,
			State:           "created",
			SourceSessionID: sourceSessionID,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
	}

	turnID := dipper.NewUUID()
	provider, _ := dipper.GetMapDataStr(msg.Payload, "provider")
	prompt, _ := dipper.GetMapDataStr(msg.Payload, "prompt")
	eventData, _ := dipper.GetMapData(msg.Payload, "event")
	ctxData, _ := dipper.GetMapData(msg.Payload, "ctx")
	workflowData, _ := dipper.GetMapData(msg.Payload, "workflow")
	toolsData, _ := dipper.GetMapData(msg.Payload, "tools")
	turn := &agentTurn{
		ID:              turnID,
		SessionID:       sessionID,
		Agent:           agentName,
		State:           "created",
		Provider:        provider,
		Prompt:          prompt,
		SourceSessionID: sourceSessionID,
		Labels:          labels,
		Event:           eventData,
		Ctx:             ctxData,
		Workflow:        workflowData,
		Tools:           toolsData,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	sess.State = "resolving_context"
	sess.CurrentTurnID = turnID
	sess.UpdatedAt = now

	if err := saveJSON(caller, agentSessionPrefix+sessionID, sess, agentSessionTTL); err != nil {
		return nil, err
	}
	if err := saveJSON(caller, agentTurnPrefix+turnID, turn, agentTurnTTL); err != nil {
		return nil, err
	}
	if _, err := caller.Call("cache", "save", map[string]any{
		"key":   idxKey,
		"value": sessionID,
		"ttl":   agentSessionTTL,
	}); err != nil {
		return nil, fmt.Errorf("save session index: %w", err)
	}

	atomic.AddInt64(&agentPendingTurns, 1)

	return &activationPersistResult{SessionID: sessionID, TurnID: turnID}, nil
}

func resolveConversationID(msg *dipper.Message, sourceSessionID string) string {
	if msg == nil {
		return dipper.NewUUID()
	}
	if cid, ok := getConversationIDFromCtx(msg.Payload); ok {
		return cid
	}
	if sourceSessionID != "" {
		return sourceSessionID
	}
	if eventID := msg.Labels["eventID"]; eventID != "" {
		return eventID
	}

	return dipper.NewUUID()
}

func resolveTurnContext(caller dipper.RPCCaller, sessionID string, turnID string) error {
	sess := &agentSession{}
	loaded, err := loadJSON(caller, agentSessionPrefix+sessionID, sess)
	if err != nil {
		return err
	}
	if !loaded {
		return fmt.Errorf("%w: %s", errAgentSessionNotFound, sessionID)
	}

	turn := &agentTurn{}
	loaded, err = loadJSON(caller, agentTurnPrefix+turnID, turn)
	if err != nil {
		return err
	}
	if !loaded {
		return fmt.Errorf("%w: %s", errAgentTurnNotFound, turnID)
	}

	history, err := loadAgentHistory(caller, agentHistoryKey(sess.Agent, sess.ConversationID))
	if err != nil {
		return err
	}
	if len(history) == 0 {
		history, err = seedAgentHistory(caller, sess.Agent, sess.ConversationID)
		if err != nil {
			return err
		}
	}

	resolved := &resolvedTurnContext{
		ConversationID: sess.ConversationID,
		History:        history,
	}
	if turn.Event != nil {
		resolved.Event = turn.Event
	}
	if turn.Ctx != nil {
		resolved.Ctx = turn.Ctx
	}
	if turn.Workflow != nil {
		resolved.Workflow = turn.Workflow
	}
	if turn.Tools != nil {
		resolved.Tools = turn.Tools
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	turn.State = "context_resolved"
	turn.ResolvedContext = resolved
	turn.UpdatedAt = now

	sess.State = "selecting_provider"
	sess.UpdatedAt = now

	if err := saveJSON(caller, agentTurnPrefix+turnID, turn, agentTurnTTL); err != nil {
		return err
	}
	if err := saveJSON(caller, agentSessionPrefix+sessionID, sess, agentSessionTTL); err != nil {
		return err
	}

	return nil
}

func agentHistoryKey(agentName string, conversationID string) string {
	return agentHistoryPrefix + agentName + ":" + conversationID
}

func loadAgentHistory(caller dipper.RPCCaller, key string) ([]agenthistory.TurnHistoryRecord, error) {
	raw, err := caller.Call("cache", "lrange", map[string]any{"key": key})
	if err != nil {
		return nil, fmt.Errorf("load history %s: %w", key, err)
	}
	history, err := agenthistory.ParseHistory(raw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal history %s: %w", key, err)
	}

	return history, nil
}

func seedAgentHistory(caller dipper.RPCCaller, agentName string, conversationID string) ([]agenthistory.TurnHistoryRecord, error) {
	prompt := getAgentSystemPrompt(agentName)
	if strings.TrimSpace(prompt) == "" {
		return []agenthistory.TurnHistoryRecord{}, nil
	}

	record := agenthistory.TurnHistoryRecord{
		Role: agenthistory.RoleSystem,
		Content: &agenthistory.MessageContent{
			Text: prompt,
		},
	}
	key := agentHistoryKey(agentName, conversationID)
	b, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal seeded history %s: %w", key, err)
	}
	if _, err := caller.Call("cache", "rpush", map[string]any{
		"key":   key,
		"value": string(b),
		"ttl":   agentSessionTTL,
	}); err != nil {
		return nil, fmt.Errorf("seed history %s: %w", key, err)
	}

	return []agenthistory.TurnHistoryRecord{record}, nil
}

func getAgentSystemPrompt(agentName string) string {
	if agent == nil || agent.config == nil || agent.config.DataSet == nil {
		return ""
	}
	def, ok := agent.config.DataSet.Agents[agentName]
	if !ok {
		return ""
	}
	if strings.TrimSpace(def.SystemPrompt) != "" {
		return def.SystemPrompt
	}

	return def.Prompt
}

func getConversationIDFromCtx(payload interface{}) (string, bool) {
	ctxData, ok := dipper.GetMapData(payload, "ctx")
	if !ok {
		return "", false
	}
	ctxMap, ok := ctxData.(map[string]interface{})
	if !ok {
		return "", false
	}
	cidData, ok := ctxMap["conversation_id"]
	if !ok {
		return "", false
	}
	cid, ok := cidData.(string)
	if !ok || strings.TrimSpace(cid) == "" {
		return "", false
	}

	return cid, true
}

func saveJSON(caller dipper.RPCCaller, key string, data interface{}, ttl string) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}
	_, err = caller.Call("cache", "save", map[string]any{
		"key":   key,
		"value": string(b),
		"ttl":   ttl,
	})
	if err != nil {
		return fmt.Errorf("save %s: %w", key, err)
	}

	return nil
}

func loadJSON(caller dipper.RPCCaller, key string, out interface{}) (bool, error) {
	raw, err := caller.Call("cache", "load", map[string]any{"key": key})
	if err != nil {
		return false, fmt.Errorf("load %s: %w", key, err)
	}
	if len(raw) == 0 {
		return false, nil
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return false, fmt.Errorf("unmarshal %s: %w", key, err)
	}

	return true, nil
}
