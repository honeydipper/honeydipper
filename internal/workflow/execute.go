// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"reflect"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

// execute is the entry point of the workflow
func (w *Session) execute(msg *dipper.Message) {
	w.fireHook("on_session", msg)
	if w.currentHook == "" {
		switch {
		case w.checkCondition() && w.checkLoopCondition(msg):
			if !w.isIteration() || w.lenOfIterate() > 0 {
				w.loopCount = 0
				if w.ID == "" {
					daemon.Children.Add(1)
					go func() {
						defer daemon.Children.Done()
						defer dipper.SafeExitOnError("Failed in execute %+v", *w.workflow)
						w.save()
						defer w.onError()
						w.executeRound(msg)
					}()
				} else {
					w.executeRound(msg)
				}
			} else if w.parent != "" {
				w.noop(msg)
			}
		case w.workflow.Else != nil:
			var elseBranch config.Workflow
			err := mapstructure.Decode(w.workflow.Else, &elseBranch)
			if err != nil {
				panic(err)
			}
			w.elseBranch = &elseBranch
			daemon.Children.Add(1)
			go func() {
				defer daemon.Children.Done()
				defer dipper.SafeExitOnError("Failed in execute else branch %+v", elseBranch)
				w.save()
				defer w.onError()
				child := w.createChildSession(w.elseBranch, msg)
				child.execute(msg)
			}()
		case w.parent != "":
			w.noop(msg)
		}
	}
}

// noop continues the workflow as doing nothing
func (w *Session) noop(msg *dipper.Message) {
	if msg.Labels["status"] != "success" {
		msg = &dipper.Message{
			Labels: map[string]string{
				"status": SessionStatusSuccess,
			},
		}
	}
	if w.ID != "" {
		w.continueExec(msg, nil)
	} else {
		daemon.Children.Add(1)
		go func() {
			defer daemon.Children.Done()
			w.store.ContinueSession(w.parent, msg, nil)
		}()
	}
}

// executeRound takes actions for a single round of a loop
func (w *Session) executeRound(msg *dipper.Message) {
	if w.isLoop() {
		w.ctx["loop_count"] = w.loopCount
		if w.loopCount == 0 {
			w.fireHook("on_first_round", msg)
			if w.currentHook != "" {
				return
			}
		}
		w.fireHook("on_round", msg)
		if w.currentHook != "" {
			return
		}
		w.iteration = 0
		w.current = 0
	}

	if _, ok := w.ctx["resume_token"]; !ok {
		if w.ctx == nil {
			w.ctx = map[string]interface{}{}
		}
		w.ctx["resume_token"] = "/" + w.workflow.Name + "/" + w.ID
	}

	w.executeIteration(msg)
}

// executeIteration takes actions for items in iteration list
func (w *Session) executeIteration(msg *dipper.Message) {
	if w.isIteration() {
		w.current = 0
		if w.workflow.Iterate != nil {
			w.ctx["current"] = reflect.ValueOf(w.workflow.Iterate).Index(int(w.iteration)).Interface()
			if w.workflow.IterateAs != "" {
				w.ctx[w.workflow.IterateAs] = w.ctx["current"]
			}
			if w.loopCount == 0 && int(w.iteration) == 0 {
				w.fireHook("on_first_item", msg)
				if w.currentHook != "" {
					return
				}
			}
			w.fireHook("on_item", msg)
			if w.currentHook == "" {
				w.executeAction(msg)
			}
		} else {
			iter := reflect.ValueOf(w.workflow.IterateParallel)
			l := iter.Len()
			single := config.Workflow{
				Workflow:     w.workflow.Workflow,
				Function:     w.workflow.Function,
				CallFunction: w.workflow.CallFunction,
				CallDriver:   w.workflow.CallDriver,
				Switch:       w.workflow.Switch,
				Cases:        w.workflow.Cases,
				Default:      w.workflow.Default,
				Steps:        w.workflow.Steps,
				Threads:      w.workflow.Threads,
			}
			for i := 0; i < l; i++ {
				child := w.createChildSession(&single, msg)
				child.ctx["current"] = iter.Index(i).Interface()
				if w.workflow.IterateAs != "" {
					child.ctx[w.workflow.IterateAs] = child.ctx["current"]
				}
				delete(child.ctx, "resume_token")

				daemon.Children.Add(1)
				go func(child *Session) {
					defer daemon.Children.Done()
					defer dipper.SafeExitOnError("Failed in execute child thread with %+v", single)
					defer w.onError()
					child.execute(msg)
				}(child)
			}
		}
	} else {
		w.executeAction(msg)
	}
}

// startWait puts a session into waiting state
func (w *Session) startWait() {
	resumeToken, ok := w.ctx["resume_token"].(string)
	if !ok || resumeToken == "" {
		dipper.Logger.Panicf("[workflow] wait identifier missing for session %s", w.ID)
	}
	oldWaiterSession, ok := w.store.suspendedSessions[resumeToken]
	if ok {
		dipper.Logger.Panicf("[workflow] wait identifier collided for sessions %s and %s", w.ID, oldWaiterSession)
	}
	w.store.suspendedSessions[resumeToken] = w.ID

	if strings.ToLower(w.workflow.Wait) != "infinite" {
		d, err := time.ParseDuration(w.workflow.Wait)
		if err != nil {
			dipper.Logger.Panicf("[workflow] fail to time.ParseDuration '%s' for %+v", w.workflow.Wait, resumeToken)
		}
		daemon.Children.Add(1)
		go func() {
			defer daemon.Children.Done()
			defer dipper.SafeExitOnError("[workflow] resuming session on timeout failed %+v", resumeToken)

			timeoutStatus, _ := dipper.GetMapDataStr(w.ctx, "timeout_status")
			reason := "timeout"
			if timeoutStatus == "" {
				timeoutStatus = SessionStatusSuccess
				reason = ""
			}
			timeoutPayload := w.ctx["return_on_timeout"]

			<-time.After(d)
			dipper.Logger.Infof("[workflow] resuming session on timeout %+v", resumeToken)

			daemon.Children.Add(1)
			go func() {
				defer daemon.Children.Done()
				w.store.ResumeSession(resumeToken, &dipper.Message{
					Payload: map[string]interface{}{
						"key": resumeToken,
						"labels": map[string]interface{}{
							"status": timeoutStatus,
							"reason": reason,
						},
						"payload": timeoutPayload,
					},
				})
			}()
		}()
	}
}

// executeSwitch will select branch to execute based on the given string
func (w *Session) executeSwitch(msg *dipper.Message) {
	envData := w.buildEnvData(msg)
	match := dipper.InterpolateStr(w.workflow.Switch, envData)
	for key, branch := range w.workflow.Cases {
		if key == match {
			w.performing = "switch branch " + key
			wf := &config.Workflow{}
			err := mapstructure.Decode(branch, wf)
			if err != nil {
				panic(err)
			}
			child := w.createChildSession(wf, msg)
			child.execute(msg)
			return
		}
	}
	if w.workflow.Default != nil {
		w.performing = "switch default branch"
		var defaultBranch config.Workflow
		err := mapstructure.Decode(w.workflow.Default, &defaultBranch)
		if err != nil {
			panic(err)
		}
		child := w.createChildSession(&defaultBranch, msg)
		child.execute(msg)
		return
	}
	w.noop(msg)
}

// executeAction takes actions for a single iteration in a single loop round
func (w *Session) executeAction(msg *dipper.Message) {
	switch {
	case w.workflow.Workflow != "":
		fallthrough
	case w.isFunction():
		fallthrough
	case w.workflow.CallDriver != "":
		fallthrough
	case w.workflow.Steps != nil:
		fallthrough
	case w.workflow.Threads != nil:
		fallthrough
	case w.workflow.Wait != "":
		fallthrough
	case w.workflow.Switch != "":
		if w.loopCount == 0 && int(w.iteration) == 0 && int(w.current) == 0 {
			w.fireHook("on_first_action", msg)
		}
		if w.currentHook == "" {
			w.fireHook("on_action", msg)
		}
		if w.currentHook != "" {
			return
		}
	}
	// no action hooks if workflow is noop

	switch {
	case w.workflow.Workflow != "":
		envData := w.buildEnvData(msg)
		work := dipper.InterpolateStr(w.workflow.Workflow, envData)
		w.performing = work
		if !w.isHook && w.workflow.Name == "" {
			w.ctx["_meta_name"] = work
		}
		child := w.createChildSessionWithName(work, msg)
		child.execute(msg)
	case w.isFunction():
		w.performing = "function"
		f := w.interpolateFunction(&w.workflow.Function, msg)
		w.callFunction(f, msg)
	case w.workflow.CallDriver != "":
		w.performing = "driver " + w.workflow.CallDriver
		w.callDriver(w.workflow.CallDriver, msg)
	case w.workflow.CallFunction != "":
		w.performing = "function " + w.workflow.CallFunction
		w.callShorthandFunction(w.workflow.CallFunction, msg)
	case w.workflow.Steps != nil:
		w.performing = "steps"
		w.current = 0
		w.executeStep(msg)
	case w.workflow.Threads != nil:
		w.performing = "threads"
		w.current = 0
		w.executeThreads(msg)
	case w.workflow.Wait != "":
		w.performing = "suspending"
		w.startWait()
	case w.workflow.Switch != "":
		w.performing = "switch"
		w.executeSwitch(msg)
	default:
		w.continueExec(&dipper.Message{
			Labels: map[string]string{
				"status": SessionStatusSuccess,
			},
		}, nil)
	}
}

// interpolateFunction interplotes the system name and function names in the target
func (w *Session) interpolateFunction(f *config.Function, msg *dipper.Message) *config.Function {
	envData := w.buildEnvData(msg)
	interpolatedFunc := *f
	interpolatedFunc.Target.System = dipper.InterpolateStr(f.Target.System, envData)
	interpolatedFunc.Target.Function = dipper.InterpolateStr(f.Target.Function, envData)

	return &interpolatedFunc
}

// callDriver makes a call to a driver function defined in short hand fashion
func (w *Session) callDriver(f string, msg *dipper.Message) {
	interpolatedNames := strings.Split(f, ".")
	driverName, rawActionName := interpolatedNames[0], interpolatedNames[1]

	var locals map[string]interface{}
	if w.workflow.Local != nil {
		envData := w.buildEnvData(msg)
		locals = dipper.Interpolate(w.workflow.Local, envData).(map[string]interface{})
	}

	w.callFunction(&config.Function{
		Driver:     driverName,
		RawAction:  rawActionName,
		Parameters: locals,
	}, msg)
}

// callShorthandFunction makes a call to a function defined in short hand fashion
func (w *Session) callShorthandFunction(f string, msg *dipper.Message) {
	interpolatedNames := strings.SplitN(f, ".", 2)
	systemName, funcName := interpolatedNames[0], interpolatedNames[1]

	w.callFunction(&config.Function{
		Target: config.Action{
			System:   systemName,
			Function: funcName,
		},
	}, msg)
}

// callFunction makes a call to a function
func (w *Session) callFunction(f *config.Function, msg *dipper.Message) {
	// stored for doing export context later
	w.inFlyFunction = f

	payload := w.buildEnvData(msg)
	payload["function"] = *f

	labels := map[string]string{}
	for k, v := range msg.Labels {
		labels[k] = v
	}
	delete(labels, "status")
	delete(labels, "reason")
	delete(labels, "performing")
	labels["sessionID"] = w.ID

	cmdmsg := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "command",
		Payload: payload,
		Labels:  labels,
	}

	w.store.Helper.SendMessage(cmdmsg)
}

// executeStep run a step in a workflow
func (w *Session) executeStep(msg *dipper.Message) {
	wf := w.workflow.Steps[w.current]
	child := w.createChildSession(&wf, msg)
	child.ctx["step_number"] = w.current
	child.execute(msg)
}

// executeThreads start all threads of the workflow
func (w *Session) executeThreads(msg *dipper.Message) {
	for i := range w.workflow.Threads {
		daemon.Children.Add(1)
		go func(i int) {
			defer daemon.Children.Done()
			defer dipper.SafeExitOnError("Failed in execute child thread with %+v", w.workflow.Threads[i])
			defer w.onError()
			child := w.createChildSession(&w.workflow.Threads[i], msg)
			child.ctx["thread_number"] = i
			delete(child.ctx, "resume_token")
			child.execute(msg)
		}(i)
	}
}
