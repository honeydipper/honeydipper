package ai

import (
	"io"
	"sync"
	"testing"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

type streamStep struct {
	text   string
	json   string
	args   map[string]any
	name   string
	callID string
}

type mockChatter struct {
	steps           []streamStep
	t               *testing.T
	current         int
	consolidated    []string
	consolidatedPtr int
	toolReturn      []string
	toolReturnPtr   int
	history         []byte
	userMessage     any
}

func (m *mockChatter) Stream(msg any, hist []byte, msgHanlder func(string, bool), toolHandler func(string, map[string]any, string, string)) {
	assert.Equal(m.t, m.userMessage, msg, "should pass the user message to stream")
	assert.Equal(m.t, m.history, hist, "should pass the chat history to stream")
	for m.current < len(m.steps) {
		assert.NotContains(m.t, []int{1, 2}, m.current, "some stream steps should be launched from function handler")
		step := m.steps[m.current]
		m.current++
		if step.text != "" {
			msgHanlder(step.text, m.current == len(m.steps))
		} else {
			toolHandler(step.json, step.args, step.name, step.callID)

			return
		}
	}
}

func (m *mockChatter) StreamWithFunctionReturn(output any, msgHanlder func(string, bool), toolHandler func(string, map[string]any, string, string)) {
	for m.current < len(m.steps) {
		assert.Contains(m.t, []int{1, 2}, m.current, "some stream steps should be launched from function handler")
		step := m.steps[m.current]
		m.current++
		if step.text != "" {
			msgHanlder(step.text, m.current == len(m.steps))
		} else {
			toolHandler(step.json, step.args, step.name, step.callID)

			return
		}
	}
}

func (m *mockChatter) InitMessages(engine string) []any {
	assert.Equal(m.t, "test-engine", engine, "should pass the configured engine to Init Messages")

	return []any{
		map[string]any{"role": "system", "text": "hello"},
		map[string]any{"role": "assistant", "text": "hi"},
	}
}

func (m *mockChatter) BuildMessage(text string) any {
	assert.Less(m.t, m.consolidatedPtr, len(m.consolidated), "should not call build message more than supposed")
	assert.Equal(m.t, m.consolidated[m.consolidatedPtr], text, "should pass the text to build json message")
	m.consolidatedPtr++

	return map[string]any{
		"role":    "assistant",
		"message": text,
	}
}

func (m *mockChatter) BuildUserMessage(user, text string) any {
	return map[string]any{
		"role":    "user",
		"message": user + " says: :quote_start: " + text + ":quote_end:",
	}
}

func (m *mockChatter) BuildToolReturnMessage(name string, callID string, b []byte) any {
	assert.Equal(m.t, m.toolReturn[m.toolReturnPtr], string(b), "should pass the text to build tool return message")
	m.toolReturnPtr++

	return map[string]any{
		"role":    "tool",
		"message": string(b),
	}
}

type testStep struct {
	description string
	sent        *dipper.Message
	recv        *dipper.Message
	fn          func(*dipper.Message, *dipper.Message)
}

func TestChat(t *testing.T) {
	mockChatServer := &mockChatter{
		userMessage: map[string]interface{}{"role": "user", "message": "test-user says: :quote_start: test prompt:quote_end:"},
		history:     []byte("[{\"role\":\"system\",\"text\":\"hello\"},{\"role\":\"assistant\",\"text\":\"hi\"}]"),
		steps: []streamStep{
			{
				name:   "test",
				args:   map[string]any{"file": "@cache:/path/to/the/file"},
				json:   "{\"role\": \"assistant\", \"function_call\": \"test\"}",
				callID: "call-1",
			},
			{
				name:   "test2",
				args:   map[string]any{"file": "/path/to/the/file", "content": "abcde"},
				json:   "{\"role\": \"assistant\", \"function_call\": \"test2\"}",
				callID: "call-2",
			},
			{
				text: "test response",
			},
		},
		toolReturn: []string{
			"{\"test\": \"result\"}",
			"{\"test\":\"localresult\"}",
		},
		consolidated: []string{"test response"},
		t:            t,
	}

	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	// Initialize driver with test options
	driver := dipper.NewDriver(
		"test",
		"gemini",
		dipper.DriverWithReader(inReader),
		dipper.DriverWithWriter(outWriter),
	)

	var wrapper *ChatWrapper

	// Set up command map
	driver.Commands["chat"] = func(m *dipper.Message) {
		wrapper = NewWrapper(driver, m, func(w ChatWrapperInterface) Chatter {
			return mockChatServer
		})
		wrapper.ChatRelay(m)
	}
	driver.Start = func(_ *dipper.Message) {
		dipper.MustGetMapData(driver.Options, "data.tools.test").(map[string]any)["workflow"] = &config.Workflow{}
		dipper.MustGetMapData(driver.Options, "data.tools.test2").(map[string]any)["workflow"] = &config.Workflow{
			CallDriver: "cache.rpush",
			Function: config.Function{
				RawAction: "rpc",
			},
			Local: map[string]any{
				"parameters": map[string]any{
					"key":   "$args.file",
					"value": "$args.content",
				},
				"output": map[string]any{
					"test": "localresult",
				},
			},
		}
	}

	// Start driver
	go driver.Run()

	// Send options message to initialize driver
	optionsMsg := &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"ttl": "100m",
				"tools": map[string]interface{}{
					"test": map[string]any{
						"tool": map[string]any{
							"functionDeclarations": []any{
								map[string]interface{}{
									"name":        "test",
									"description": "test",
									"parameters": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"arg": map[string]interface{}{
												"type":        "string",
												"description": "test",
											},
										},
									},
								},
							},
						},
					},
					"test2": map[string]any{
						"tool": map[string]any{
							"functionDeclarations": []any{
								map[string]interface{}{
									"name":        "test",
									"description": "test",
									"parameters": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"arg": map[string]interface{}{
												"type":        "string",
												"description": "test",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	dipper.SendMessage(inWriter, optionsMsg)

	// Send start message
	startMsg := &dipper.Message{
		Channel: "command",
		Subject: "start",
	}
	dipper.SendMessage(inWriter, startMsg)

	// Wait for driver to be ready
	<-driver.ReadySignal

	// Send chat message
	chatMsg := &dipper.Message{
		Channel: "eventbus",
		Subject: "command",
		Labels: map[string]string{
			"method":    "chat",
			"sessionID": "1",
			"timeout":   "1000",
		},
		Payload: map[string]interface{}{
			"convID": "test-conv",
			"engine": "test-engine",
			"prompt": "test prompt",
			"user":   "test-user",
		},
	}
	dipper.SendMessage(inWriter, chatMsg)

	// Define expected message sequence
	outOfOrder := []testStep{
		{
			description: "reporting service state alive",
			sent: &dipper.Message{
				Channel: "state",
				Subject: "alive",
			},
		},
		{
			description: "command return through eventbus",
			sent: &dipper.Message{
				Channel: "eventbus",
				Subject: "return",
				Payload: map[string]interface{}{
					"convID":   "test-conv",
					"counter":  "1",
					"response": "Test response",
				},
			},
		},
	}

	rpcs := []testStep{
		{
			description: "lock conversation",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "lock",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
			},
		},
		{
			description: "assign step number",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "incr",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "set step flag",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "save",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "fetch chat history",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "lrange",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				Labels: map[string]string{
					"error": "not found",
				},
			},
		},
		{
			description: "save system message 1 to history",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
			fn: func(sent, recv *dipper.Message) {
				assert.Contains(t, string(sent.Payload.([]byte)), "hello", "should persist the first system message")
			},
		},
		{
			description: "save system message 2 to history",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
			fn: func(sent, recv *dipper.Message) {
				assert.Contains(t, string(sent.Payload.([]byte)), "hi", "should persist the first system message")
			},
		},
		{
			description: "save user message to history",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
			fn: func(sent, recv *dipper.Message) {
				assert.Contains(t, string(sent.Payload.([]byte)), "quote_end", "should persist the first system message")
			},
		},
		{
			description: "save tool calls to history",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "fetch file from cache",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "lrange",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("abcdefg"),
			},
			fn: func(sent, recv *dipper.Message) {
				assert.Equal(t, "{\"del\":false,\"key\":\"gemini/conv/test-conv///path/to/the/file\",\"raw\":false}", string(sent.Payload.([]byte)), "should fetch from the cache")
			},
		},
		{
			description: "launch workflow for tool call",
			sent: &dipper.Message{
				Channel: "eventbus",
				Subject: "message",
			},
			fn: func(sent, recv *dipper.Message) {
				assert.Contains(t, string(sent.Payload.([]byte)), "{\"file\":\"abcdefg\"}", "should replace the content with cache content")
			},
		},
		{
			description: "get workflow result",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "blpop",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte(`{"test": "result"}`),
			},
		},
		{
			description: "save tool result to history",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		// {
		// 	description: "check step cancellation",
		// 	sent: &dipper.Message{
		// 		Channel: channelRPC,
		// 		Subject: "call",
		// 		Labels: map[string]string{
		// 			"method": "exists",
		// 		},
		// 	},
		// 	recv: &dipper.Message{
		// 		Channel: channelRPC,
		// 		Subject: "return",
		// 		IsRaw:   true,
		// 		Payload: []byte("1"),
		// 	},
		// },
		{
			description: "save tool calls to history",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "calling rpc based on AI function",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "save tool result to history",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "save assistant response to history",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "emit assistant response to display",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "unlock conversation",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "unlock",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "clear step flag",
			sent: &dipper.Message{
				Channel: channelRPC,
				Subject: "call",
				Labels: map[string]string{
					"method": "del",
				},
			},
			recv: &dipper.Message{
				Channel: channelRPC,
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
	}

	rpcStep := 0
	othStep := 0
	calls := sync.WaitGroup{}
	calls.Add(len(rpcs))
	others := sync.WaitGroup{}
	others.Add(len(outOfOrder))

	// Start goroutine to read messages
	go func() {
		for {
			msg := dipper.FetchRawMessage(outReader)
			if msg == nil {
				return
			}

			var step testStep
			if msg.Channel == channelRPC || msg.Subject == "message" {
				step = rpcs[rpcStep]
			} else {
				step = outOfOrder[othStep]
			}

			assert.Equal(t, step.sent.Channel, msg.Channel, step.description)
			assert.Equal(t, step.sent.Subject, msg.Subject, step.description)
			for k, v := range step.sent.Labels {
				assert.Equal(t, v, msg.Labels[k], step.description)
			}

			ret := step.recv
			if step.fn != nil {
				step.fn(msg, ret)
			}
			if msg.Channel == channelRPC && ret != nil {
				if ret.Labels == nil {
					ret.Labels = map[string]string{}
				}
				ret.Labels["rpcID"] = msg.Labels["rpcID"]
			}

			if ret != nil {
				dipper.SendMessage(inWriter, ret)
			}

			if msg.Channel == channelRPC || msg.Subject == "message" {
				rpcStep++
				calls.Done()
			} else {
				othStep++
				others.Done()
			}
		}
	}()

	calls.Wait()
	others.Wait()
	driver.State = dipper.DriverStateCompleted
}
