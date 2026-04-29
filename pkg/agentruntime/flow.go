// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package agentruntime

import (
	"fmt"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/agenthistory"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

type enqueueProviderFunc func(*dipper.Message) error

type definitionResolver func(string) (config.Agent, bool)

func PersistActivation(caller dipper.RPCCaller, msg *dipper.Message, agentName string) (*ActivationPersistResult, error) {
	labels := map[string]string{}
	for k, v := range msg.Labels {
		labels[k] = v
	}

	sourceSessionID := labels["sourceSessionID"]
	conversationID := resolveConversationID(msg, sourceSessionID)
	idxKey := SessionIndexKey(agentName, conversationID)

	sessionID := ""
	rawSessionID, err := caller.Call("cache", "load", map[string]any{"key": idxKey})
	if err != nil {
		return nil, fmt.Errorf("load session index: %w", err)
	}
	if len(rawSessionID) > 0 {
		sessionID = string(rawSessionID)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sess := &Session{}
	if sessionID != "" {
		loaded, lerr := LoadJSON(caller, SessionKey(sessionID), sess)
		if lerr != nil {
			return nil, fmt.Errorf("load existing session %s: %w", sessionID, lerr)
		}
		if !loaded {
			sessionID = ""
		}
	}
	if sessionID == "" {
		sessionID = dipper.NewUUID()
		sess = &Session{
			ID:              sessionID,
			Agent:           agentName,
			ConversationID:  conversationID,
			State:           StateCreated,
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
	turn := &Turn{
		ID:              turnID,
		SessionID:       sessionID,
		Agent:           agentName,
		State:           StateCreated,
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

	sess.State = StateResolvingContext
	sess.CurrentTurnID = turnID
	sess.UpdatedAt = now

	if err := SaveJSON(caller, SessionKey(sessionID), sess, SessionTTL); err != nil {
		return nil, fmt.Errorf("persist session %s: %w", sessionID, err)
	}
	if err := SaveJSON(caller, TurnKey(turnID), turn, TurnTTL); err != nil {
		return nil, fmt.Errorf("persist turn %s: %w", turnID, err)
	}
	if _, err := caller.Call("cache", "save", map[string]any{
		"key":   idxKey,
		"value": sessionID,
		"ttl":   SessionTTL,
	}); err != nil {
		return nil, fmt.Errorf("save session index: %w", err)
	}

	return &ActivationPersistResult{SessionID: sessionID, TurnID: turnID}, nil
}

func ResolveTurnContext(
	caller dipper.RPCCaller,
	sessionID string,
	turnID string,
	resolveDef definitionResolver,
	enqueue enqueueProviderFunc,
) error {
	sess := &Session{}
	loaded, err := LoadJSON(caller, SessionKey(sessionID), sess)
	if err != nil {
		return fmt.Errorf("load session %s: %w", sessionID, err)
	}
	if !loaded {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	turn := &Turn{}
	loaded, err = LoadJSON(caller, TurnKey(turnID), turn)
	if err != nil {
		return fmt.Errorf("load turn %s: %w", turnID, err)
	}
	if !loaded {
		return fmt.Errorf("%w: %s", ErrTurnNotFound, turnID)
	}

	history, err := LoadHistory(caller, HistoryKey(sess.Agent, sess.ConversationID))
	if err != nil {
		return fmt.Errorf("load agent history: %w", err)
	}
	if len(history) == 0 {
		history, err = seedAgentHistory(caller, sess.Agent, sess.ConversationID, resolveDef)
		if err != nil {
			return err
		}
	}

	provider := resolveProvider(sess.Agent, turn.Ctx, resolveDef)
	tools := resolveTools(sess.Agent, resolveDef)

	resolved := &ResolvedTurnContext{
		ConversationID: sess.ConversationID,
		History:        history,
		Provider:       provider,
		Tools:          tools,
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

	now := time.Now().UTC().Format(time.RFC3339Nano)
	turn.State = StateStartingProvider
	turn.FailureReason = ""
	turn.Provider = provider
	turn.Tools = tools
	turn.ResolvedContext = resolved
	turn.UpdatedAt = now

	sess.State = StateStartingProvider
	sess.UpdatedAt = now

	if err := SaveJSON(caller, TurnKey(turnID), turn, TurnTTL); err != nil {
		return fmt.Errorf("save context-resolved turn %s: %w", turnID, err)
	}
	if err := SaveJSON(caller, SessionKey(sessionID), sess, SessionTTL); err != nil {
		return fmt.Errorf("save context-resolved session %s: %w", sessionID, err)
	}

	if err := startProviderTurn(sess, turn, enqueue); err != nil {
		return failTurnStart(caller, sess, turn, err)
	}

	now = time.Now().UTC().Format(time.RFC3339Nano)
	turn.State = StateWaitingProvider
	turn.ProviderStart = nil
	turn.UpdatedAt = now

	sess.State = StateWaitingProvider
	sess.UpdatedAt = now

	if err := SaveJSON(caller, TurnKey(turnID), turn, TurnTTL); err != nil {
		return fmt.Errorf("save waiting-provider turn %s: %w", turnID, err)
	}
	if err := SaveJSON(caller, SessionKey(sessionID), sess, SessionTTL); err != nil {
		return fmt.Errorf("save waiting-provider session %s: %w", sessionID, err)
	}

	return nil
}

func ContinueProviderTurn(caller dipper.RPCCaller, msg *dipper.Message, enqueue enqueueProviderFunc) error {
	sessionID, ok := msg.Labels["sessionID"]
	if !ok || sessionID == "" {
		return nil
	}
	turnID, ok := msg.Labels["turnID"]
	if !ok || turnID == "" {
		return nil
	}

	sess := &Session{}
	loaded, err := LoadJSON(caller, SessionKey(sessionID), sess)
	if err != nil || !loaded {
		return err
	}
	if sess.CurrentTurnID != turnID {
		return nil
	}

	turn := &Turn{}
	loaded, err = LoadJSON(caller, TurnKey(turnID), turn)
	if err != nil || !loaded {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := msg.Labels["status"]
	if status == "error" || status == "failure" {
		reason := msg.Labels["reason"]
		if strings.TrimSpace(reason) == "" {
			reason = "provider command failed"
		}
		turn.State = StateFailed
		turn.FailureReason = reason
		sess.State = StateFailed
	} else {
		turnState, sessionState, failureReason := handleProviderSuccess(sess, turn, msg.Payload, enqueue)
		turn.State = turnState
		sess.State = sessionState
		turn.FailureReason = failureReason
	}
	turn.UpdatedAt = now
	sess.UpdatedAt = now

	if err := SaveJSON(caller, TurnKey(turnID), turn, TurnTTL); err != nil {
		return err
	}

	return SaveJSON(caller, SessionKey(sessionID), sess, SessionTTL)
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

func seedAgentHistory(
	caller dipper.RPCCaller,
	agentName string,
	conversationID string,
	resolveDef definitionResolver,
) ([]agenthistory.TurnHistoryRecord, error) {
	prompt := getAgentSystemPrompt(agentName, resolveDef)
	if strings.TrimSpace(prompt) == "" {
		return []agenthistory.TurnHistoryRecord{}, nil
	}

	key := HistoryKey(agentName, conversationID)
	history, err := SeedHistory(caller, key, prompt, SessionTTL)
	if err != nil {
		return nil, fmt.Errorf("seed agent history: %w", err)
	}

	return history, nil
}

func startProviderTurn(sess *Session, turn *Turn, enqueue enqueueProviderFunc) error {
	if strings.TrimSpace(turn.Provider) == "" {
		return ErrProviderMissing
	}

	params := map[string]any{
		"user":   "agent",
		"prompt": turn.Prompt,
		"convID": sess.ConversationID,
	}
	if ctxMap, ok := turn.Ctx.(map[string]interface{}); ok {
		if user, ok := ctxMap["user"].(string); ok && strings.TrimSpace(user) != "" {
			params["user"] = user
		}
		if engine, ok := ctxMap["engine"].(string); ok && strings.TrimSpace(engine) != "" {
			params["engine"] = engine
		}
	}

	payload := map[string]any{
		"function": config.Function{
			Driver:    turn.Provider,
			RawAction: "chat",
		},
		"data":  params,
		"event": turn.Event,
		"ctx":   turn.Ctx,
	}

	labels := map[string]string{}
	for k, v := range turn.Labels {
		labels[k] = v
	}
	labels["sessionID"] = sess.ID
	labels["turnID"] = turn.ID
	labels["provider"] = turn.Provider

	cmd := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: dipper.EventbusAgentCommand,
		Payload: payload,
		Labels:  labels,
	}

	if err := enqueue(cmd); err != nil {
		return fmt.Errorf("start provider command: %w", err)
	}

	return nil
}

func handleProviderSuccess(sess *Session, turn *Turn, payload interface{}, enqueue enqueueProviderFunc) (string, string, string) {
	turn.ProviderStart = payload
	if !ShouldReceiveTurnChunk(payload) {
		return StateStreamingComplete, StateStreamingComplete, ""
	}
	if err := requestTurnChunk(sess, turn, payload, enqueue); err != nil {
		return StateFailed, StateFailed, err.Error()
	}

	return StateWaitingProviderChunk, StateWaitingProviderChunk, ""
}

func requestTurnChunk(sess *Session, turn *Turn, payload interface{}, enqueue enqueueProviderFunc) error {
	if strings.TrimSpace(turn.Provider) == "" {
		return ErrProviderMissing
	}
	convID, counter, err := ChunkRef(payload)
	if err != nil {
		return fmt.Errorf("parse provider chunk reference: %w", err)
	}

	cmdPayload := map[string]any{
		"function": config.Function{
			Driver:    turn.Provider,
			RawAction: "chatContinue",
		},
		"data": map[string]any{
			"convID":  convID,
			"counter": counter,
		},
	}

	labels := map[string]string{}
	for k, v := range turn.Labels {
		labels[k] = v
	}
	labels["sessionID"] = sess.ID
	labels["turnID"] = turn.ID
	labels["provider"] = turn.Provider

	cmd := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: dipper.EventbusAgentCommand,
		Payload: cmdPayload,
		Labels:  labels,
	}

	if err := enqueue(cmd); err != nil {
		return fmt.Errorf("request provider turn chunk: %w", err)
	}

	return nil
}

func failTurnStart(caller dipper.RPCCaller, sess *Session, turn *Turn, reason error) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	turn.State = StateFailed
	turn.FailureReason = reason.Error()
	turn.UpdatedAt = now

	sess.State = StateFailed
	sess.UpdatedAt = now

	if err := SaveJSON(caller, TurnKey(turn.ID), turn, TurnTTL); err != nil {
		return fmt.Errorf("save failed turn %s: %w", turn.ID, err)
	}
	if err := SaveJSON(caller, SessionKey(sess.ID), sess, SessionTTL); err != nil {
		return fmt.Errorf("save failed session %s: %w", sess.ID, err)
	}

	return reason
}

func resolveProvider(agentName string, ctxPayload interface{}, resolveDef definitionResolver) string {
	if provider, ok := getProviderFromCtx(ctxPayload); ok {
		return provider
	}

	def, ok := resolveDef(agentName)
	if !ok {
		return ""
	}
	if strings.TrimSpace(def.Provider) != "" {
		return def.Provider
	}
	for _, candidate := range def.Providers {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}

	return ""
}

func resolveTools(agentName string, resolveDef definitionResolver) interface{} {
	def, ok := resolveDef(agentName)
	if !ok {
		return nil
	}

	return def.Tools
}

func getAgentSystemPrompt(agentName string, resolveDef definitionResolver) string {
	def, ok := resolveDef(agentName)
	if !ok {
		return ""
	}
	if strings.TrimSpace(def.SystemPrompt) != "" {
		return def.SystemPrompt
	}

	return def.Prompt
}

func getProviderFromCtx(ctxPayload interface{}) (string, bool) {
	ctxMap, ok := ctxPayload.(map[string]interface{})
	if !ok {
		return "", false
	}
	p, ok := ctxMap["provider"]
	if !ok {
		return "", false
	}
	provider, ok := p.(string)
	if !ok || strings.TrimSpace(provider) == "" {
		return "", false
	}

	return provider, true
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
