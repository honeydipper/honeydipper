// Copyright 2024 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package agent

import (
	"unicode/utf8"

	agentpkg "github.com/honeydipper/honeydipper/v4/pkg/agent"
)

// SimpleTokenCounter is a default implementation of TokenCounter that uses
// a character-based heuristic for rough token estimation.
//
// The heuristic uses the common approximation of 1 token ≈ 4 characters
// for English text. This is a rough estimate and may not be accurate for
// all languages or text types (e.g., code, mathematical expressions, or
// logographic writing systems).
//
// For more accurate token counting, consider implementing a custom
// TokenCounter using model-specific tokenizers or API-based counting.
type SimpleTokenCounter struct{}

// CountTokens returns an estimate of the number of tokens in the given text.
// It uses a simple heuristic: len(text) / 4, which assumes approximately
// 4 characters per token for typical English text.
//
// The count is always at least 1 for non-empty text to avoid zero-token
// calculations that might cause issues in downstream logic.
func (s *SimpleTokenCounter) CountTokens(text string) int {
	if text == "" {
		return 0
	}

	// Use character count (runes) for better Unicode support
	charCount := utf8.RuneCountInString(text)

	// Apply the 1 token ≈ 4 characters heuristic
	tokens := charCount / 4

	// Ensure at least 1 token for non-empty text
	if tokens == 0 {
		tokens = 1
	}

	return tokens
}

// Ensure SimpleTokenCounter implements TokenCounter interface at compile time.
var _ agentpkg.TokenCounter = (*SimpleTokenCounter)(nil)
