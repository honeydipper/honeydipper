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

	// IsSynthetic marks a user-role turn that the daemon fabricated to resume an
	// interrupted agent turn (see ContinuePolicy). It is the OPPOSITE of IsSlash:
	// it is write-only in this phase. A synthesized turn is appended to the
	// driver request only and is NEVER persisted to s.history /
	// convo_history:<id>, NEVER counted in ContextTokens, and NEVER subject to
	// max_history_len trimming. No daemon code reads this field back; it exists so
	// tests and logs can assert provenance now, and so drivers can act on it in a
	// later phase.
	IsSynthetic bool `json:"is_synthetic,omitempty" mapstructure:"is_synthetic"`
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
// to the model when the threshold is exceeded, and fires at most once per real
// user turn (a once-per-turn guard applies to both threshold types; it is a
// no-op for history_len in normal operation but prevents total_tokens from
// looping within a single turn).
//
// Behavior note: because the total_tokens baseline self-heals to the actual
// compacted context size (rather than resetting to zero), setting Threshold
// below roughly (summary + preserve-window) tokens will re-trigger compaction
// on every subsequent user turn. Set Threshold comfortably above the expected
// post-compaction baseline to avoid re-firing each turn.
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
	//   - "total_tokens":  the context size of the latest model call
	//     (InputTokens + OutputTokens of the most recent complete agent message
	//     in the conversation history), compared against Threshold. In the
	//     default mode (TokenCounter == nil) these are driver-reported token
	//     counts — the exact unit the user observes, populated by
	//     hd-driver-openai from msg.Usage.PromptTokens / msg.Usage.CompletionTokens.
	//     Under token_counter: "simple" the baseline is instead heuristic:
	//     appendConvoHistory overwrites each message's InputTokens/OutputTokens
	//     with simple character-based counts, so the baseline reflects those
	//     heuristic values rather than driver-reported usage. This comparison
	//     does not mutate the conversation's ConvoState.total_tokens accounting
	//     field, which is left unchanged.
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

// DefaultContinuePrompt is the built-in resume marker the daemon synthesizes
// when an interrupted agent turn needs a trailing user message. It is worded to
// be unambiguous to the model (an automatic continuation, NOT new user input)
// and is byte-stable across turns so it can be recognized and deduplicated.
const DefaultContinuePrompt = "Continue from where you left off — automatic resume of an interrupted turn; " +
	"this is not new input from the user."

// ContinuePolicy configures how the engine resumes a conversation whose
// serialized history does not end on a user-role message. It follows the same
// config shape and validation style as CompactionPolicy.
//
// The engine synthesizes a resume marker ONLY when the trailing real message is
// an interrupted agent turn (a RoleAgent message with no pending ToolCalls).
// Tool round-trips (trailing RoleToolResult) and every other role are sent
// as-is because all supported driver protocols terminate correctly there.
type ContinuePolicy struct {
	// Mode selects whether the engine may synthesize a resume marker.
	//   - "" / "auto" (default): synthesize only for an interrupted agent turn.
	//   - "never": RESERVED. Not honored in this phase; the daemon emits an
	//     assembly-time warning and degrades to "auto" at runtime. It is reserved
	//     for drivers that advertise their own require_trailing_user handling.
	Mode string `json:"mode" mapstructure:"mode"`

	// Prompt overrides the built-in resume marker text (DefaultContinuePrompt).
	//
	// It is intentionally NOT interpolated. Interpolating session data into a
	// user-authority turn is the anti-goal, and the marker must stay byte-stable
	// across turns so it stays recognizable. An empty or whitespace-only value
	// falls back to DefaultContinuePrompt; an empty prompt is never transmitted
	// (Anthropic and Gemini reject empty content with a 400, OpenAI treats it as
	// inert).
	Prompt string `json:"prompt" mapstructure:"prompt"`
}

// TokenCounter is an interface for counting tokens in text.
// Implementations can use different strategies such as character-based
// heuristics, model-specific tokenizers, or API-based counting.
type TokenCounter interface {
	// CountTokens returns the estimated number of tokens in the given text.
	CountTokens(text string) int
}
