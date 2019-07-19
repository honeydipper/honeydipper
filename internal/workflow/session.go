// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"reflect"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// Session is the data structure about a running workflow and its definition.
type Session struct {
	ID                string
	workflow          *config.Workflow
	current           int32 // current thread or step
	iteration         int32 // current item in the iteration list
	loopCount         int   // counter for looping
	parent            string
	ctx               map[string]interface{}
	event             map[string]interface{}
	exported          map[string]interface{}
	elseBranch        *config.Workflow
	collapsedFunction *config.CollapsedFunction
	store             *SessionStore
	loadedContexts    []string
	currentHook       string
	savedMsg          *dipper.Message
}

const (
	// SessionStatusSuccess means the workflow executed successfully
	SessionStatusSuccess = "success"
	// SessionStatusFailure means the workflow completed with failure
	SessionStatusFailure = "failure"
	// SessionStatusError means the workflow ran into error, and was not able to complete
	SessionStatusError = "error"

	// SessionContextDefault is a builtin context for all workflows
	SessionContextDefault = "_default"
	// SessionContextEvents is a context with default values for all directly event triggered workflows
	SessionContextEvents = "_events"
	// SessionContextHooks is a context for all hooks workflows
	SessionContextHooks = "_hooks"
)

// buildEnvData builds a map of environmental data for interpolation
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

// save will store the session in the memory
func (w *Session) save() string {
	if w.ID == "" {
		w.ID = dipper.IDMapPut(&w.store.sessions, w)
		dipper.Logger.Infof("[workflow] session with parent [%s] saved as [%s]", w.parent, w.ID)
	}
	return w.ID
}

// isLoop checks if the workflow uses looping statements while and until
func (w *Session) isLoop() bool {
	return len(w.workflow.While) > 0 || len(w.workflow.Until) > 0 || len(w.workflow.WhileAny) > 0 || len(w.workflow.UntilAny) > 0
}

// isIteration checks if the workflow needs to iterate through a list
func (w *Session) isIteration() bool {
	return w.workflow.Iterate != nil || w.workflow.IterateParallel != nil
}

// lenOfIterate gives the length of the iteration list
func (w *Session) lenOfIterate() int {
	var it reflect.Value
	if w.workflow.Iterate != nil {
		it = reflect.ValueOf(w.workflow.Iterate)
	} else {
		it = reflect.ValueOf(w.workflow.IterateParallel)
	}

	return it.Len()
}

// isFunction checks if the workflow is a simple function call
func (w *Session) isFunction() bool {
	return w.workflow.Function.Driver != "" || w.workflow.Function.Target.System != ""
}

// injectMsg injects the dipper message data into the session as event
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

// injectNamedCTX inject a named context into the workflow
func (w *Session) injectNamedCTX(name string) {
	var err error
	var contexts = w.store.GetConfig().DataSet.Contexts

	namedCTXs, ok := contexts[name]
	if name[0] != '_' && !ok {
		dipper.Logger.Panicf("[workflow] named workflow %s not defined", name)
	}
	if namedCTXs != nil {
		ctx, ok := namedCTXs[w.workflow.Name]
		if !ok {
			ctx = namedCTXs["default"]
		}
		ctx, err = dipper.DeepCopy(ctx)
		if err != nil {
			panic(err)
		}
		dipper.MergeMap(w.ctx, ctx)
	}
}

// initCTX initialize the contextual data used in this workflow
func (w *Session) initCTX(wf *config.Workflow) {
	w.injectNamedCTX(SessionContextDefault)
	if w.parent == "" {
		w.injectNamedCTX(SessionContextEvents)
	}

	var isHook bool

	for _, name := range w.loadedContexts {
		if name == SessionContextHooks {
			isHook = true
		}
		w.injectNamedCTX(name)
	}

	if wf.Context != "" {
		if wf.Context == SessionContextHooks {
			isHook = true
		}
		w.injectNamedCTX(wf.Context)
		w.loadedContexts = append(w.loadedContexts, wf.Context)
	} else {
		for _, name := range wf.Contexts {
			if name == SessionContextHooks {
				isHook = true
			}
			w.injectNamedCTX(name)
			w.loadedContexts = append(w.loadedContexts, name)
		}
	}

	if isHook {
		// avoid hook in hook
		delete(w.ctx, "hooks")
	}
}

// injectEventCTX injects the contextual data from the event into the workflow
func (w *Session) injectEventCTX(ctx map[string]interface{}) {
	if ctx != nil {
		dipper.MergeMap(w.ctx, ctx)
	}
}

// injectLocalCTX injects the workflow local context data
func (w *Session) injectLocalCTX(wf *config.Workflow, msg *dipper.Message) {
	if wf.Local != nil {
		envData := w.buildEnvData(msg)
		locals := dipper.Interpolate(wf.Local, envData)

		dipper.MergeMap(w.ctx, locals)
	}
}

// interpolateWorkflow creates a copy of the workflow and interpolates it with envData
func (w *Session) interpolateWorkflow(v *config.Workflow, msg *dipper.Message) {
	envData := w.buildEnvData(msg)
	ret := config.Workflow{}

	ret.Name = dipper.InterpolateStr(v.Name, envData)
	ret.If = dipper.Interpolate(v.If, envData).([]string)
	ret.IfAny = dipper.Interpolate(v.IfAny, envData).([]string)
	ret.Unless = dipper.Interpolate(v.Unless, envData).([]string)
	ret.UnlessAny = dipper.Interpolate(v.UnlessAny, envData).([]string)
	ret.Iterate = dipper.Interpolate(v.Iterate, envData)
	ret.IterateParallel = dipper.Interpolate(v.IterateParallel, envData)
	ret.Retry = dipper.InterpolateStr(v.Retry, envData)
	ret.Backoff = dipper.InterpolateStr(v.Backoff, envData)
	ret.Wait = dipper.InterpolateStr(v.Wait, envData)

	ret.While = v.While                     // repeatedly interpolated later
	ret.WhileAny = v.WhileAny               // repeatedly interpolated later
	ret.Until = v.Until                     // repeatedly interpolated later
	ret.UntilAny = v.UntilAny               // repeatedly interpolated later
	ret.Else = v.Else                       // delayed
	ret.Workflow = v.Workflow               // delayed
	ret.Function = v.Function               // delayed
	ret.Steps = v.Steps                     // delayed
	ret.Threads = v.Threads                 // delayed
	ret.Export = v.Export                   // delayed
	ret.ExportOnSuccess = v.ExportOnSuccess // delayed
	ret.ExportOnFailure = v.ExportOnFailure // delayed
	ret.Switch = v.Switch                   // delayed
	ret.Cases = v.Cases                     // delayed

	ret.Context = v.Context   // no interpolate
	ret.Contexts = v.Contexts // no interpolate

	w.workflow = &ret
}

// createChildSession creates a child workflow session
func (w *Session) createChildSession(wf *config.Workflow, msg *dipper.Message) *Session {
	child := w.store.newSession(w.ID)
	child.injectMsg(msg)
	child.initCTX(wf)
	child.injectLocalCTX(wf, msg)
	child.interpolateWorkflow(wf, msg)
	return child
}

// createChildSessionWithName creates a child workflow session
func (w *Session) createChildSessionWithName(name string, msg *dipper.Message) *Session {
	src := w.store.GetConfig().DataSet.Workflows[name]
	wf := &src
	return w.createChildSession(wf, msg)
}
