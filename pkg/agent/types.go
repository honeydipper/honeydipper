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
	Thoughts     string                   `json:"thoughts" mapstructure:"thoughts"` // optional field for model reasoning text
	IsComplete   bool                     `json:"is_complete" mapstructure:"is_complete"`
	IsChunk      bool                     `json:"is_chunk" mapstructure:"is_chunk"` // true when the message is a streaming chunk
	InputTokens  int                      `json:"input_tokens" mapstructure:"input_tokens"`
	OutputTokens int                      `json:"output_tokens" mapstructure:"output_tokens"`

	// IsSlash marks a message that originated from a slash command (the command
	// text itself or its reply). Slash-origin messages are persisted in the
	// conversation history so the workflow/UI return paths can observe them, but
	// they are NEVER sent to the model as context. This field is persisted to
	// Redis as part of the serialized message so markers survive session restore.
	IsSlash bool `json:"is_slash,omitempty" mapstructure:"is_slash"`
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
	ConvoID  string // populated for agent tool calls (ag__*) to link to the sub-agent's conversation
}

// State reports the runtime state of an agent session at the end of a turn.
// It is included in every agent_response payload so the workflow engine can
// make decisions such as triggering history compaction.
type State struct {
	HistoryLen  int    `json:"history_len" mapstructure:"history_len"`
	TotalTokens int    `json:"total_tokens" mapstructure:"total_tokens"`
	ConvoID     string `json:"convo_id" mapstructure:"convo_id"`
}

// CompactionStrategy defines how conversation history should be compacted
// when it exceeds the configured threshold.
type CompactionStrategy string

const (
	// CompactionStrategySummarize uses an LLM summarization agent to compress
	// older conversation history into a concise summary, preserving the most
	// recent messages verbatim.
	CompactionStrategySummarize CompactionStrategy = "summarize"
)

// CompactionPolicy configures the automatic compaction behavior for an agent's
// conversation history. Compaction is triggered lazily before sending history
// to the model when the threshold is exceeded.
type CompactionPolicy struct {
	// Strategy selects the compaction method. Currently only "summarize" is supported.
	Strategy CompactionStrategy `json:"strategy" mapstructure:"strategy"`

	// Threshold is the value at which compaction triggers, interpreted according
	// to ThresholdType. When the current value equals or exceeds Threshold,
	// compaction runs before the next model invocation.
	Threshold int `json:"threshold" mapstructure:"threshold"`

	// ThresholdType specifies what metric Threshold refers to.
	// Supported values:
	//   - "history_len":   number of messages in the conversation history
	//   - "total_tokens":  cumulative input + output tokens across the session
	ThresholdType string `json:"threshold_type" mapstructure:"threshold_type"`

	// PreserveRecent is the number of most recent conversation messages that
	// should always be kept verbatim after compaction. The rest is summarized.
	PreserveRecent int `json:"preserve_recent" mapstructure:"preserve_recent"`

	// SummarizationAgent references the name of another Agent (defined in the
	// same config) whose driver/model will be used to produce the summary.
	// The referenced agent should be configured with a concise, fast model
	// suitable for summarization tasks.
	SummarizationAgent string `json:"summarization_agent" mapstructure:"summarization_agent"`

	// SummarizationPrompt is an optional template used for the summarization
	// request. If empty, a sensible default is used.
	// The template receives the conversation history as formatted text.
	SummarizationPrompt string `json:"summarization_prompt" mapstructure:"summarization_prompt"`
}

// TokenCounter is an interface for counting tokens in text.
// Implementations can use different strategies such as character-based
// heuristics, model-specific tokenizers, or API-based counting.
type TokenCounter interface {
	// CountTokens returns the estimated number of tokens in the given text.
	CountTokens(text string) int
}
