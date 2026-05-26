package agent

import (
	"fmt"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

const (
	// ConversationKeyPrefix is the prefix for Redis keys storing conversation session IDs.
	ConversationKeyPrefix = "convo:"
)

// Conversation stores a list of agent session IDs associated with a single conversation.
type Conversation struct {
	ConvoID    string   `json:"convo_id"`
	SessionIDs []string `json:"session_ids"`
}

// trackSessionInConversation is a helper to update the conversation record in the cache via RPC.
// It should be called whenever a new AgentSession is successfully persisted.
func trackSessionInConversation(store dipper.RPCCaller, convoID string, sessionID string) error {
	if convoID == "" {
		return nil
	}

	key := ConversationKeyPrefix + convoID

	// 1. Try to load the existing conversation
	var convo Conversation
	err := store.Call("cache", "get", map[string]interface{}{
		"key": key,
	}, &convo)

	// If not found, initialize a new one
	if err != nil {
		convo = Conversation{
			ConvoID:    convoID,
			SessionIDs: []string{},
		}
	}

	// 2. Check if session is already in the list to avoid duplicates
	for _, id := range convo.SessionIDs {
		if id == sessionID {
			return nil
		}
	}

	// 3. Append the new session ID
	convo.SessionIDs = append(convo.SessionIDs, sessionID)

	// 4. Save the updated conversation back to cache
	return store.Call("cache", "save", map[string]interface{}{
		"key":   key,
		"value": dipper.SerializeContent(convo),
		"ttl":   86400, // Default 24h TTL for conversation metadata
	}).(error)
}
