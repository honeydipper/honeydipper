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
	Role       string
	User       string                   // optional; distinguishes speakers in multi-user conversations.
	IsThinking bool                     // true when the message carries intermediate model reasoning.
	ToolCalls  []ToolCall               // non-empty when the model requests tool invocations.
	ToolResult []map[string]interface{} // non-empty for tool-result messages; Content will be empty.
	Content    string                   `json:"content" mapstructure:"content"`         // the main content of the message.
	IsComplete bool                     `json:"is_complete" mapstructure:"is_complete"` // true when this is the final (non-streaming) message for a turn.
	ChunkSeq   int                      // sequence number for ordering message chunks within a turn.
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
