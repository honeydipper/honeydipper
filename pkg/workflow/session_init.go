// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"golang.org/x/exp/slices"
)

// NewSession creates a new session from the workflow definition.
func NewSession(id string, wf *config.Workflow, store Store) *Session {
	replica := *wf
	w := &Session{
		ID:         id,
		Workflow:   &replica,
		StartTime:  time.Now(),
		store:      store,
		Performing: []string{"initializing"},

		Ctx: map[string]any{
			"_meta_desc": wf.Description,
		},
		IsHook: wf.Context == SessionContextHooks,
	}
	w.threads = &sync.WaitGroup{}

	return w
}

// inheritParentData prepares the session using parent data.
func (w *Session) inherentParentData(parent *Session) {
	w.Event = parent.Event
	w.EventID = parent.EventID
	w.Ctx = dipper.MustDeepCopyMap(parent.Ctx)
	w.Performing = parent.Performing
	w.Performing = append(w.Performing, "initializing")
	w.LoadedContexts = parent.LoadedContexts
	w.depth = parent.depth + 1
	w.IsHook = w.IsHook || parent.IsHook
	w.parent = parent

	delete(w.Ctx, "hooks") // hooks don't get inherited
}

// injectMsg injects the dipper message data into the session as event.
func (w *Session) injectMsg(msg *dipper.Message) {
	w.CurrentMsg = msg
	w.CurrentMsg.Labels["sessionID"] = w.ID
	if w.Event == nil {
		data, _ := dipper.GetMapData(msg.Payload, "data")
		if data != nil {
			w.Event = data.(map[string]interface{})
		} else {
			w.Event = map[string]interface{}{}
		}
		w.EventID = msg.Labels["eventID"]
	}
}

// injectNamedCTX inject a named context into the workflow.
func (w *Session) injectNamedCTX(name string, firstTime bool) {
	namedCTXs, ok := w.store.GetConfig().DataSet.Contexts[name]
	if name[0] != '_' && !ok {
		w.store.GetLogger().Panicf("[workflow] named context %s not defined", name)
	}

	if namedCTXs == nil {
		return
	}

	envData := w.buildEnvData()
	ctx, ok := namedCTXs.(map[string]interface{})["*"]
	if firstTime && ok {
		ctx = dipper.MustDeepCopyMap(ctx.(map[string]interface{}))
		ctx = dipper.Interpolate(ctx, envData)
		w.Ctx = dipper.MergeMap(w.Ctx, ctx)
		w.store.GetLogger().Debugf("session [%s.%s] depth %d merged global values (*) from named context %s to workflow",
			w.ID,
			w.CurrentMsg.Labels["cursor"],
			w.depth,
			name,
		)
	}

	if w.Parent == "" && w.parent == nil {
		ctx, ok := namedCTXs.(map[string]interface{})["_events"]
		if ok {
			ctx = dipper.MustDeepCopyMap(ctx.(map[string]interface{}))
			ctx = dipper.Interpolate(ctx, envData)
			w.Ctx = dipper.MergeMap(w.Ctx, ctx)
		}
	}

	if w.Workflow.Name != "" {
		ctx, ok := namedCTXs.(map[string]interface{})[w.Workflow.Name]
		if ok {
			ctx = dipper.MustDeepCopyMap(ctx.(map[string]interface{}))
			ctx = dipper.Interpolate(ctx, envData)
			w.Ctx = dipper.MergeMap(w.Ctx, ctx)
			w.store.GetLogger().Debugf("session [%s.%s] depth %d merged named context [%s] to [%s]",
				w.ID,
				w.CurrentMsg.Labels["cursor"],
				w.depth,
				name,
				w.Workflow.Name,
			)
		}
	}
}

// injectCTXs loads the contexts specified through context or contexts fields.
func (w *Session) injectCTXs() {
	envdata := w.buildEnvData()
	w.Workflow.Context = dipper.InterpolateStr(w.Workflow.Context, envdata)
	w.Workflow.Contexts = dipper.Interpolate(w.Workflow.Contexts, envdata)

	if w.Workflow.Context != "" {
		if !slices.Contains(w.LoadedContexts, w.Workflow.Context) {
			w.injectNamedCTX(w.Workflow.Context, true)
			w.LoadedContexts = append(w.LoadedContexts, w.Workflow.Context)
		}
	}

	if w.Workflow.Contexts != nil {
		for _, n := range w.Workflow.Contexts.([]interface{}) {
			if n == nil {
				continue
			}
			name, ok := n.(string)
			if !ok {
				panic(fmt.Errorf("%w: expected list of strings in contexts in workflow: %s", ErrWorkflowError, w.Workflow.Name))
			}
			if name == "" {
				continue
			}

			if !slices.Contains(w.LoadedContexts, name) {
				w.injectNamedCTX(name, true)
				w.LoadedContexts = append(w.LoadedContexts, name)
			}
		}
	}
}

// injectEventCTX injects the contextual data from the event into the workflow.
func (w *Session) injectEventCTX(ctx map[string]interface{}) {
	if ctx != nil {
		w.Ctx = dipper.MergeMap(w.Ctx, ctx)
	}
}

// initCTX initialize the contextual data used in this workflow.
func (w *Session) initCTX(eventCtx map[string]interface{}) {
	w.injectNamedCTX(SessionContextDefault, w.parent == nil && w.Parent == "")
	if w.Parent == "" && w.parent == nil {
		w.injectNamedCTX(SessionContextEvents, true)
		if eventCtx != nil {
			w.injectEventCTX(eventCtx)
		}
	}

	for _, name := range w.LoadedContexts {
		w.injectNamedCTX(name, false)
	}

	w.injectCTXs()

	if w.IsHook {
		// avoid hook in hook
		delete(w.Ctx, "hooks")
	}
}

// injects the workflow local context data.
func (w *Session) injectLocalCTX() {
	if w.Workflow.Local != nil && w.Workflow.CallDriver == "" {
		layers, ok := w.Workflow.Local.([]interface{})
		if !ok {
			layers = []interface{}{w.Workflow.Local}
		}
		for _, l := range layers {
			envData := w.buildEnvData()
			locals := dipper.Interpolate(l, envData)
			envData["ctx"] = dipper.MergeMap(envData["ctx"].(map[string]interface{}), locals)
			w.Ctx = envData["ctx"].(map[string]interface{})
		}
	}
}

// interpolateWorkflow creates a copy of the workflow and interpolates it with envData.
func (w *Session) interpolateWorkflow() {
	v := w.Workflow
	envData := w.buildEnvData()
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
	ret.Resume = dipper.InterpolateStr(v.Resume, envData)

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

	w.Workflow = &ret
}

// inheritParentSettings copies some workflow settings from the parent session.
func (w *Session) inherentParentSettings(p *Session) {
	w.ID = p.ID
	if w.Workflow.OnError == "" {
		w.Workflow.OnError = p.Workflow.OnError
	}
	if w.Workflow.OnFailure == "" {
		w.Workflow.OnFailure = p.Workflow.OnFailure
	}
}

// Init initializes the session for execution.
func (w *Session) Init(msg *dipper.Message, parent *Session, ctx map[string]interface{}) {
	if parent != nil {
		w.inherentParentData(parent)
		w.inherentParentSettings(parent)
		w.context, w.cancelFunc = context.WithCancel(parent.context)
	} else {
		w.context, w.cancelFunc = context.WithCancel(context.Background())
	}
	w.injectMsg(msg)
	w.initCTX(ctx)
	w.injectLocalCTX()
	w.interpolateWorkflow()
	w.store.GetLogger().Infof("session [%s.%s] depth %d initialized: %s", w.ID, w.CurrentMsg.Labels["cursor"], w.depth, w.Brief())
}
