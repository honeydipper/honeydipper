// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package agent provides shared types used by the agent engine and AI model drivers.
package agent

// Session type labels placed in message labels.
const (
	SessionTypeInference = "inference"
	SessionTypeChatTurn  = "chat_turn"
)

// Role labels used in conversation history entries.
const (
	RoleSystem     = "system"
	RoleUser       = "user"
	RoleAgent      = "agent"
	RoleTool       = "tool"
	RoleToolResult = "tool_result"
)

// Message is a single entry in the conversation history exchanged between the engine and drivers.
type Message struct {
	Role         string
	User         string                   // optional; distinguishes speakers in multi-user conversations.
	IsThinking   bool                     // true when the message carries intermediate model reasoning.
	ToolCalls    []ToolCall               // non-empty when the model requests tool invocations.
	ToolResult   []map[string]interface{} // non-empty for tool-result messages; Content will be empty.
	Content      string                   `json:"content" mapstructure:"content"`
	IsComplete   bool                     `json:"is_complete" mapstructure:"is_complete"`
	InputTokens  int                      `json:"input_tokens" mapstructure:"input_tokens"`
	OutputTokens int                      `json:"output_tokens" mapstructure:"output_tokens"`
}

// Tool describes a callable tool exposed to the model.
type Tool struct {
	Name        string
	Description string
	Params      map[string]interface{}
}

// ToolCall records a single tool invocation requested by the model.
type ToolCall struct {
	FuncName string
	Params   map[string]interface{}
}

// State reports the runtime state of an agent session at the end of a turn.
// It is included in every agent_response payload so the workflow engine can
// make decisions such as triggering history compaction.
type State struct {
	HistoryLen  int    `json:"history_len" mapstructure:"history_len"`
	TotalTokens int    `json:"total_tokens" mapstructure:"total_tokens"`
	ConvoID     string `json:"convo_id" mapstructure:"convo_id"`
}
