// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package agenthistory

import (
	"encoding/json"
	"errors"
	"fmt"
)

var errUnsupportedMessageContentFormat = errors.New("unsupported message content format")

// Role is a provider-agnostic chat role shared by agent and driver integrations.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentPartType represents multimodal content variants used by common AI APIs.
type ContentPartType string

const (
	ContentPartTypeText       ContentPartType = "text"
	ContentPartTypeImageURL   ContentPartType = "image_url"
	ContentPartTypeInputText  ContentPartType = "input_text"
	ContentPartTypeInputImage ContentPartType = "input_image"
)

// ContentPart is an abstract multimodal content unit.
type ContentPart struct {
	Type     ContentPartType `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL string          `json:"image_url,omitempty"`
	MimeType string          `json:"mime_type,omitempty"`
	Data     string          `json:"data,omitempty"`
}

// ToolCall is a provider-neutral representation of assistant tool call requests.
type ToolCall struct {
	ID        string          `json:"id,omitempty"`
	Type      string          `json:"type,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// MessageContent supports both plain string content and multi-part content arrays.
type MessageContent struct {
	Text  string
	Parts []ContentPart
}

// UnmarshalJSON accepts either a string content body or an array of content parts.
func (c *MessageContent) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*c = MessageContent{}

		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		c.Text = text
		c.Parts = nil

		return nil
	}

	var parts []ContentPart
	if err := json.Unmarshal(data, &parts); err == nil {
		c.Text = ""
		c.Parts = parts

		return nil
	}

	return errUnsupportedMessageContentFormat
}

// MarshalJSON preserves string form for text-only content and array form for multi-part content.
func (c MessageContent) MarshalJSON() ([]byte, error) {
	if len(c.Parts) > 0 {
		b, err := json.Marshal(c.Parts)
		if err != nil {
			return nil, fmt.Errorf("marshal content parts: %w", err)
		}

		return b, nil
	}

	b, err := json.Marshal(c.Text)
	if err != nil {
		return nil, fmt.Errorf("marshal text content: %w", err)
	}

	return b, nil
}

// TurnHistoryRecord is a provider-neutral history message shape for agent turns.
type TurnHistoryRecord struct {
	ID         string          `json:"id,omitempty"`
	Role       Role            `json:"role"`
	Name       string          `json:"name,omitempty"`
	Content    *MessageContent `json:"content,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	Metadata   map[string]any  `json:"metadata,omitempty"`
}

// ParseHistory parses JSON-encoded history records.
func ParseHistory(raw []byte) ([]TurnHistoryRecord, error) {
	if len(raw) == 0 {
		return []TurnHistoryRecord{}, nil
	}

	history := []TurnHistoryRecord{}
	if err := json.Unmarshal(raw, &history); err != nil {
		return nil, fmt.Errorf("parse history records: %w", err)
	}

	return history, nil
}
