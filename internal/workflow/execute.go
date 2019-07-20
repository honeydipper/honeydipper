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
		if w.checkCondition() && w.checkLoopCondition(msg) {
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
			}
		} else if w.workflow.Else != nil {
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
		}
	}
}

// executeRound takes actions for a single round of a loop
func (w *Session) executeRound(msg *dipper.Message) {
	if w.isLoop() {
		w.ctx["loop_count"] = w.loopCount
		w.fireHook("on_round", msg)
		if w.currentHook != "" {
			return
		}
		w.iteration = 0
		w.current = 0
	}

	if _, ok := w.ctx["waiter"]; !ok {
		w.ctx["waiter"] = "/" + w.workflow.Name + "/" + w.ID
	}

	w.executeIteration(msg)
}

// executeIteration takes actions for items in iteration list
func (w *Session) executeIteration(msg *dipper.Message) {
	if w.isIteration() {
		w.current = 0
		if w.workflow.Iterate != nil {
			w.ctx["current"] = reflect.ValueOf(w.workflow.Iterate).Index(int(w.iteration)).Interface()
			w.fireHook("on_item", msg)
			if w.currentHook == "" {
				w.executeAction(msg)
			}
		} else {
			iter := reflect.ValueOf(w.workflow.IterateParallel)
			l := iter.Len()
			single := config.Workflow{
				Workflow: w.workflow.Workflow,
				Function: w.workflow.Function,
				Steps:    w.workflow.Steps,
				Threads:  w.workflow.Threads,
			}
			for i := 0; i < l; i++ {
				child := w.createChildSession(&single, msg)
				child.ctx["current"] = iter.Index(i).Interface()
				child.ctx["waiter"] = ""
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
	waiter, ok := w.ctx["waiter"]
	if !ok || waiter == "" {
		dipper.Logger.Panicf("[workflow] wait identifier missing for session %s", w.ID)
	}
	oldWaiterSession, ok := w.store.suspendedSessions[waiter.(string)]
	if ok {
		dipper.Logger.Panicf("[workflow] wait identifier collided for sessions %s and %s", w.ID, oldWaiterSession)
	}
	w.store.suspendedSessions[waiter.(string)] = w.ID

	if strings.ToLower(w.workflow.Wait) != "infinite" {
		d, err := time.ParseDuration(w.workflow.Wait)
		if err != nil {
			dipper.Logger.Panicf("[workflow] fail to time.ParseDuration '%s' for %+v", w.workflow.Wait, waiter)
		}
		daemon.Children.Add(1)
		go func() {
			defer daemon.Children.Done()
			defer dipper.SafeExitOnError("[workflow] resuming session on timeout failed %+v", waiter)

			timeoutStatus, _ := dipper.GetMapDataStr(w.ctx, "timeout_status")
			reason := "time out"
			if timeoutStatus == "" {
				timeoutStatus = SessionStatusSuccess
				reason = ""
			}
			timeoutPayload := w.ctx["return_on_timeout"]

			<-time.After(d)
			dipper.Logger.Infof("[workflow] resuming session on timeout %+v", waiter)

			w.store.ResumeSession(&dipper.Message{
				Payload: map[string]interface{}{
					"key": waiter,
					"labels": map[string]interface{}{
						"status": timeoutStatus,
						"reason": reason,
					},
					"payload": timeoutPayload,
				},
			})
		}()
	}
}

// executeSwitch will select branch to execute based on the given string
func (w *Session) executeSwitch(msg *dipper.Message) {
	envData := w.buildEnvData(msg)
	match := dipper.InterpolateStr(w.workflow.Switch, envData)
	for key, branch := range w.workflow.Cases {
		if key == match {
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
		var defaultBranch config.Workflow
		err := mapstructure.Decode(w.workflow.Default, &defaultBranch)
		if err != nil {
			panic(err)
		}
		child := w.createChildSession(&defaultBranch, msg)
		child.execute(msg)
		return
	}
	w.continueExec(msg, nil)
}

// executeAction takes actions for a single iteration in a single loop round
func (w *Session) executeAction(msg *dipper.Message) {
	w.fireHook("on_action", msg)
	if w.currentHook == "" {
		switch {
		case w.workflow.Workflow != "":
			child := w.createChildSessionWithName(w.workflow.Workflow, msg)
			child.execute(msg)
		case w.isFunction():
			w.callFunction(&w.workflow.Function, msg)
		case w.workflow.Steps != nil:
			w.current = 0
			w.executeStep(msg)
		case w.workflow.Threads != nil:
			w.current = 0
			w.executeThreads(msg)
		case w.workflow.Wait != "":
			w.startWait()
		case w.workflow.Switch != "":
			w.executeSwitch(msg)
		default:
			w.continueExec(msg, nil)
		}
	}
}

// callFunction makes a call to a function
func (w *Session) callFunction(f *config.Function, msg *dipper.Message) {

	// stored for doing export context later
	w.collapsedFunction = config.CollapseFunction(nil, f, w.store.GetConfig())

	payload := w.buildEnvData(msg)
	payload["function"] = *f

	labels := msg.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labels["sessionID"] = w.ID

	cmdmsg := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "Command",
		Payload: payload,
		Labels:  labels,
	}

	w.store.SendMessage(cmdmsg)
}

// executeStep run a step in a workflow
func (w *Session) executeStep(msg *dipper.Message) {
	wf := w.workflow.Steps[w.current]
	child := w.createChildSession(&wf, msg)
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
			child.ctx["waiter"] = ""
			child.execute(msg)
		}(i)
	}
}
