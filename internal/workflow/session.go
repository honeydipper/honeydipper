// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"golang.org/x/exp/slices"
)

// ErrWorkflowError is the base error for all workflow related errors.
var ErrWorkflowError = errors.New("workflow error")

// Session is the data structure about a running workflow and its definition.
type Session struct {
	ID             string
	EventID        string
	workflow       *config.Workflow
	current        int32 // current thread or step
	iteration      int32 // current item in the iteration list
	iterationLock  *sync.Mutex
	iterationOut   *dipper.Message
	loopCount      int // counter for looping
	parent         string
	ctx            map[string]interface{}
	ctxLock        *sync.Mutex
	event          map[string]interface{}
	exported       []map[string]interface{}
	elseBranch     *config.Workflow
	inFlyFunction  *config.Function
	store          *SessionStore
	loadedContexts []string
	currentHook    string
	savedMsg       *dipper.Message
	origMsg        *dipper.Message
	performing     string
	isHook         bool
	context        context.Context
	cancelFunc     context.CancelFunc
	startTime      time.Time
	completionTime time.Time
}

// SessionHandler prepare and execute the session provides entry point for SessionStore to invoke and mock for testing.
type SessionHandler interface {
	prepare(msg *dipper.Message, parent interface{}, ctx map[string]interface{})
	execute(msg *dipper.Message)
	continueExec(msg *dipper.Message, exports []map[string]interface{})
	onError()
	GetName() string
	GetDescription() string
	GetParent() string
	GetEventID() string
	GetEventName() string
	GetStatus() (string, string)
	GetExported() []map[string]interface{}
	Watch() <-chan struct{}
	GetStartTime() time.Time
	GetCompletionTime() time.Time
}

const (
	// SessionStatusSuccess means the workflow executed successfully.
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

// buildEnvData builds a map of environmental data for interpolation.
func (w *Session) buildEnvData(msg *dipper.Message) map[string]interface{} {
	data := msg.Payload
	if data == nil {
		data = map[string]interface{}{}
	}

	envData := map[string]interface{}{
		"data":   data,
		"labels": msg.Labels,
		"event":  w.event,
		"ctx":    w.ctx,
	}

	return envData
}

// save will store the session in the memory.
func (w *Session) save() {
	if w.ID == "" {
		w.ID = dipper.IDMapPut(&w.store.sessions, w)
		dipper.Logger.Infof("[workflow] session with parent [%s] saved as [%s]", w.parent, w.ID)
	}
}

// isLoop checks if the workflow uses looping statements while and until.
func (w *Session) isLoop() bool {
	return len(w.workflow.While) > 0 ||
		len(w.workflow.Until) > 0 ||
		len(w.workflow.WhileAny) > 0 ||
		len(w.workflow.UntilAll) > 0 ||
		w.workflow.WhileMatch != nil ||
		w.workflow.UntilMatch != nil
}

// isIteration checks if the workflow needs to iterate through a list.
func (w *Session) isIteration() bool {
	return w.workflow.Iterate != nil || w.workflow.IterateParallel != nil
}

// lenOfIterate gives the length of the iteration list.
func (w *Session) lenOfIterate() int {
	var it reflect.Value
	if w.workflow.Iterate != nil {
		it = reflect.ValueOf(w.workflow.Iterate)
	} else {
		it = reflect.ValueOf(w.workflow.IterateParallel)
	}

	return it.Len()
}

// isFunction checks if the workflow is a simple function call.
func (w *Session) isFunction() bool {
	return w.workflow.Function.Driver != "" || w.workflow.Function.Target.System != ""
}

// injectMsg injects the dipper message data into the session as event.
func (w *Session) injectMsg(msg *dipper.Message) {
	if w.parent == "" {
		data, _ := dipper.GetMapData(msg.Payload, "data")
		if data != nil {
			d := data.(map[string]interface{})
			w.event = d
		} else {
			w.event = map[string]interface{}{}
		}
	}
}

// injectNamedCTX inject a named context into the workflow.
func (w *Session) injectNamedCTX(name string, msg *dipper.Message, firstTime bool) {
	contexts := w.store.Helper.GetConfig().DataSet.Contexts

	namedCTXs, ok := contexts[name]
	if name[0] != '_' && !ok {
		dipper.Logger.Panicf("[workflow] named context %s not defined", name)
	}

	if namedCTXs == nil {
		return
	}

	envData := w.buildEnvData(msg)
	ctx, ok := namedCTXs.(map[string]interface{})["*"]
	if firstTime && ok {
		ctx = dipper.MustDeepCopyMap(ctx.(map[string]interface{}))
		ctx = dipper.Interpolate(ctx, envData)
		w.ctx = dipper.MergeMap(w.ctx, ctx)
		dipper.Logger.Infof("merged global values (*) from named context %s to workflow", name)
	}

	if w.parent == "" {
		ctx, ok := namedCTXs.(map[string]interface{})["_events"]
		if ok {
			ctx = dipper.MustDeepCopyMap(ctx.(map[string]interface{}))
			ctx = dipper.Interpolate(ctx, envData)
			w.ctx = dipper.MergeMap(w.ctx, ctx)
			dipper.Logger.Infof("[workflow] merged _events section of context [%s] to workflow [%s]", name, w.workflow.Name)
		}
	}

	if w.workflow.Name != "" {
		ctx, ok := namedCTXs.(map[string]interface{})[w.workflow.Name]
		if ok {
			ctx = dipper.MustDeepCopyMap(ctx.(map[string]interface{}))
			ctx = dipper.Interpolate(ctx, envData)
			w.ctx = dipper.MergeMap(w.ctx, ctx)
			dipper.Logger.Infof("[workflow] merged named context [%s] to workflow [%s]", name, w.workflow.Name)
		}
	}
}

// initCTX initialize the contextual data used in this workflow.
func (w *Session) initCTX(msg *dipper.Message) {
	w.injectNamedCTX(SessionContextDefault, msg, w.parent == "")
	if w.parent == "" {
		w.injectNamedCTX(SessionContextEvents, msg, true)
	}

	for _, name := range w.loadedContexts {
		if name == SessionContextHooks {
			w.isHook = true
		}
		w.injectNamedCTX(name, msg, false)
	}

	w.injectCTXs(msg)

	if w.isHook {
		// avoid hook in hook
		delete(w.ctx, "hooks")
	}
}

// injectCTXs loads the contexts specified through context or contexts fields.
func (w *Session) injectCTXs(msg *dipper.Message) {
	envdata := w.buildEnvData(msg)
	w.workflow.Context = dipper.InterpolateStr(w.workflow.Context, envdata)
	w.workflow.Contexts = dipper.Interpolate(w.workflow.Contexts, envdata)

	if w.workflow.Context != "" {
		if w.workflow.Context == SessionContextHooks {
			w.isHook = true
		}

		if !slices.Contains(w.loadedContexts, w.workflow.Context) {
			w.injectNamedCTX(w.workflow.Context, msg, true)
			w.loadedContexts = append(w.loadedContexts, w.workflow.Context)
		}
	}

	if w.workflow.Contexts != nil {
		for _, n := range w.workflow.Contexts.([]interface{}) {
			if n == nil {
				continue
			}
			name, ok := n.(string)
			if !ok {
				panic(fmt.Errorf("%w: expected list of strings in contexts in workflow: %s", ErrWorkflowError, w.workflow.Name))
			}
			if name == "" {
				continue
			}
			// at this stage the hooks flag is added only through `context` not `contexts`
			// this part of the code is unreachable
			// if name == SessionContextHooks {
			//	 w.isHook = true
			// }
			if !slices.Contains(w.loadedContexts, name) {
				w.injectNamedCTX(name, msg, true)
				w.loadedContexts = append(w.loadedContexts, name)
			}
		}
	}
}

// injectMeta injects the meta info into context.
func (w *Session) injectMeta() {
	if !w.isHook {
		if w.workflow.Name != "" {
			w.ctx["_meta_name"] = w.workflow.Name
		} else {
			w.ctx["_meta_name"] = w.performing
		}
		w.ctx["_meta_desc"] = w.workflow.Description
	}
}

// injectEventCTX injects the contextual data from the event into the workflow.
func (w *Session) injectEventCTX(ctx map[string]interface{}) {
	if ctx != nil {
		w.ctx = dipper.MergeMap(w.ctx, ctx)
	}
}

// injectLocalCTX injects the workflow local context data.
func (w *Session) injectLocalCTX(msg *dipper.Message) {
	if w.workflow.Local != nil && w.workflow.CallDriver == "" {
		layers, ok := w.workflow.Local.([]interface{})
		if !ok {
			layers = []interface{}{w.workflow.Local}
		}
		envData := w.buildEnvData(msg)
		for _, l := range layers {
			locals := dipper.Interpolate(l, envData)
			envData["ctx"] = dipper.MergeMap(envData["ctx"].(map[string]interface{}), locals)
		}
		w.ctx = envData["ctx"].(map[string]interface{})
	}
}

// interpolateWorkflow creates a copy of the workflow and interpolates it with envData.
func (w *Session) interpolateWorkflow(msg *dipper.Message) {
	v := w.workflow
	envData := w.buildEnvData(msg)
	ret := *v

	ret.Name = dipper.InterpolateStr(v.Name, envData)
	ret.Description = dipper.InterpolateStr(v.Description, envData)
	ret.If = dipper.Interpolate(v.If, envData).([]string)
	ret.IfAny = dipper.Interpolate(v.IfAny, envData).([]string)
	ret.Unless = dipper.Interpolate(v.Unless, envData).([]string)
	ret.UnlessAll = dipper.Interpolate(v.UnlessAll, envData).([]string)
	ret.Match = dipper.Interpolate(v.Match, envData)
	ret.UnlessMatch = dipper.Interpolate(v.UnlessMatch, envData)
	ret.Retry = dipper.InterpolateStr(v.Retry, envData)
	ret.Backoff = dipper.InterpolateStr(v.Backoff, envData)
	ret.Wait = dipper.InterpolateStr(v.Wait, envData)
	ret.CallFunction = dipper.InterpolateStr(v.CallFunction, envData)
	ret.CallDriver = dipper.InterpolateStr(v.CallDriver, envData)

	ret.Iterate = dipper.Interpolate(v.Iterate, envData)
	if ret.Iterate == nil && v.Iterate != nil {
		ret.Iterate = []interface{}{}
	}
	ret.IterateParallel = dipper.Interpolate(v.IterateParallel, envData)
	if ret.IterateParallel == nil && v.IterateParallel != nil {
		ret.IterateParallel = []interface{}{}
	}
	if v.IterateParallel != nil {
		ret.IteratePool = dipper.InterpolateStr(v.IteratePool, envData)
	}

	// ret.While = v.While                     // repeatedly interpolated later
	// ret.WhileAny = v.WhileAny               // repeatedly interpolated later
	// ret.WhileMatch = v.WhileMatch           // repeatedly interpolated later
	// ret.Until = v.Until                     // repeatedly interpolated later
	// ret.UntilAll = v.UntilAll               // repeatedly interpolated later
	// ret.UntilMatch = v.UntilMatch           // repeatedly interpolated later
	// ret.Else = v.Else                       // delayed
	// ret.Workflow = v.Workflow               // delayed
	// ret.Function = v.Function               // delayed
	// ret.Steps = v.Steps                     // delayed
	// ret.Threads = v.Threads                 // delayed
	// ret.Export = v.Export                   // delayed
	// ret.ExportOnSuccess = v.ExportOnSuccess // delayed
	// ret.ExportOnFailure = v.ExportOnFailure // delayed
	// ret.ExportOnError = v.ExportOnError     // delayed
	// ret.Switch = v.Switch                   // delayed
	// ret.Cases = v.Cases                     // delayed
	// ret.Default = v.Default                 // delayed

	// ret.Context = v.Context     // interpolated in initCTX
	// ret.Contexts = v.Contexts   // interpolated in initCTX
	// ret.NoExport = v.NoExport   // no interpolation
	// ret.IterateAs = v.IterateAs // no interpolation
	// ret.OnError = v.OnError     // no interpolation
	// ret.OnFailure = v.OnFailure // no interpolation
	// ret.Local = v.Local         // no interpolation
	// ret.Detach = v.Detach       // no interpolation

	w.workflow = &ret
}

// inheritParentSettings copies some workflow settings from the parent session.
func (w *Session) inheritParentSettings(p *Session) {
	if w.workflow.OnError == "" {
		w.workflow.OnError = p.workflow.OnError
	}
	if w.workflow.OnFailure == "" {
		w.workflow.OnFailure = p.workflow.OnFailure
	}
}

// createChildSession creates a child workflow session.
func (w *Session) createChildSession(wf *config.Workflow, msg *dipper.Message) *Session {
	child := w.store.newSession(w.ID, w.EventID, wf)
	child.prepare(msg, w, nil)

	return child.(*Session)
}

// setPerforming records what is happening within the workflow.
func (w *Session) setPerforming(performing string) string {
	wf := w.workflow
	switch {
	case performing != "":
		w.performing = performing
	case wf.Description != "":
		w.performing = wf.Description
	case wf.Function.Target.System != "":
		w.performing = wf.Function.Target.System + "." + wf.Function.Target.Function
	case wf.Function.Driver != "":
		w.performing = "driver:" + wf.Function.Driver + "." + wf.Function.RawAction
	case wf.CallFunction != "":
		w.performing = wf.CallFunction
	case wf.CallDriver != "":
		w.performing = wf.CallDriver
	case wf.Workflow != "":
		w.performing = "calling " + wf.Workflow
	case w.isIteration():
		w.performing = "iterating"
	case w.isLoop():
		w.performing = "looping"
	case len(wf.Steps) > 0:
		w.performing = "steps"
	case len(wf.Threads) > 0:
		w.performing = "threads"
	default:
		w.performing = wf.Name
	}

	return w.performing
}

// inheritParentData prepares the session using parent data.
func (w *Session) inheritParentData(parent *Session) {
	parent.setPerforming(w.performing)

	w.event = parent.event
	w.ctx = dipper.MustDeepCopyMap(parent.ctx)
	w.loadedContexts = parent.loadedContexts

	delete(w.ctx, "hooks") // hooks don't get inherited
}

// prepare prepares a session for execution.
func (w *Session) prepare(msg *dipper.Message, parent interface{}, ctx map[string]interface{}) {
	if parent != nil {
		w.inheritParentData(parent.(*Session))
	}
	w.injectMsg(msg)
	w.initCTX(msg)
	if ctx != nil {
		w.injectEventCTX(ctx)
	}
	w.injectLocalCTX(msg)
	w.interpolateWorkflow(msg)
	if parent != nil {
		w.inheritParentSettings(parent.(*Session))
	}
	w.injectMeta()
}

// createChildSessionWithName creates a child workflow session.
func (w *Session) createChildSessionWithName(name string, msg *dipper.Message) *Session {
	src, ok := w.store.Helper.GetConfig().DataSet.Workflows[name]
	if !ok {
		panic(fmt.Errorf("%w: not defined: %s", ErrWorkflowError, name))
	}
	if src.Name == "" {
		src.Name = name
	}
	wf := &src

	return w.createChildSession(wf, msg)
}

// GetName returns the workflow name.
func (w *Session) GetName() string {
	return w.workflow.Name
}

// GetDescription returns the workflow description.
func (w *Session) GetDescription() string {
	return w.workflow.Description
}

// GetEventID returns the global unique eventID.
func (w *Session) GetEventID() string {
	return w.EventID
}

// GetParent returns the parent ID of the session.
func (w *Session) GetParent() string {
	return w.parent
}

// Watch returns a channel for watching the session.
func (w *Session) Watch() <-chan struct{} {
	if w.context == nil {
		w.context, w.cancelFunc = context.WithCancel(context.Background())
	}

	return w.context.Done()
}

// GetStatus return the session status.
func (w *Session) GetStatus() (string, string) {
	var (
		status, reason string
		ok             bool
	)
	if status, ok = w.savedMsg.Labels["status"]; ok {
		status = SessionStatusSuccess
	}
	reason = w.savedMsg.Labels["reason"]

	return status, reason
}

// GetExported returns the exported data from the session.
func (w *Session) GetExported() []map[string]interface{} {
	return w.exported
}

// GetEventName returns the name of the event.
func (w *Session) GetEventName() string {
	evn, ok := w.ctx["_meta_event"]
	if ok {
		return evn.(string)
	}

	return ""
}

// GetStartTime returns the start time of the session.
func (w *Session) GetStartTime() time.Time {
	return w.startTime
}

// GetCompletionTime returns the complete time of the session.
func (w *Session) GetCompletionTime() time.Time {
	return w.completionTime
}
