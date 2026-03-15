package ai

import (
	"encoding/json"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

func ChatContinue(d *dipper.Driver, msg *dipper.Message) {
	msg.Reply <- dipper.Message{Labels: map[string]string{"no-timeout": "true"}}

	msg = dipper.DeserializePayload(msg)
	convID := dipper.MustGetMapDataStr(msg.Payload, "convID")
	counter := dipper.MustGetMapDataStr(msg.Payload, "counter")
	timeout, ok := msg.Labels["timeout"]
	if !ok || len(timeout) == 0 {
		timeout = "30s"
	}
	step := d.Name + "/conv/" + convID + "/" + counter

	resp, _ := d.Call("cache", "blpop", map[string]any{"key": step + "/response"}, "timeout", timeout)

	var cancelled bool
	if len(resp) == 0 {
		cancelled = len(dipper.Must(d.CallRaw("cache", "exists", []byte(step))).([]byte)) == 0
		if cancelled {
			// check the result again in case there is a race condition btween chatContinue and chatRelay.
			resp, _ = d.Call("cache", "blpop", map[string]any{"key": step + "/response"}, "timeout", timeout)
		}
	}

	ret := dipper.Message{}
	if len(resp) == 0 {
		ret.Payload = map[string]any{"done": cancelled, "content": "", "type": ""}
	} else {
		ret.Payload = resp
		ret.IsRaw = true
	}

	msg.Reply <- ret
}

func ChatStop(d *dipper.Driver, msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	convID := dipper.MustGetMapDataStr(msg.Payload, "convID")
	prefix := d.Name + "/conv/" + convID + "/"

	counter, _ := dipper.GetMapDataStr(msg.Payload, "counter")
	if counter == "" {
		counter = string(dipper.Must(d.Call("cache", "load", map[string]any{"key": prefix + "counter"})).([]byte))
	}

	step := d.Name + "/conv/" + convID + "/" + counter
	dipper.Must(d.CallNoWait("cache", "del", map[string]any{"key": step}))
	msg.Reply <- dipper.Message{}
}

func ChatListen(d *dipper.Driver, msg *dipper.Message, builder func(string, string) any) {
	msg = dipper.DeserializePayload(msg)
	convID := dipper.MustGetMapDataStr(msg.Payload, "convID")
	prefix := d.Name + "/conv/" + convID + "/"
	inConversation := len(dipper.Must(d.CallRaw("cache", "exists", []byte(prefix+"history"))).([]byte)) > 0
	if !inConversation {
		msg.Reply <- dipper.Message{}

		return
	}

	user := dipper.MustGetMapDataStr(msg.Payload, "user")
	prompt := dipper.MustGetMapDataStr(msg.Payload, "prompt")
	userMessage := builder(user, prompt)

	dipper.Must(d.CallNoWait("cache", "rpush", map[string]any{
		"key":   prefix + "history",
		"value": string(dipper.Must(json.Marshal(userMessage)).([]byte)),
	}))

	msg.Reply <- dipper.Message{}
}
