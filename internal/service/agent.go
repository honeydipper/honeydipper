// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const (
	agentSessionTTL      = "168h"
	agentTurnTTL         = "72h"
	agentSessionPrefix   = "agent:session:"
	agentTurnPrefix      = "agent:turn:"
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
	ID              string            `json:"id"`
	SessionID       string            `json:"session_id"`
	Agent           string            `json:"agent"`
	State           string            `json:"state"`
	Provider        string            `json:"provider,omitempty"`
	Prompt          string            `json:"prompt,omitempty"`
	SourceSessionID string            `json:"source_session_id,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Event           interface{}       `json:"event,omitempty"`
	Ctx             interface{}       `json:"ctx,omitempty"`
	Workflow        interface{}       `json:"workflow,omitempty"`
	Tools           interface{}       `json:"tools,omitempty"`
	CreatedAt       string            `json:"created_at"`
	UpdatedAt       string            `json:"updated_at"`
}

type activationPersistResult struct {
	SessionID string
	TurnID    string
}

var (
	agent             *Service
	agentMatchCounter int64
	agentPendingTurns int64
)

var persistActivationFn = persistAgentActivation

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

	atomic.AddInt64(&agentMatchCounter, 1)
	dipper.Logger.Infof("[agent] activation persisted for agent %s session %s turn %s", agentName, result.SessionID, result.TurnID)
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
