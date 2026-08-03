package agent

import (
	"reflect"
	"testing"

	"github.com/honeydipper/honeydipper/v4/pkg/agent"
)

// TestFilterHistoryForModel tests the filterHistoryForModel helper function.
func TestFilterHistoryForModel(t *testing.T) {
	tests := []struct {
		name     string
		history  []agent.Message
		expected []agent.Message
	}{
		{
			name:     "empty history returns empty",
			history:  []agent.Message{},
			expected: []agent.Message{},
		},
		{
			name: "empty assistant message at end is filtered",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: false},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
			},
		},
		{
			name: "thinking-only assistant message at end is filtered",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsThinking: true, Thoughts: "Let me think..."},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
			},
		},
		{
			name: "complete assistant message with content is preserved",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "Hi there!", IsComplete: true},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "Hi there!", IsComplete: true},
			},
		},
		{
			name: "complete assistant message with tool calls is preserved",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: true, ToolCalls: []agent.ToolCall{{FuncName: "test_func"}}},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: true, ToolCalls: []agent.ToolCall{{FuncName: "test_func"}}},
			},
		},
		{
			name: "user messages are always preserved",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: false},
				{Role: RoleUser, Content: "Are you there?"},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: false},
				{Role: RoleUser, Content: "Are you there?"},
			},
		},
		{
			name: "tool result messages are always preserved",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: false},
				{Role: RoleToolResult, ToolResult: []map[string]interface{}{{"status": "success"}}},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: false},
				{Role: RoleToolResult, ToolResult: []map[string]interface{}{{"status": "success"}}},
			},
		},
		{
			name: "system prompt is always preserved when prepended",
			history: []agent.Message{
				{Role: RoleSystem, Content: "System prompt"},
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: false},
			},
			expected: []agent.Message{
				{Role: RoleSystem, Content: "System prompt"},
				{Role: RoleUser, Content: "Hello"},
			},
		},
		{
			name: "multiple trailing incomplete agent messages all filtered",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "Thinking...", IsThinking: true},
				{Role: RoleAgent, Content: "", IsComplete: false},
				{Role: RoleAgent, Content: "", IsComplete: false},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
			},
		},
		{
			name: "incomplete agent message followed by complete one preserves both",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "Thinking...", IsThinking: true},
				{Role: RoleAgent, Content: "Final answer", IsComplete: true},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "Thinking...", IsThinking: true},
				{Role: RoleAgent, Content: "Final answer", IsComplete: true},
			},
		},
		{
			name: "agent message with tool calls but not complete is filtered",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: false, ToolCalls: []agent.ToolCall{{FuncName: "test_func"}}},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
			},
		},
		{
			name: "agent message with content but not complete is filtered",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "Partial response", IsComplete: false},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
			},
		},
		{
			name: "all agent messages filtered when all are incomplete",
			history: []agent.Message{
				{Role: RoleAgent, Content: "", IsComplete: false},
				{Role: RoleAgent, Content: "Thinking...", IsThinking: true},
			},
			expected: []agent.Message{},
		},
		{
			name: "returns copy not original slice",
			history: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAgent, Content: "", IsComplete: false},
			},
			expected: []agent.Message{
				{Role: RoleUser, Content: "Hello"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterHistoryForModel(tt.history)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("filterHistoryForModel(%v) = %v, want %v", tt.history, result, tt.expected)
			}

			// Verify it returns a copy, not the original slice
			if len(tt.history) > 0 && len(result) > 0 {
				if &result[0] == &tt.history[0] {
					t.Errorf("filterHistoryForModel returned original slice, not a copy")
				}
			}
		})
	}
}

// TestFilterHistoryForModel_SendToDriverIntegration tests that sendToDriver uses filtered history.
func TestFilterHistoryForModel_SendToDriverIntegration(t *testing.T) {
	// This test verifies the integration by checking that the history
	// passed to the driver would be filtered. We can't easily test the
	// actual driver call without a full mock setup, but we can verify
	// the filter function works correctly in context.

	// Setup a session-like history
	history := []agent.Message{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAgent, Content: "", IsComplete: false, IsThinking: true},
	}

	// Filter as sendToDriver would
	filtered := filterHistoryForModel(history)

	// The filtered history should not have the incomplete agent message
	if len(filtered) != 1 {
		t.Errorf("expected filtered history length 1, got %d", len(filtered))
	}
	if filtered[0].Role != RoleUser {
		t.Errorf("expected first message role %s, got %s", RoleUser, filtered[0].Role)
	}
	if filtered[0].Content != "Hello" {
		t.Errorf("expected first message content 'Hello', got %s", filtered[0].Content)
	}
}
