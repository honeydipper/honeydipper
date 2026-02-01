package ai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/honeydipper/honeydipper/v3/pkg/workflow"
	"github.com/mitchellh/mapstructure"
)

func (w *ChatWrapper) fetchCache(_ string, v any) (any, bool) {
	s, ok := v.(string)
	if ok && strings.HasPrefix(s, "@cache:") {
	} else {
		return nil, false
	}
	parts := strings.Split(strings.TrimPrefix(s, "@cache:"), ",")
	var useRaw bool
	var useStep bool
	var del bool
	if len(parts) > 3 {
		useRaw = parts[0] == "raw"
		parts = parts[1:]
	}
	if len(parts) > 2 {
		del = parts[0] == "del"
		parts = parts[1:]
	}
	if len(parts) > 1 {
		useStep = parts[0] == "step"
		parts = parts[1:]
	}
	var key string
	if useStep {
		key = w.step + "/" + parts[0]
	} else {
		key = w.prefix + "/" + parts[0]
	}

	dipper.Logger.Debugf("fetching cache key %s, raw %v, del %v", key, useRaw, del)
	val := dipper.Must(w.driver.CallWithMessage(&dipper.Message{
		Labels: map[string]string{
			"feature": "cache",
			"method":  "lrange",
		},
		Payload: map[string]any{
			"key": key,
			"raw": useRaw,
			"del": del,
		},
	})).([]byte)

	return string(val), true
}

func (w *ChatWrapper) toolCallHandler(jsonMessage string, args map[string]any, name string, callID string) {
	defer dipper.SafeExitOnError("failed to call driver", func(x any) {
		w.relayFuncReturn(name, callID, dipper.Must(json.Marshal(map[string]any{"error": x})).([]byte))
	})

	if w.builder.Len() > 0 {
		w.chatEmit(false)
	}
	w.AppendHistory(jsonMessage)

	dipper.Recursive(args, w.fetchCache)

	wf := dipper.MustGetMapData(w.driver.Options, "data.tools."+name+".workflow").(*config.Workflow)
	if wf.Function.RawAction == "rpc" {
		w.toolCallDriverHandler(name, args, wf, callID)

		return
	}

	var data any = args
	if local, ok := dipper.GetMapData(wf.Local, "_local"); ok {
		delete(wf.Local.(map[string]any), "_local")

		// interpolate the fields
		envData := map[string]any{
			"args":   args,
			"prefix": w.prefix,
			"step":   w.step,
		}
		local = dipper.Interpolate(local, envData)
		dipper.Recursive(local, w.fetchCache)

		if len(wf.Local.(map[string]any)) > 0 {
			wf.Local = []any{local, wf.Local}
		} else {
			wf.Local = local
		}
	}

	id := w.driver.EmitEvent(map[string]any{
		"do":   wf,
		"data": data,
	})

	dipper.Logger.Debugf("Waiting for event ID to finish %s", id)
	retMsg := workflow.Wait(context.Background(), w.driver, id, w.step).Dump()
	ret, _ := dipper.GetMapData(retMsg, "data.output")
	dipper.Logger.Debugf("Got result for ai: %+v", ret)

	w.relayFuncReturn(name, callID, dipper.SerializeContent(ret))
}

func (w *ChatWrapper) toolCallDriverHandler(name string, args map[string]any, wf *config.Workflow, callID string) {
	// build rpc calling message
	msg := &dipper.Message{}
	if p, found := dipper.GetMapData(wf.Local, "parameters"); found {
		dipper.Must(mapstructure.Decode(p, &msg.Payload))
	}
	if l, found := dipper.GetMapData(wf.Local, "labels"); found {
		dipper.Must(mapstructure.Decode(l, &msg.Labels))
	}

	// interpolate the fields
	envData := map[string]any{
		"args":   args,
		"prefix": w.prefix,
		"step":   w.step,
	}
	msg.Payload = dipper.Interpolate(msg.Payload, envData)
	noWaitData, _ := dipper.GetMapData(wf.Local, "no_wait")
	noWait := dipper.IsTruthy(dipper.Interpolate(noWaitData, envData))

	// interpolate the labels
	for k, v := range msg.Labels {
		msg.Labels[k] = dipper.InterpolateStr(v, envData)
	}

	// setup callee information
	if msg.Labels == nil {
		msg.Labels = map[string]string{}
	}
	parts := strings.Split(wf.CallDriver, ".")
	msg.Labels["feature"], msg.Labels["method"] = parts[0], parts[1]

	// call the driver
	if noWait {
		dipper.Must(w.driver.CallWithMessageNoWait(msg))
	} else {
		envData["ret"] = dipper.DeserializeContent(dipper.Must(w.driver.CallWithMessage(msg)).([]byte))
	}
	dipper.Logger.Debugf("Got rpc result for ai: %+v", envData["ret"])

	// interpolate the return
	o, _ := dipper.GetMapData(wf.Local, "output")
	o = dipper.Interpolate(o, envData)

	w.relayFuncReturn(name, callID, dipper.Must(json.Marshal(o)).([]byte))
}

func (w *ChatWrapper) relayFuncReturn(name string, callID string, b []byte) {
	ret := w.chatter.BuildToolReturnMessage(name, callID, b)
	w.AppendHistory(string(dipper.Must(json.Marshal(ret)).([]byte)))

	defer dipper.SafeExitOnError("failed to relay ai chat session")

	w.chatter.StreamWithFunctionReturn(ret, w.streamHandler, w.toolCallHandler)
}
