package ai

import (
	"context"
	"errors"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// ChatWrapperInterface defines the behavior of a chat wrapper.
type ChatWrapperInterface interface {
	// Core functionality.
	Lock()                         // Obtains a lock so a conversation can have one active turn at a time.
	Unlock()                       // Unlocks the conversation and cleans up flags to allow future turns.
	GetHistory() []byte            // Returns a byte stream with JSON representation of the chat history.
	AppendHistory(msg string)      // Appends a JSON message to the end of the chat history.
	ChatRelay(msg *dipper.Message) // Wraps around chat stream loop providing cleanup and error handling.

	// Getters.
	Engine() string           // Returns the AI engine used for this chat wrapper.
	Context() context.Context // Returns the context used for this chat wrapper.

	// Control.
	Cancel() // Cancels the chat wrapper.
}

// Chatter is implemented by AI drivers to work with wrapper to communicate with AI backends.
type Chatter interface {
	// Stream kicks off a loop to handle AI backend responses. The AI driver should catch and handle
	// any errors the handlers may throw.
	Stream(
		msg any,
		hist []byte,
		streamHandler func(string, bool),
		toolCallHandler func(string, map[string]any, string, string),
	)

	// StreamWithFunctionReturn kicks off a loop to handle AI backend responses after providing
	// function return. The AI driver should catch and handle any errors the handlers may throw.
	StreamWithFunctionReturn(
		ret any,
		streamHandler func(text string, done bool),
		toolCallHandler func(jsonMessage string, args map[string]any, name string, callID string),
	)

	// InitMessages injects a few messages in JSON format at the top of the chat history.
	InitMessages(engine string) []any

	// BuildMessage creates an assistant message based on the text and returns the JSON string.
	BuildMessage(text string) any

	// BuildUserMessage creates a user message based on the text and returns the API native message.
	BuildUserMessage(user, text string) any

	// BuildToolReturnMessage creates a message based on the tool call result and returns a API native message.
	BuildToolReturnMessage(name string, callID string, b []byte) any
}

// ChatFactoryFunc defines a function type that creates a Chatter instance.
type ChatFactoryFunc func(w ChatWrapperInterface) Chatter

// ErrCancelled means the AI chat turn has been cancelled.
var ErrCancelled = errors.New("AI chat turn cancelled")
