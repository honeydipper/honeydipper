package ai

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
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

func TestChatContinue(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	driver := dipper.NewDriver(
		"test",
		"test-ai",
		dipper.DriverWithReader(inReader),
		dipper.DriverWithWriter(outWriter),
	)

	driver.Commands["chatContinue"] = func(msg *dipper.Message) { ChatContinue(driver, msg) }

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

	driver := dipper.NewDriver(
		"test",
		"test-ai",
		dipper.DriverWithReader(inReader),
		dipper.DriverWithWriter(outWriter),
	)

	driver.Commands["chatStop"] = func(msg *dipper.Message) { ChatStop(driver, msg) }

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
						assert.Equal(t, "{\"key\":\"test-ai/conv/test-conv/1\"}", string(msg.Payload.([]byte)))
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

func testBuider(user, prompt string) any {
	return map[string]any{
		"user":   user,
		"prompt": prompt,
	}
}

func TestChatListen(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	driver := dipper.NewDriver(
		"test",
		"test-ai",
		dipper.DriverWithReader(inReader),
		dipper.DriverWithWriter(outWriter),
	)

	driver.Commands["chatListen"] = func(msg *dipper.Message) { ChatListen(driver, msg, testBuider) }

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
						assert.Equal(t, "test-ai/conv/test-conv/history", string(msg.Payload.([]byte)))
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
							"{\"key\":\"test-ai/conv/test-conv/history\",\"value\":\"{\\\"prompt\\\":\\\"test prompt\\\",\\\"user\\\":\\\"test-user\\\"}\"}",
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
