package agent

import (
	"testing"
)

func TestSimpleTokenCounter_CountTokens(t *testing.T) {
	counter := &SimpleTokenCounter{}

	testCases := []struct {
		name     string
		text     string
		expected int
	}{
		{"empty string", "", 0},
		{"single character", "a", 1},                         // 1 char / 4 = 0, but min is 1
		{"short word", "hello", 1},                           // 5 chars / 4 = 1
		{"4 characters", "test", 1},                          // 4 chars / 4 = 1
		{"5 characters", "hello!", 1},                        // 6 chars / 4 = 1
		{"8 characters", "testtest", 2},                      // 8 chars / 4 = 2
		{"typical sentence", "The quick brown fox", 4},       // 19 chars / 4 = 4
		{"unicode characters", "你好世界", 1},                    // 4 chars / 4 = 1
		{"longer text", "This is a longer piece of text", 7}, // 30 chars / 4 = 7
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := counter.CountTokens(tc.text)
			if result != tc.expected {
				t.Errorf("CountTokens(%q) = %d, expected %d", tc.text, result, tc.expected)
			}
		})
	}
}

func TestSimpleTokenCounter_ImplementsInterface(t *testing.T) {
	counter := &SimpleTokenCounter{}

	// Test that SimpleTokenCounter implements TokenCounter interface
	var iface interface{} = counter
	if _, ok := iface.(interface{ CountTokens(string) int }); !ok {
		t.Error("SimpleTokenCounter does not implement TokenCounter interface")
	}
}
