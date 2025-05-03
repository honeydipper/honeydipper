package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// ChatWrapper wraps around the chat client and sessions handles streaming, emitting and error handling.
type ChatWrapper struct {
	driver         *dipper.Driver
	ctx            context.Context
	cancel         context.CancelFunc
	internalCtx    context.Context
	internalCancel context.CancelFunc

	engine    string
	convID    string
	prefix    string
	counter   string
	step      string
	ttl       string
	user      string
	prompt    string
	hideThink bool

	chatter Chatter

	msgType    string
	codeBlock  bool
	inlineCode bool
	builder    strings.Builder
	lastCheck  time.Time
	done       bool
}

// NewWrapper initializes a ChatWrapper using provided factory function, a message and the driver.
func NewWrapper(d *dipper.Driver, msg *dipper.Message, factory ChatFactoryFunc) *ChatWrapper {
	msg = dipper.DeserializePayload(msg)
	w := &ChatWrapper{
		driver: d,
		user:   dipper.MustGetMapDataStr(msg.Payload, "user"),
		prompt: dipper.MustGetMapDataStr(msg.Payload, "prompt"),
	}

	w.convID = dipper.MustGetMapDataStr(msg.Payload, "convID")
	w.prefix = w.driver.Name + "/conv/" + w.convID + "/"

	// get engine
	w.engine, _ = dipper.GetMapDataStr(msg.Payload, "engine")
	if w.engine == "" {
		w.engine = "default"
	}

	// establish ttl
	w.ttl, _ = dipper.GetMapDataStr(w.driver.Options, "data.engine."+w.engine+".ttl")
	if w.ttl == "" {
		w.ttl = "10m"
	}

	// hide think
	w.hideThink, _ = dipper.GetMapDataBool(w.driver.Options, "data.engine."+w.engine+".hide_think")

	// locking
	func() {
		defer dipper.SafeExitOnError("failed to lock conversation", func(x any) {
			msg.Reply <- dipper.Message{Payload: map[string]any{"busy": true}}
			panic(x)
		})
		w.Lock()
	}()
	defer dipper.SafeExitOnError("fail to initiate conversation", func(x any) {
		w.Unlock()
		panic(x)
	})

	// determine step
	w.counter = string(dipper.Must(w.driver.Call("cache", "incr", map[string]any{"key": w.prefix + "counter"})).([]byte))
	w.step = w.prefix + w.counter

	// establish cancellation handle
	dipper.Must(w.driver.Call("cache", "save", map[string]any{"key": w.step, "value": "1"}))

	// establish context
	w.ctx, w.cancel = context.WithTimeout(context.Background(), dipper.Must(time.ParseDuration(w.ttl)).(time.Duration))
	w.internalCtx, w.internalCancel = context.WithCancel(w.ctx)

	// create the client and session
	w.chatter = factory(w)

	return w
}

// Lock obtains a lock so a conversation can have one active turn at a time.
func (w *ChatWrapper) Lock() {
	dipper.Must(w.driver.Call("locker", "lock", map[string]any{"name": w.prefix + "lock", "expire": w.ttl}))
}

// Unlock unlocks the conversation and cleans up flags to allow future turns.
func (w *ChatWrapper) Unlock() {
	w.internalCancel()
	dipper.Must(w.driver.CallNoWait("locker", "unlock", map[string]any{"name": w.prefix + "lock"}))
	dipper.Must(w.driver.CallNoWait("cache", "del", map[string]any{"key": w.step}))
}

// GetHistory returns a byte stream with JSON representation of the chat history.
func (w *ChatWrapper) GetHistory() []byte {
	resp, _ := w.driver.Call("cache", "lrange", map[string]any{"key": w.prefix + "history"})
	dipper.Logger.Debugf("history %s: %s", w.prefix, string(resp))
	if len(resp) <= 2 {
		buf := bytes.Buffer{}
		dipper.Must(buf.WriteRune('['))
		initMsg := w.chatter.InitMessages(w.engine)
		for _, msg := range initMsg {
			j := dipper.Must(json.Marshal(msg)).([]byte)
			w.AppendHistory(string(j))
			if buf.Len() > 2 {
				buf.WriteRune(',')
			}
			buf.Write(j)
		}
		buf.WriteRune(']')

		return buf.Bytes()
	}

	return resp
}

// AppendHistory appends a JSON message to the end of the chat history.
func (w *ChatWrapper) AppendHistory(msg string) {
	dipper.Must(w.driver.Call("cache", "rpush", map[string]any{"key": w.prefix + "history", "value": msg}))
}

// ChatRelay wraps around chat stream loop providing cleanup and error handling.
func (w *ChatWrapper) ChatRelay(msg *dipper.Message) {
	msg.Reply <- dipper.Message{Payload: map[string]any{"counter": w.counter, "convID": w.convID}}
	go func() {
		defer dipper.SafeExitOnError("failed to relay ai chat session")
		defer w.Unlock()

		h := w.GetHistory()
		m := w.chatter.BuildUserMessage(w.user, w.prompt)
		w.AppendHistory(string(dipper.Must(json.Marshal(m)).([]byte)))

		w.chatter.Stream(m, h, w.streamHandler, w.toolCallHandler)
	}()
}

func (w *ChatWrapper) handleThoughts(text string) (string, bool, bool) {
	if strings.Contains(text, "<think>") {
		w.msgType = "think"
	}
	typeChange := strings.Contains(text, "</think>")
	if w.hideThink && w.msgType == "think" {
		if !typeChange {
			return "", typeChange, true
		}

		w.msgType = ""
		text = strings.TrimSpace(strings.SplitN(text, "</think>", 2)[1])
		if len(text) == 0 {
			return "", typeChange, true
		}
	}

	return text, typeChange, false
}

func (w *ChatWrapper) streamHandler(t string, done bool) {
	dipper.SafeExitOnError("failed to process streamed message", func(x any) {
		w.cancel()
		panic(x)
	})

	// handling thoughts
	text, typeChange, skip := w.handleThoughts(t)
	if skip {
		return
	}

	// check code block and boundaries
	codeBlockBoundary := strings.Count(text, "```")%2 != 0
	if !w.codeBlock && codeBlockBoundary {
		// flush the content to start a new code block
		w.chatEmit(false)
	}

	if w.codeBlock && codeBlockBoundary {
		boundary := strings.LastIndex(text, "```")
		w.builder.WriteString(text[:boundary] + "```")
		text = strings.TrimSpace(text[boundary+3:])
		w.chatEmit(done && len(text) == 0)
	}

	w.codeBlock = w.codeBlock != codeBlockBoundary

	inlineBoundary := strings.Count(strings.ReplaceAll(text, "```", ""), "`")%2 != 0
	w.inlineCode = w.inlineCode != inlineBoundary

	// buffer the content
	w.builder.WriteString(text)

	switch {
	case w.codeBlock || w.inlineCode:
		// do not break a code block into multiple emits
	case done:
		defer w.internalCancel()

		fallthrough
	case typeChange:
		fallthrough
	case strings.HasSuffix(text, "\n"):
		// natural break at the end of sentences.
		fallthrough
	case w.builder.Len() > 2990:
		// break to avoid excessive long messages.
		w.chatEmit(done)
	}

	// no more thoughts
	if typeChange {
		w.msgType = ""
	}

	if !done {
		// check for user cancellation
		now := time.Now()
		if time.Now().After(w.lastCheck.Add(time.Second * 10)) {
			w.lastCheck = now
			cancelled := len(dipper.Must(w.driver.CallRaw("cache", "exists", []byte(w.step))).([]byte)) == 0
			if cancelled {
				defer w.cancel()
				dipper.Logger.Warningf("cancelling after user cancel")
				w.chatEmit(true)
			}
		}
	}
}

func (w *ChatWrapper) chatEmit(done bool) {
	if w.done {
		return
	}
	w.done = done

	content := w.builder.String()
	w.builder = strings.Builder{}

	if content != "" {
		consolidated := w.chatter.BuildMessage(content)
		w.AppendHistory(string(dipper.Must(json.Marshal(consolidated)).([]byte)))
	}
	dipper.Must(w.driver.Call("cache", "rpush", map[string]any{
		"key": w.step + "/response",
		"value": map[string]any{
			"done":    done,
			"content": content,
			"type":    w.msgType,
		},
	}))
}

// Engine returns the AI engine used for this chat wrapper.
func (w *ChatWrapper) Engine() string {
	return w.engine
}

// Contenxt returns the context used for this chat wrapper.
func (w *ChatWrapper) Context() context.Context {
	return w.ctx
}

// Cancel cancels the chat wrapper.
func (w *ChatWrapper) Cancel() {
	dipper.Logger.Warningf("cancelling from external")
	w.cancel()
}
