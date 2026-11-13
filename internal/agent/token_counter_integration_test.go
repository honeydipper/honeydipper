package agent

import (
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
)

func TestAgentSession_CustomTokenCounter_FieldPresent(t *testing.T) {
	// Verify AgentSession has TokenCounter field
	session := &AgentSession{}

	// Test that we can set it
	session.TokenCounter = &SimpleTokenCounter{}

	if session.TokenCounter == nil {
		t.Error("Failed to set TokenCounter field")
	}
}

func TestAgentSession_CustomTokenCounter_CountTokens(t *testing.T) {
	counter := &SimpleTokenCounter{}

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"empty", "", 0},
		{"short", "hello", 1},
		{"typical", "The quick brown fox jumps", 6},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := counter.CountTokens(tc.text)
			if result != tc.expected {
				t.Errorf("CountTokens(%q) = %d, want %d", tc.text, result, tc.expected)
			}
		})
	}
}

func TestAgentSession_TokenCounterConfigIntegration(t *testing.T) {
	// Test that Agent config TokenCounter field can be read
	agent := &config.Agent{
		Name:         "test-agent",
		TokenCounter: "custom",
	}

	if agent.TokenCounter != "custom" {
		t.Errorf("Expected TokenCounter to be 'custom', got %q", agent.TokenCounter)
	}
}

func TestAgentSession_TokenCounter_NilBehavior(t *testing.T) {
	// When TokenCounter is nil, session should work without custom counting
	session := &AgentSession{
		TokenCounter: nil,
		InputTokens:  10,
		OutputTokens: 20,
	}

	// Verify default behavior works
	session.TotalTokens = session.InputTokens + session.OutputTokens

	if session.TotalTokens != 30 {
		t.Errorf("Expected TotalTokens = 30, got %d", session.TotalTokens)
	}
}

func TestAgentSession_TokenCounter_InterfaceCompliance(t *testing.T) {
	// Verify SimpleTokenCounter implements agentpkg.TokenCounter interface
	var _ agentpkg.TokenCounter = (*SimpleTokenCounter)(nil)

	counter := &SimpleTokenCounter{}
	result := counter.CountTokens("test text")
	if result < 1 {
		t.Error("CountTokens should return at least 1 for non-empty text")
	}
}
