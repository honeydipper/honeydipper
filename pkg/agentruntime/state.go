// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package agentruntime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/honeydipper/honeydipper/v4/pkg/agenthistory"
)

const (
	SessionTTL = "168h"
	TurnTTL    = "72h"

	SessionPrefix      = "agent:session:"
	TurnPrefix         = "agent:turn:"
	HistoryPrefix      = "agent:history:"
	SessionIndexPrefix = "agent:session:index:"

	StateCreated              = "created"
	StateResolvingContext     = "resolving_context"
	StateStartingProvider     = "starting_provider"
	StateWaitingProvider      = "waiting_provider"
	StateWaitingProviderChunk = "waiting_provider_chunk"
	StateStreamingComplete    = "streaming_complete"
	StateFailed               = "failed"
)

var (
	ErrSessionNotFound        = errors.New("agent session not found")
	ErrTurnNotFound           = errors.New("agent turn not found")
	ErrProviderMissing        = errors.New("agent provider not configured")
	ErrInvalidProviderPayload = errors.New("invalid provider response payload")
	ErrChunkRefMissing        = errors.New("provider response missing convID/counter")
)

type Session struct {
	ID              string `json:"id"`
	Agent           string `json:"agent"`
	ConversationID  string `json:"conversation_id"`
	State           string `json:"state"`
	CurrentTurnID   string `json:"current_turn_id"`
	SourceSessionID string `json:"source_session_id,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type Turn struct {
	ID              string               `json:"id"`
	SessionID       string               `json:"session_id"`
	Agent           string               `json:"agent"`
	State           string               `json:"state"`
	FailureReason   string               `json:"failure_reason,omitempty"`
	Provider        string               `json:"provider,omitempty"`
	ProviderStart   interface{}          `json:"provider_start,omitempty"`
	Prompt          string               `json:"prompt,omitempty"`
	SourceSessionID string               `json:"source_session_id,omitempty"`
	Labels          map[string]string    `json:"labels,omitempty"`
	Event           interface{}          `json:"event,omitempty"`
	Ctx             interface{}          `json:"ctx,omitempty"`
	Workflow        interface{}          `json:"workflow,omitempty"`
	Tools           interface{}          `json:"tools,omitempty"`
	ResolvedContext *ResolvedTurnContext `json:"resolved_context,omitempty"`
	CreatedAt       string               `json:"created_at"`
	UpdatedAt       string               `json:"updated_at"`
}

type ResolvedTurnContext struct {
	ConversationID string                           `json:"conversation_id"`
	History        []agenthistory.TurnHistoryRecord `json:"history"`
	Provider       string                           `json:"provider,omitempty"`
	Event          interface{}                      `json:"event,omitempty"`
	Ctx            interface{}                      `json:"ctx,omitempty"`
	Workflow       interface{}                      `json:"workflow,omitempty"`
	Tools          interface{}                      `json:"tools,omitempty"`
}

type ActivationPersistResult struct {
	SessionID string
	TurnID    string
}

func SessionKey(sessionID string) string {
	return SessionPrefix + sessionID
}

func TurnKey(turnID string) string {
	return TurnPrefix + turnID
}

func HistoryKey(agentName string, conversationID string) string {
	return HistoryPrefix + agentName + ":" + conversationID
}

func SessionIndexKey(agentName string, conversationID string) string {
	return SessionIndexPrefix + agentName + ":" + conversationID
}

func ShouldReceiveTurnChunk(payload interface{}) bool {
	convID, counter, err := ChunkRef(payload)
	if err != nil || strings.TrimSpace(convID) == "" || strings.TrimSpace(counter) == "" {
		return false
	}
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		return false
	}
	done, doneOK := payloadMap["done"].(bool)

	return !doneOK || !done
}

func ChunkRef(payload interface{}) (string, string, error) {
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		return "", "", ErrInvalidProviderPayload
	}
	convID, convOK := payloadMap["convID"].(string)
	counter, counterOK := payloadMap["counter"].(string)
	if !convOK || !counterOK || strings.TrimSpace(convID) == "" || strings.TrimSpace(counter) == "" {
		return "", "", ErrChunkRefMissing
	}

	return convID, counter, nil
}

func WrapNotFound(err error, id string) error {
	return fmt.Errorf("%w: %s", err, id)
}
