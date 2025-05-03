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

const (
	channelRPC      = "rpc"
	channelEventbus = "eventbus"
	subjectCall     = "call"
	subjectReturn   = "return"
	methodRPush     = "rpush"
	methodExists    = "exists"
	methodDel       = "del"
)

func TestChat(t *testing.T) {
	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/chat", r.URL.Path)

		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)
		assert.Equal(t, "default", reqBody["model"])

		w.Header().Set("Content-Type", "application/x-ndjson")
		response := map[string]interface{}{
			"message": map[string]interface{}{
				"role":    "assistant",
				"content": "Test response",
			},
			"done": true,
		}
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

	// Send chat message through the pipe
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

	// Channel to signal test completion
	done := make(chan bool)

	calls := sync.WaitGroup{}
	calls.Add(12)
	// --- chat
	// lock - lock ollama
	// lock - lock conversataion
	// incr - step
	// save - step flag
	// lrange - get history
	// rpush - push user message
	// --- chatRelay
	// exists - check if cancelled
	// --- chatEmit
	// rpush - push message for display
	// rpush - push message to history
	// --- chat defer
	// del - step flag
	// unlock - unlock conversation
	// unlock - unlock ollama

	// Start a goroutine to read from outReader
	go func() {
		for {
			msg := dipper.FetchRawMessage(outReader)
			if msg == nil {
				return
			}

			switch msg.Channel {
			case channelRPC:
				switch msg.Subject {
				case subjectCall:
					// Simulate RPC responses
					switch msg.Labels["method"] {
					case "lock":
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
							},
						})
						calls.Done()
					case "incr":
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
							},
							Payload: []byte("1"),
							IsRaw:   true,
						})
						calls.Done()
					case "save":
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
							},
							Payload: []byte("1"),
						})
						calls.Done()
					case methodRPush:
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
							},
							Payload: []byte("1"),
						})
						calls.Done()
					case "lrange":
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
								"error": "not found",
							},
						})
						calls.Done()
					case methodExists:
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
							},
							Payload: []byte("1"),
						})
						calls.Done()
					case "unlock":
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
							},
							Payload: []byte("1"),
						})
						calls.Done()
					case methodDel:
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
							},
							Payload: []byte("1"),
						})
						calls.Done()
					}
				}
			case channelEventbus:
				if msg.Subject == subjectReturn {
					payload := dipper.DeserializeContent(msg.Payload.([]byte)).(map[string]interface{})
					assert.Equal(t, "test-conv", payload["convID"])
					assert.Equal(t, "1", payload["counter"])
					close(done)
				}
			}
		}
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// Test completed successfully
	case <-time.After(10 * time.Second): // for testing
		t.Fatal("timeout waiting for responses")
	}

	calls.Wait()
	driver.State = dipper.DriverStateCompleted
}

func TestChatContinue(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	driver = dipper.NewDriver(
		"test",
		"ollama",
		dipper.DriverWithReader(inReader),
		dipper.DriverWithWriter(outWriter),
	)

	driver.Commands["chatContinue"] = chatContinue
	driver.Start = loadOptions

	go driver.Run()

	optionsMsg := &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"api_timeout": "10",
			},
		},
	}
	dipper.SendMessage(inWriter, optionsMsg)

	startMsg := &dipper.Message{
		Channel: "command",
		Subject: "start",
	}
	dipper.SendMessage(inWriter, startMsg)

	<-driver.ReadySignal

	chatMsg := &dipper.Message{
		Channel: channelEventbus,
		Subject: "command",
		Labels: map[string]string{
			"method":    "chatContinue",
			"sessionID": "2",
		},
		Payload: map[string]interface{}{
			"convID":  "test-conv",
			"counter": "1",
		},
	}
	dipper.SendMessage(inWriter, chatMsg)

	done := make(chan bool)

	calls := sync.WaitGroup{}
	calls.Add(1)

	go func() {
		for {
			msg := dipper.FetchRawMessage(outReader)
			if msg == nil {
				return
			}

			switch msg.Channel {
			case channelRPC:
				switch msg.Subject {
				case subjectCall:
					switch msg.Labels["method"] {
					case "blpop":
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
							},
							Payload: map[string]any{
								"done": true,
								"content": map[string]any{
									"role":    "assistant",
									"content": "Test response",
								},
								"type": "",
							},
						})
						calls.Done()
					}
				}
			case channelEventbus:
				if msg.Subject == subjectReturn {
					payload := string(msg.Payload.([]byte))
					assert.Equal(t, "{\"content\":{\"content\":\"Test response\",\"role\":\"assistant\"},\"done\":true,\"type\":\"\"}", payload)
					close(done)
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for responses")
	}

	calls.Wait()
	driver.State = dipper.DriverStateCompleted
}

func TestChatStop(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	driver = dipper.NewDriver(
		"test",
		"ollama",
		dipper.DriverWithReader(inReader),
		dipper.DriverWithWriter(outWriter),
	)

	driver.Commands["chatStop"] = chatStop
	driver.Start = loadOptions

	go driver.Run()

	optionsMsg := &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"api_timeout": "10",
			},
		},
	}
	dipper.SendMessage(inWriter, optionsMsg)

	startMsg := &dipper.Message{
		Channel: "command",
		Subject: "start",
	}
	dipper.SendMessage(inWriter, startMsg)

	<-driver.ReadySignal

	chatMsg := &dipper.Message{
		Channel: channelEventbus,
		Subject: "command",
		Labels: map[string]string{
			"method":    "chatStop",
			"sessionID": "3",
		},
		Payload: map[string]interface{}{
			"convID":  "test-conv",
			"counter": "1",
		},
	}
	dipper.SendMessage(inWriter, chatMsg)

	done := make(chan bool)

	calls := sync.WaitGroup{}
	calls.Add(1)

	go func() {
		for {
			msg := dipper.FetchRawMessage(outReader)
			if msg == nil {
				return
			}

			switch msg.Channel {
			case channelRPC:
				switch msg.Subject {
				case subjectCall:
					switch msg.Labels["method"] {
					case methodDel:
						assert.Equal(t, "{\"key\":\"ollama/conv/test-conv/1\"}", string(msg.Payload.([]byte)))
						calls.Done()
					}
				}
			case channelEventbus:
				if msg.Subject == subjectReturn {
					assert.Nil(t, msg.Payload)
					close(done)
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for responses")
	}

	calls.Wait()
	driver.State = dipper.DriverStateCompleted
}

func TestChatListen(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	driver = dipper.NewDriver(
		"test",
		"ollama",
		dipper.DriverWithReader(inReader),
		dipper.DriverWithWriter(outWriter),
	)

	driver.Commands["chatListen"] = chatListen
	driver.Start = loadOptions

	go driver.Run()

	optionsMsg := &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"api_timeout": "10",
			},
		},
	}
	dipper.SendMessage(inWriter, optionsMsg)

	startMsg := &dipper.Message{
		Channel: "command",
		Subject: "start",
	}
	dipper.SendMessage(inWriter, startMsg)

	<-driver.ReadySignal

	chatMsg := &dipper.Message{
		Channel: channelEventbus,
		Subject: "command",
		Labels: map[string]string{
			"method":    "chatListen",
			"sessionID": "4",
		},
		Payload: map[string]interface{}{
			"convID": "test-conv",
			"user":   "test-user",
			"prompt": "test prompt",
		},
	}
	dipper.SendMessage(inWriter, chatMsg)

	done := make(chan bool)

	calls := sync.WaitGroup{}
	calls.Add(2)

	go func() {
		for {
			msg := dipper.FetchRawMessage(outReader)
			if msg == nil {
				return
			}

			switch msg.Channel {
			case channelRPC:
				switch msg.Subject {
				case subjectCall:
					switch msg.Labels["method"] {
					case methodExists:
						assert.Equal(t, "ollama/conv/test-conv/history", string(msg.Payload.([]byte)))
						dipper.SendMessage(inWriter, &dipper.Message{
							Channel: channelRPC,
							Subject: subjectReturn,
							Labels: map[string]string{
								"rpcID": msg.Labels["rpcID"],
							},
							Payload: []byte("1"),
							IsRaw:   true,
						})
						calls.Done()
					case methodRPush:
						assert.Equal(t,
							"{\"key\":\"ollama/conv/test-conv/history\",\"value\":\"{\\\"role\\\":\\\"user\\\",\\\"content\\\":\\\"test-user says :start quote: test prompt\\\\n\\\\n :end quote:\\\"}\"}",
							string(msg.Payload.([]byte)))
						calls.Done()
					}
				}
			case channelEventbus:
				if msg.Subject == subjectReturn {
					assert.Nil(t, msg.Payload)
					close(done)
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for responses")
	}

	calls.Wait()
	driver.State = dipper.DriverStateCompleted
}
