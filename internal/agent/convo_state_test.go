package agent

import "testing"

func TestUpdateSessionStatusDoesNotDoubleCount(t *testing.T) {
	cs := &ConvoState{}
	// Set up First and Last to point to the same session ID to simulate the
	// common case where both refs reference the same session object.
	cs.FirstSession = &ConvoSessionRef{SessionID: "s1"}
	cs.LastSession = &ConvoSessionRef{SessionID: "s1"}

	// First update: session completes with totalTokens 30
	cs.updateSessionStatus("s1", ConvoSessionStatusComplete, 10, 20, 30)
	if cs.TotalTokens != 30 {
		t.Fatalf("expected convo TotalTokens 30, got %d", cs.TotalTokens)
	}

	// Repeated update with same totalTokens should not increase convo total
	cs.updateSessionStatus("s1", ConvoSessionStatusComplete, 10, 20, 30)
	if cs.TotalTokens != 30 {
		t.Fatalf("expected convo TotalTokens to remain 30 after repeated update, got %d", cs.TotalTokens)
	}
}
