package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/ollama/ollama/api"
)

const typeThink = "think"

type chatWrapper struct {
	client *api.Client
	chat   *api.ChatRequest
	ctx    context.Context
	cancel context.CancelFunc
	prefix string
	step   string

	msgType   string
	codeBlock bool
	builder   strings.Builder
	relayDone bool
}

func (w *chatWrapper) streamReceiver(response api.ChatResponse) error {
	defer dipper.SafeExitOnError("[ollama] failed to handle chat response message from ai")
	var ret error

	if handleToolCalls(w, &response) {
		return nil
	}

	dipper.Logger.Debugf("[%s] AI response chunk %s", driver.Service, response.Message.Content)
	if strings.Contains(response.Message.Content, "<think>") {
		w.msgType = typeThink
	}
	typeChange := strings.Contains(response.Message.Content, "</think>")

	codeBlockBoundary := strings.Contains(response.Message.Content, "```")
	w.codeBlock = w.codeBlock != (codeBlockBoundary && strings.Count(response.Message.Content, "```")%2 != 0)

	if w.codeBlock && codeBlockBoundary {
		// flush the content to start a new code block
		ret = w.chatEmit(response.Done)
	}

	w.builder.WriteString(response.Message.Content)

	switch {
	case w.codeBlock:
		// skip
	case response.Done:
		fallthrough
	case typeChange:
		fallthrough
	case codeBlockBoundary:
		fallthrough
	case strings.HasSuffix(response.Message.Content, "\n"):
		fallthrough
	case w.builder.Len() > 2000:
		ret = w.chatEmit(response.Done)
	}

	if typeChange {
		w.msgType = ""
	}

	cancelled := len(dipper.Must(driver.CallRaw("cache", "exists", []byte(w.step))).([]byte)) == 0
	if cancelled || response.Done {
		w.cancel()
		if cancelled {
			ret = ErrCancelled
		}
	}

	return ret
}

func (w *chatWrapper) chatRelay() {
	defer dipper.SafeExitOnError("[ollama] failed to send chat message to ai")

	attachTools(w.chat)

	dipper.Logger.Debugf("sending to AI %+v", w.chat.Messages)
	dipper.Must(w.client.Chat(w.ctx, w.chat, w.streamReceiver))
}

func (w *chatWrapper) chatEmit(done bool) error {
	if w.relayDone {
		return nil
	}
	w.relayDone = done
	var ret error

	content := w.builder.String()
	w.builder = strings.Builder{}

	consolidated := api.Message{
		Role:    "assistant",
		Content: content,
	}
	if w.msgType != typeThink {
		_, err := driver.Call("cache", "rpush", map[string]any{
			"key":   w.prefix + "history",
			"value": string(dipper.Must(json.Marshal(consolidated)).([]byte)),
		})
		if err != nil {
			ret = fmt.Errorf("failed to record the AI response: %w", err)
		}
	}
	_, err := driver.Call("cache", "rpush", map[string]any{
		"key": w.step + "/response",
		"value": map[string]any{
			"done":    done,
			"content": content,
			"type":    w.msgType,
		},
	})
	if err != nil {
		ret = fmt.Errorf("failed to relay the AI response: %w", err)
	}

	return ret
}

func (w *chatWrapper) run(step string) (context.Context, context.CancelFunc) {
	w.step = step
	w.ctx, w.cancel = context.WithCancel(context.Background())
	go w.chatRelay()

	return w.ctx, w.cancel
}

func newWrapper(msg *dipper.Message, engine string, prefix string, ollamaHost string) *chatWrapper {
	// constructing chat history
	var messages []api.Message
	resp, err := driver.Call("cache", "lrange", map[string]any{"key": prefix + "history"})
	if err == nil {
		dipper.Must(json.Unmarshal(resp, &messages))
	}
	if len(messages) == 0 {
		systemPrompt, _ := driver.GetOptionStr(fmt.Sprintf("data.engine.%s.system_prompt", engine))

		if len(systemPrompt) > 0 {
			systemMessage := api.Message{Role: "system", Content: systemPrompt}
			dipper.Must(driver.Call("cache", "rpush", map[string]any{
				"key":   prefix + "history",
				"value": string(dipper.Must(json.Marshal(systemMessage)).([]byte)),
			}))
			messages = append(messages, systemMessage)
		}
	}

	// building current user message
	userMessage := api.Message{
		Role:    "user",
		Content: dipper.MustGetMapDataStr(msg.Payload, "prompt"),
	}
	jsonUserMessage := string(dipper.Must(json.Marshal(userMessage)).([]byte))
	messages = append(messages, userMessage)
	dipper.Must(driver.Call("cache", "rpush", map[string]any{"key": prefix + "history", "value": jsonUserMessage}))

	// setting client and chat request
	var client *api.Client
	if len(ollamaHost) > 0 {
		client = api.NewClient(dipper.Must(url.ParseRequestURI(ollamaHost)).(*url.URL), http.DefaultClient)
	} else {
		client = dipper.Must(api.ClientFromEnvironment()).(*api.Client)
	}
	model := engine
	if n, ok := driver.GetOption(fmt.Sprintf("data.engine.%s.model", engine)); ok {
		model = n.(string)
	}
	chatReq := &api.ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   &chatStream,
	}
	if t, hasTemp := dipper.GetMapData(msg.Payload, "temperature"); hasTemp {
		chatReq.Options = map[string]any{
			"temperature": t.(float64),
		}
	}

	return &chatWrapper{
		client: client,
		chat:   chatReq,
		prefix: prefix,
	}
}
