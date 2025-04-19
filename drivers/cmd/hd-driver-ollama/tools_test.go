package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

type testStep struct {
	description string
	sent        *dipper.Message
	recv        *dipper.Message
	fn          func(*dipper.Message, *dipper.Message)
}

func TestHandleToolCalls(t *testing.T) {
	// Create mock HTTP server
	apiCalls := sync.WaitGroup{}
	apiCalls.Add(2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/chat", r.URL.Path)

		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)
		assert.Equal(t, "default", reqBody["model"])

		lenOfHist := len(reqBody["messages"].([]interface{}))
		lastMessage := reqBody["messages"].([]any)[lenOfHist-1]
		var response map[string]interface{}
		switch lastMessage.(map[string]interface{})["role"].(string) {
		case "user":
			assert.Equal(t, "test prompt", lastMessage.(map[string]interface{})["content"])
			response = map[string]interface{}{
				"message": map[string]interface{}{
					"role": "assistant",
					"tool_calls": []any{
						map[string]any{
							"function": map[string]any{
								"name": "test",
								"arguments": map[string]any{
									"arg1": "value1",
								},
							},
						},
					},
				},
				"done": true,
			}
			apiCalls.Done()
		case "tool":
			assert.Equal(t, "{\"function\": \"test\", \"return\": {\"test\": \"foobar\"}}", lastMessage.(map[string]interface{})["content"])
			response = map[string]interface{}{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Test response",
				},
				"done": true,
			}
			apiCalls.Done()
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	time.Sleep(time.Millisecond * 100) // wait for the http server to be ready

	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	// Initialize driver with test options
	driver = dipper.NewDriver(
		"test",
		"ollama",
		dipper.DriverWithReader(inReader),
		dipper.DriverWithWriter(outWriter),
	)

	// Set up command map like in main method
	driver.Commands["chat"] = chat
	driver.Start = loadOptions

	// Start driver
	go driver.Run()

	// Send options message to initialize driver
	optionsMsg := &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"api_timeout": "10",
				"tools": map[string]interface{}{
					"test": map[string]interface{}{
						"tool": map[string]interface{}{
							"type": "function",
							"function": map[string]interface{}{
								"name":        "test",
								"description": "test",
							},
						},
					},
				},
			},
		},
	}
	dipper.SendMessage(inWriter, optionsMsg)

	// Send start message to initialize driver
	startMsg := &dipper.Message{
		Channel: "command",
		Subject: "start",
	}
	dipper.SendMessage(inWriter, startMsg)

	// Wait for driver to be ready
	<-driver.ReadySignal

	// Send chat message through the pipe to invoke the command
	chatMsg := &dipper.Message{
		Channel: "eventbus",
		Subject: "command",
		Labels: map[string]string{
			"method":    "chat",
			"sessionID": "1",
		},
		Payload: map[string]interface{}{
			"convID":      "test-conv",
			"prompt":      "test prompt",
			"ollama_host": server.URL,
		},
	}
	dipper.SendMessage(inWriter, chatMsg)

	// expected sequence of i/o messages
	// ---------------------------------
	// state.alive
	// --- chat
	// lock - lock ollama
	// lock - lock conversataion
	// incr - assign step number
	// save - set step flag
	// lrange - get history
	// rpush - push user message
	// eventbus.return - chat command return
	// --- handleToolCalls
	// rpush - save the message with tool call invokation to history
	// eventbus.message - oursource the tool call work to a workflow
	// blpop - get the workflow result
	// rpush - save the tool call return to history
	// --- chatEmit
	// rpush - push message for display
	// rpush - push message to history
	// --- chatRelay
	// exists - check if the conversation step is cancelled
	// --- chat defer
	// del - clear step flag
	// unlock - unlock conversation
	// unlock - unlock ollama

	outOfOrder := []testStep{
		{
			description: "reporting to service state.alive",
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
			description: "lock per ollama instance",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "lock",
				},
			},
			recv: &dipper.Message{ // received
				Channel: "rpc",
				Subject: "return",
			},
		},
		{
			description: "lock the conversation",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "lock",
				},
			},
			recv: &dipper.Message{ // received
				Channel: "rpc",
				Subject: "return",
			},
		},
		{
			description: "assign a step number for chat turn",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "incr",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "set a flag for the step for possible cancellation",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "save",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "fetch the previous chat history of the conversation",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "lrange",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				Labels: map[string]string{
					"error": "not found",
				},
			},
		},
		{
			description: "save the current user message to the chat history",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "save the received tool_calls from the model to chat history",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
		},
		{
			description: "launch a workflow to execute the tool_call",
			sent: &dipper.Message{
				Channel: "eventbus",
				Subject: "message",
			},
		},
		{
			description: "getting the result from the workflow",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "blpop",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				IsRaw:   true,
				Payload: []byte(`{"test": "foobar"}`),
			},
		},
		{
			description: "save the tool_call result to chat history",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
		},
		{
			description: "save the received assistant message to chat history",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "send the assistant message to chatContinue for display",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "rpush",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "check if the conversation step is cancelled by user",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "exists",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "unlock the conversation for future steps",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "unlock",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "clear the step flag",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "del",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
				Subject: "return",
				IsRaw:   true,
				Payload: []byte("1"),
			},
		},
		{
			description: "unlock the ollama instance",
			sent: &dipper.Message{
				Channel: "rpc",
				Subject: "call",
				Labels: map[string]string{
					"method": "unlock",
				},
			},
			recv: &dipper.Message{
				Channel: "rpc",
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

	// Start a goroutine to read from outReader
	go func() {
		for {
			msg := dipper.FetchRawMessage(outReader)
			if msg == nil {
				return
			}

			var step testStep
			//nolint:goconst
			if msg.Channel == "rpc" || msg.Subject == "message" {
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
			if msg.Channel == "rpc" && ret != nil {
				if ret.Labels == nil {
					ret.Labels = map[string]string{}
				}
				ret.Labels["rpcID"] = msg.Labels["rpcID"]
			}

			if ret != nil {
				dipper.SendMessage(inWriter, ret)
			}

			if msg.Channel == "rpc" || msg.Subject == "message" {
				rpcStep++
				calls.Done()
			} else {
				othStep++
				others.Done()
			}
		}
	}()

	calls.Wait()
	apiCalls.Wait()
	others.Wait()
	driver.State = dipper.DriverStateCompleted
}
