// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const (
	// SessionStatusSuccess means the workflow completed successfully.
	SessionStatusSuccess = "success"
	// SessionStatusFailure means the workflow completed with failure.
	SessionStatusFailure = "failure"
	// SessionStatusError means the workflow ran into error, and was not able to complete.
	SessionStatusError = "error"

	// SessionContextDefault is a builtin context for all workflows.
	SessionContextDefault = "_default"
	// SessionContextEvents is a context with default values for all directly event triggered workflows.
	SessionContextEvents = "_events"
	// SessionContextHooks is a context for all hooks workflows.
	SessionContextHooks = "_hooks"
)

// ErrWorkflowError is the base error for all workflow related errors.
var ErrWorkflowError = errors.New("workflow error")

// Session is the data structure about a running workflow and its definition.
type Session struct {
	// EventID identifies the event that triggered the workflow.
	EventID string
	// ID is the unique ID of the workflow session.
	ID string
	// Parent is the parent workflow session ID.
	Parent string

	// Current is the current step or thread.
	Current int
	// Iteration is the current iteration of the iteration list.
	Iteration int
	// LoopCount is the counter for looping.
	LoopCount int

	// Ctx is the context data of the workflow session.
	Ctx map[string]interface{}
	// Event is the event data that triggered the workflow.
	Event map[string]interface{}
	// EventCtx is the original context exported from the event.
	EventCtx map[string]interface{}
	// Exported is the exported data from the workflow.
	Exported []map[string]interface{}

	// ElseBranch is the else branch of the workflow.
	ElseBranch *config.Workflow
	// InFlyFunction is the function that is currently being executed.
	InFlyFunction *config.Function
	// Workflow is the workflow definition.
	Workflow *config.Workflow

	// Performing describes the current action being performed.
	Performing *[]string

	// LoadedContexts is the list of context names that have been loaded.
	LoadedContexts []string
	// CurrentMsg is the latest message that has been received from operators as results.
	CurrentMsg *dipper.Message
	// OrigMsg is the original return message before hook execution.
	OrigMsg *dipper.Message
	// OrigChild is the original child session before hook execution.
	OrigChild *Session

	// CurrentHook is the current hook that is being executed.
	CurrentHook string
	// IsHook is a flag that indicates if session is a hook execution for another session.
	IsHook bool

	// CompletionTime is the time when the workflow session completed.
	CompletionTime time.Time
	// StartTime is the time when the workflow session started.
	StartTime time.Time
	// State represents the current state of the session.
	State int
	// IsNoop is a flag that indicates if the workflow is a noop.
	IsNoop *bool
	// Action is a counter to track how many actions have been performed, used for displaying.
	Action int

	threads    *sync.WaitGroup
	cancelFunc context.CancelFunc
	context    context.Context
	parent     *Session
	store      Store
	child      *Session
	depth      int
	pending    bool
	brief      string
}

// Marshal converts the live session into a byte array for persistence.
func (w *Session) Marshal() []byte {
	return dipper.Must(json.Marshal(w)).([]byte)
}

// Unmarshal converts the byte array into a live session.
func (w *Session) Unmarshal(data []byte) {
	dipper.Must(json.Unmarshal(data, w))
}

// Cancel cancels the workflow session.
func (w *Session) Cancel() {
	w.cancelFunc()
}

// GetContext returns the context object of the workflow session.
func (w *Session) GetContext() context.Context {
	return w.context
}

// buildEnvData builds a map of environmental data for interpolation.
func (w *Session) buildEnvData() map[string]interface{} {
	data := w.CurrentMsg.Payload
	if data == nil {
		data = map[string]interface{}{}
	}
	labels := w.CurrentMsg.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	if !w.IsHook {
		if w.Workflow.Name != "" {
			w.Ctx["_meta_name"] = w.Workflow.Name
		} else {
			w.Ctx["_meta_desc"] = w.Brief()
		}
	}
	envData := map[string]interface{}{
		"data":   data,
		"labels": labels,
		"event":  w.Event,
		"ctx":    w.Ctx,
	}

	return envData
}

// interpolateFunction interplotes the system name and function names in the target.
func (w *Session) interpolateFunction(f *config.Function) *config.Function {
	envData := w.buildEnvData()
	interpolatedFunc := *f
	interpolatedFunc.Target.System = dipper.InterpolateStr(f.Target.System, envData)
	interpolatedFunc.Target.Function = dipper.InterpolateStr(f.Target.Function, envData)

	return &interpolatedFunc
}

func (w *Session) performingValues() []string {
	if w.Performing == nil {
		return []string{}
	}

	return *w.Performing
}

func (w *Session) trimPerformingToCurrentDepth() {
	if w.Performing == nil {
		return
	}
	target := w.depth + 1
	if len(*w.Performing) > target {
		*w.Performing = (*w.Performing)[:target]
	}
}

// setPerforming sets the current performing action.
func (w *Session) setPerforming(action string) {
	p := w.Brief() + " - " + action
	logger := w.store.GetLogger()
	if logger != nil {
		logger.Debugf("session [%s.%s] depth %d: %s",
			w.ID,
			w.CurrentMsg.Labels["cursor"],
			w.depth,
			p,
		)
	}
	(*w.Performing)[w.depth] = p
}

// Brief derives the brief description of the workflow.
func (w *Session) Brief() string {
	if w.brief != "" {
		return w.brief
	}

	w.brief = "unamed workflow"

	wf := w.Workflow
	if wf == nil {
		return w.brief
	}

	switch {
	case wf.Name != "":
		w.brief = wf.Name
	case wf.Description != "":
		if wf.IterateParallel == nil {
			// for iterate parallel workflow, description is interpolated in each child session to
			// include the current item value. For non-iterate parallel workflow, we can use the
			// workflow description as brief.
			w.brief = wf.Description
		} else {
			w.brief = "parallel iteration"
		}
	case w.isIteration():
		w.brief = "iteration"
	case w.isLoop():
		w.brief = "looping"
	case wf.Function.Target.System != "":
		w.brief = "system func: " + wf.Function.Target.System + "." + wf.Function.Target.Function
	case wf.Function.Driver != "":
		w.brief = "driver func: " + wf.Function.Driver + "." + wf.Function.RawAction
	case wf.CallFunction != "":
		w.brief = "system func: " + wf.CallFunction
	case wf.CallDriver != "":
		w.brief = "driver func: " + wf.CallDriver
	case wf.Workflow != "":
		w.brief = "wrapper: " + wf.Workflow
	case len(wf.Steps) > 0:
		w.brief = "steps"
	case len(wf.Threads) > 0:
		w.brief = "threads"
	case len(wf.Cases) > 0:
		w.brief = "switch"
	}

	return w.brief
}

// isLoop checks if the workflow uses looping statements while and until.
func (w *Session) isLoop() bool {
	return len(w.Workflow.While) > 0 ||
		len(w.Workflow.Until) > 0 ||
		len(w.Workflow.WhileAny) > 0 ||
		len(w.Workflow.UntilAll) > 0 ||
		w.Workflow.WhileMatch != nil ||
		w.Workflow.UntilMatch != nil
}

// isIteration checks if the workflow needs to iterate through a list.
func (w *Session) isIteration() bool {
	return w.Workflow.Iterate != nil || w.Workflow.IterateParallel != nil
}

// lenOfIterate gives the length of the iteration list.
func (w *Session) lenOfIterate() int {
	var it reflect.Value
	if w.Workflow.Iterate != nil {
		it = reflect.ValueOf(w.Workflow.Iterate)
	} else {
		it = reflect.ValueOf(w.Workflow.IterateParallel)
	}

	return it.Len()
}

// isFunction checks if the workflow is a simple function call.
func (w *Session) isFunction() bool {
	return w.Workflow.Function.Driver != "" || w.Workflow.Function.Target.System != ""
}

// Wait waits for the session to wrap up the current operations and ready to be deactivated.
func (w *Session) Wait() {
	w.threads.Wait()
}

// GetStatus return the session status.
func (w *Session) GetStatus() (string, string) {
	var (
		status, reason string
		ok             bool
	)
	if status, ok = w.CurrentMsg.Labels["status"]; !ok {
		status = SessionStatusSuccess
	}
	reason = w.CurrentMsg.Labels["reason"]

	return status, reason
}

// GetEventName returns the name of the event.
func (w *Session) GetEventName() string {
	evn, ok := w.Ctx["_meta_event"]
	if ok {
		return evn.(string)
	}

	return ""
}

// checkIsNoop checks if the workflow does nothing but pass through.
func (w *Session) checkIsNoop() bool {
	if w.IsNoop != nil {
		return *w.IsNoop
	}

	ret := false
	switch {
	case w.Workflow.Workflow != "":
	case w.isFunction():
	case w.Workflow.CallDriver != "":
	case w.Workflow.CallFunction != "":
	case len(w.Workflow.Steps) != 0:
	case len(w.Workflow.Threads) != 0:
	case w.Workflow.Wait != "":
	case w.Workflow.Switch != "":
	case w.Workflow.Resume != "":
	default:
		ret = true
	}

	w.IsNoop = &ret

	return ret
}

// incCursor increments the cursor in the message label to sync between incoming and outgoing messages.
func (w *Session) incCursor() {
	cursor := 0
	if _, ok := w.CurrentMsg.Labels["cursor"]; ok {
		cursor = dipper.Must(strconv.Atoi(w.CurrentMsg.Labels["cursor"])).(int)
	}

	w.CurrentMsg.Labels["cursor"] = strconv.Itoa(cursor + 1)
}

func (w *Session) resolveLogStream() interface{} {
	for current := w; current != nil; current = current.child {
		if current.Ctx == nil {
			continue
		}

		if stream, ok := current.Ctx["_log_stream"]; ok && stream != nil {
			return stream
		}
	}

	return nil
}

// Dump converts the session info to a map for exporting.
func (w *Session) Dump() map[string]interface{} {
	labels := map[string]string{}
	for k, v := range w.CurrentMsg.Labels {
		labels[k] = v
	}
	delete(labels, "performing")
	delete(labels, "sessionID")
	delete(labels, "eventID")

	if !w.StartTime.IsZero() {
		labels["start"] = w.StartTime.Format(time.RFC3339Nano)
	}
	if !w.CompletionTime.IsZero() {
		labels["end"] = w.CompletionTime.Format(time.RFC3339Nano)
	}

	isNoop := w.IsNoop == nil || *w.IsNoop
	if w.State == SessionStateDone {
		isNoop = w.Action == 0

		if !isNoop {
			isNoop, _ = dipper.GetMapDataBool(w.Ctx, "_effectively_noop")
		}
	}

	description := ""
	if w.Workflow != nil {
		description = w.Workflow.Description
	}
	ret := map[string]interface{}{
		"data": map[string]any{
			"brief":       w.Brief(),
			"description": description,
			"state":       SessionStates[w.State],
			"output":      w.Ctx["_output"],
			"log_stream":  w.resolveLogStream(),
			"is_noop":     isNoop,
			"is_hook":     w.IsHook,
			"session_id":  w.ID,
			"event_id":    w.EventID,
			"event_name":  w.GetEventName(),
			"event_ctx":   w.EventCtx,
			"parent":      w.Parent,
		},
		"labels": labels,
	}

	if w.State != SessionStateDone {
		ret["performing"] = w.performingValues()
	} else if labels["status"] == SessionStatusError || labels["status"] == SessionStatusFailure {
		ret["performing"] = strings.Split(w.CurrentMsg.Labels["performing"], "\n")
	}

	return ret
}
