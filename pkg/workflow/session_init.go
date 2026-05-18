// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"golang.org/x/exp/slices"
)

func cloneWorkflow(wf *config.Workflow) *config.Workflow {
	if wf == nil {
		return nil
	}

	buf := dipper.Must(json.Marshal(wf)).([]byte)
	clone := &config.Workflow{}
	dipper.Must(json.Unmarshal(buf, clone))

	return clone
}

// NewSession creates a new session from the workflow definition.
func NewSession(id string, wf *config.Workflow, store Store) *Session {
	runtimeWorkflow := cloneWorkflow(wf)
	if runtimeWorkflow == nil {
		runtimeWorkflow = &config.Workflow{}
	}
	w := &Session{
		ID:               id,
		OriginalWorkflow: cloneWorkflow(wf),
		Workflow:         runtimeWorkflow,
		StartTime:        time.Now(),
		store:            store,
		Performing:       &[]string{"initializing"},

		Ctx: map[string]any{
			"_meta_desc": runtimeWorkflow.Description,
		},
		IsHook: runtimeWorkflow.Context == SessionContextHooks,
	}
	w.threads = &sync.WaitGroup{}

	return w
}

// inheritParentData prepares the session using parent data.
func (w *Session) inherentParentData(parent *Session) {
	w.Event = parent.Event
	w.EventID = parent.EventID
	w.Ctx = dipper.MustDeepCopyMap(parent.Ctx)
	*parent.Performing = append(*parent.Performing, "initializing")
	w.Performing = parent.Performing
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
		w.OriginLabels = make(map[string]string, len(msg.Labels))
		for k, v := range msg.Labels {
			w.OriginLabels[k] = v
		}
	}
	if w.EventID != "" {
		w.CurrentMsg.Labels["eventID"] = w.EventID
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
		ctx = dipper.Interpolate("context", ctx, envData)
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
			ctx = dipper.Interpolate("context", ctx, envData)
			w.Ctx = dipper.MergeMap(w.Ctx, ctx)
		}
	}

	if w.Workflow.Name != "" {
		ctx, ok := namedCTXs.(map[string]interface{})[w.Workflow.Name]
		if ok {
			ctx = dipper.MustDeepCopyMap(ctx.(map[string]interface{}))
			ctx = dipper.Interpolate("context", ctx, envData)
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
	w.Workflow.Context = dipper.InterpolateStr("context", w.Workflow.Context, envdata)
	w.Workflow.Contexts = dipper.Interpolate("context", w.Workflow.Contexts, envdata)

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

// injectRerunCTX injects rerun-specific context values into the workflow context.
func (w *Session) injectRerunCTX(ctx map[string]interface{}) {
	if ctx == nil {
		return
	}

	if w.Ctx == nil {
		w.Ctx = map[string]interface{}{}
	}

	copy := dipper.MustDeepCopyMap(ctx)
	for k, v := range copy {
		w.Ctx[k] = v
	}
}

// initCTX initialize the contextual data used in this workflow.
func (w *Session) initCTX(eventCtx map[string]interface{}, rerunCtx map[string]interface{}) {
	if w.Parent == "" && w.parent == nil && rerunCtx != nil {
		w.injectRerunCTX(rerunCtx)
	}
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
	if w.Workflow.Local != nil {
		layers, ok := w.Workflow.Local.([]interface{})
		if !ok {
			layers = []interface{}{w.Workflow.Local}
		}
		for _, l := range layers {
			envData := w.buildEnvData()
			locals := dipper.Interpolate("context", l, envData)
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

	ret.Name = dipper.InterpolateStr("session", v.Name, envData)
	if v.IterateParallel == nil {
		// for iterate parallel workflow, description is interpolated in each child session to
		// include the current item value. For non-iterate parallel workflow, we can interpolate
		// description here.
		ret.Description = dipper.InterpolateStr("session", v.Description, envData)
	}
	ret.If = dipper.Interpolate("session", v.If, envData).([]string)
	ret.IfAny = dipper.Interpolate("session", v.IfAny, envData).([]string)
	ret.Unless = dipper.Interpolate("session", v.Unless, envData).([]string)
	ret.UnlessAll = dipper.Interpolate("session", v.UnlessAll, envData).([]string)
	ret.Match = dipper.Interpolate("session", v.Match, envData)
	ret.UnlessMatch = dipper.Interpolate("session", v.UnlessMatch, envData)
	ret.Retry = dipper.InterpolateStr("session", v.Retry, envData)
	ret.Backoff = dipper.InterpolateStr("session", v.Backoff, envData)
	ret.Wait = dipper.InterpolateStr("session", v.Wait, envData)
	ret.CallFunction = dipper.InterpolateStr("session", v.CallFunction, envData)
	ret.CallDriver = dipper.InterpolateStr("session", v.CallDriver, envData)
	ret.SendEvent = dipper.Interpolate("session", v.SendEvent, envData)
	ret.Resume = dipper.InterpolateStr("session", v.Resume, envData)
	ret.CacheKey = dipper.InterpolateStr("session", v.CacheKey, envData)
	ret.CacheTTL = dipper.InterpolateStr("session", v.CacheTTL, envData)

	ret.Iterate = dipper.Interpolate("session", v.Iterate, envData)
	if ret.Iterate == nil && v.Iterate != nil {
		ret.Iterate = []interface{}{}
	}
	ret.IterateParallel = dipper.Interpolate("session", v.IterateParallel, envData)
	if ret.IterateParallel == nil && v.IterateParallel != nil {
		ret.IterateParallel = []interface{}{}
	}
	if v.IterateParallel != nil {
		ret.IteratePool = dipper.InterpolateStr("session", v.IteratePool, envData)
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
	// if w.Workflow.OnError == "" {
	// 	w.Workflow.OnError = p.Workflow.OnError
	// }
	// if w.Workflow.OnFailure == "" {
	// 	w.Workflow.OnFailure = p.Workflow.OnFailure
	// }
}

// Init initializes the session for execution.
func (w *Session) Init(
	msg *dipper.Message,
	parent *Session,
	eventCtx map[string]interface{},
	rerunCtx map[string]interface{},
	loadedContexts []string,
) {
	if parent != nil {
		w.inherentParentData(parent)
		w.inherentParentSettings(parent)
		w.context, w.cancelFunc = context.WithCancel(parent.context)
	} else {
		w.context, w.cancelFunc = context.WithCancel(context.Background())
	}
	w.injectMsg(msg)
	if loadedContexts != nil {
		w.LoadedContexts = append([]string(nil), loadedContexts...)
	}
	if len(eventCtx) > 0 {
		w.EventCtx = dipper.MustDeepCopyMap(eventCtx)
	}
	if len(rerunCtx) > 0 {
		w.RerunCtx = dipper.MustDeepCopyMap(rerunCtx)
	}
	w.initCTX(eventCtx, rerunCtx)
	w.injectLocalCTX()
	w.interpolateWorkflow()
	w.brief = "" // refreshing the brief after interpolation
	w.store.GetLogger().Infof("session [%s.%s] depth %d initialized: %s", w.ID, w.CurrentMsg.Labels["cursor"], w.depth, w.Brief())
}
