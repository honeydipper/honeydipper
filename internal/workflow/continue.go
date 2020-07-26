// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"fmt"
	"sync/atomic"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

const (
	// WorkflowNextComplete starts the list of statuses used for determine how to handle a session
	WorkflowNextComplete = iota
	// WorkflowNextStep means the workflow will continue with next step in a multi-step workflow
	WorkflowNextStep
	// WorkflowNextThread means the workflow will continue waiting for the next threa to finish
	WorkflowNextThread
	// WorkflowNextIteration means the workflow will continue process next item in the iteration list
	WorkflowNextIteration
	// WorkflowNextParallelIteration means the workflow will continue waiting for the next item
	// to be processed in the iteration list
	WorkflowNextParallelIteration
	// WorkflowNextRound means the workflow will continue with the next round of the loop
	WorkflowNextRound
)

const (
	// WorkflowHookSuccess is the hook executed when a workflow succeeds.
	WorkflowHookSuccess = "on_success"
	// WorkflowHookFailure is the hook executed when a workflow fails.
	WorkflowHookFailure = "on_failure"
	// WorkflowHookError is the hook executed when a workflow ran into errors.
	WorkflowHookError = "on_error"
	// WorkflowHookExit is the hook executed when a workflow execute.
	WorkflowHookExit = "on_exit"
)

// WorkflowNextStrings are used for logging the routing result
var WorkflowNextStrings = []string{
	"complete",
	"next step",
	"next thread",
	"next item",
	"next item(parallel)",
	"next loop round",
}

// routeNext determines what to do next for the workflow
func (w *Session) routeNext(msg *dipper.Message) int {
	if msg.Labels["status"] == SessionStatusError && w.workflow.OnError != "continue" {
		return WorkflowNextComplete
	}
	if msg.Labels["status"] == SessionStatusFailure && w.workflow.OnFailure == "exit" {
		return WorkflowNextComplete
	}

	switch {
	case w.elseBranch != nil:
		return WorkflowNextComplete
	case len(w.workflow.Steps) > 0 && int(w.current) < len(w.workflow.Steps)-1:
		return WorkflowNextStep
	case len(w.workflow.Threads) > 0 && !(atomic.AddInt32(&w.current, int32(1)) >= int32(len(w.workflow.Threads))):
		return WorkflowNextThread
	}

	if w.isIteration() && int(w.iteration) < w.lenOfIterate()-1 {
		if w.workflow.Iterate != nil {
			return WorkflowNextIteration
		}
		return WorkflowNextParallelIteration
	}

	if w.isLoop() && w.checkLoopCondition(msg) {
		return WorkflowNextRound
	}

	return WorkflowNextComplete
}

// mergeContext merges child workflow exported context to parent workflow
func (w *Session) mergeContext(exports map[string]interface{}) {
	w.ctx = dipper.MergeMap(w.ctx, dipper.MustDeepCopy(exports))
	w.processNoExport(exports)
	w.exported = dipper.MergeMap(w.exported, exports)
}

// processNoExport prevent exporting the data into parent workflow session
func (w *Session) processNoExport(exported map[string]interface{}) {
	for _, key := range w.workflow.NoExport {
		if key == "*" {
			for k := range exported {
				delete(exported, k)
			}
			break
		}
		delete(exported, key)
		delete(exported, key+"-")
		delete(exported, key+"+")
	}
}

// processExport export the data into parent workflow session
func (w *Session) processExport(msg *dipper.Message) {
	if w.elseBranch == nil {
		envData := w.buildEnvData(msg)
		status := msg.Labels["status"]

		if w.inFlyFunction != nil && status != SessionStatusError {
			exports := config.ExportFunctionContext(w.inFlyFunction, envData, w.store.Helper.GetConfig())
			w.mergeContext(exports)
			delete(envData, "sysData")
		}
		if status != SessionStatusError {
			w.postWorkflowExport(w.workflow.Export, envData)
		}
		if status == SessionStatusSuccess {
			w.postWorkflowExport(w.workflow.ExportOnSuccess, envData)
		}
		if status == SessionStatusFailure {
			w.postWorkflowExport(w.workflow.ExportOnFailure, envData)
		}
	}
}

func (w *Session) postWorkflowExport(exportMap map[string]interface{}, envData map[string]interface{}) {
	delta := dipper.Interpolate(exportMap, envData).(map[string]interface{})
	w.mergeContext(delta)
	envData["ctx"] = w.ctx
}

// fireCompleteHooks fires all the hooks at completion time asychronously
func (w *Session) fireCompleteHooks(msg *dipper.Message) {
	defer dipper.SafeExitOnError("session [%s] error on running completion hooks", w.ID)

	// clear other lifecycle hooks
	if w.currentHook != "" && !w.isInCompleteHooks() {
		w.fireHook(w.currentHook, msg)
	}

	if w.currentHook == "" {
		// call conditional completion hook
		var hookName string
		switch msg.Labels["status"] {
		case SessionStatusError:
			hookName = WorkflowHookError
		case SessionStatusFailure:
			hookName = WorkflowHookFailure
		default:
			hookName = WorkflowHookSuccess
		}
		w.fireHook(hookName, msg)
	} else if w.currentHook != WorkflowHookExit {
		// clear conditional completion hook
		w.fireHook(w.currentHook, msg)
	}

	// fire or clear exit hook
	if w.currentHook == "" || w.currentHook == WorkflowHookExit {
		w.fireHook(WorkflowHookExit, msg)
	}
}

// isInCompleteHooks needs to take care of compete hooks carefully to not fall into crash loop
func (w *Session) isInCompleteHooks() bool {
	switch w.currentHook {
	case WorkflowHookError:
		fallthrough
	case WorkflowHookFailure:
		fallthrough
	case WorkflowHookSuccess:
		fallthrough
	case WorkflowHookExit:
		return true
	}
	return false
}

// complete gracefully terminates a session and return exported data to parent
func (w *Session) complete(msg *dipper.Message) {
	w.savedMsg = msg
	if msg.Labels == nil {
		msg.Labels = map[string]string{}
	}
	if msg.Labels["status"] != SessionStatusSuccess && msg.Labels["performing"] == "" {
		msg.Labels["performing"] = w.performing
	}

	dipper.Logger.Infof("[workflow] session [%s] completing with msg labels %+v", w.ID, msg.Labels)
	if w.ID != "" && dipper.IDMapGet(&w.store.sessions, w.ID) != nil {
		if w.currentHook == "" {
			w.processExport(msg)
		}
		w.fireCompleteHooks(msg)
		if w.currentHook != "" {
			return
		}

		dipper.IDMapDel(&w.store.sessions, w.ID)
		if w.parent != "" {
			daemon.Children.Add(1)
			go func() {
				defer daemon.Children.Done()
				w.store.ContinueSession(w.parent, msg, w.exported)
			}()
		}
	}

	if w.ID != "" {
		dipper.Logger.Infof("[workflow] session [%s] completed", w.ID)
		w.ID = ""
	}
	if w.cancelFunc != nil {
		w.cancelFunc()
	}
}

// onError catches any error and complete the session
func (w *Session) onError() {
	if r := recover(); r != nil {
		w.complete(&dipper.Message{
			Channel: dipper.ChannelEventbus,
			Subject: dipper.EventbusReturn,
			Labels: map[string]string{
				"status": SessionStatusError,
				"reason": fmt.Sprintf("%+v", r),
			},
			Payload: map[string]interface{}{},
		})
		panic(r)
	}
}

// continueAfterHook resume a session after finishing a hook
func (w *Session) continueAfterHook(msg *dipper.Message) {
	if msg.Labels["status"] == SessionStatusSuccess {
		switch w.currentHook {
		case "on_session":
			w.execute(w.savedMsg)
		case "on_first_round":
			fallthrough
		case "on_round":
			w.executeRound(w.savedMsg)
		case "on_first_item":
			fallthrough
		case "on_item":
			w.executeIteration(w.savedMsg)
		case "on_first_action":
			fallthrough
		case "on_action":
			w.executeAction(w.savedMsg)
		case WorkflowHookExit:
			fallthrough
		case WorkflowHookFailure:
			fallthrough
		case WorkflowHookSuccess:
			fallthrough
		case WorkflowHookError:
			w.complete(w.savedMsg)
		}
	} else {
		reason := fmt.Sprintf("hook [%s] failed with status '%s' due to: %s", w.currentHook, msg.Labels["status"], msg.Labels["reason"])
		if !w.isInCompleteHooks() {
			// clear the hook flag start completion process
			w.currentHook = ""
		}
		w.complete(&dipper.Message{
			Channel: dipper.ChannelEventbus,
			Subject: dipper.EventbusReturn,
			Labels: map[string]string{
				"status": SessionStatusError,
				"reason": reason,
			},
			Payload: map[string]interface{}{},
		})
	}
}

// continueExec resume a session with given dipper message
func (w *Session) continueExec(msg *dipper.Message, exports map[string]interface{}) {
	w.mergeContext(exports)
	if w.currentHook != "" {
		w.continueAfterHook(msg)
		return
	}
	route := w.routeNext(msg)
	dipper.Logger.Debugf("[workflow] session [%s] routing with '%s'", w.ID, WorkflowNextStrings[route])
	switch route {
	case WorkflowNextStep:
		w.current++
		w.executeStep(msg)
	case WorkflowNextThread:
	case WorkflowNextIteration:
		w.iteration++
		w.executeIteration(msg)
	case WorkflowNextParallelIteration:
		atomic.AddInt32(&w.iteration, 1)
	case WorkflowNextRound:
		w.loopCount++
		w.executeRound(msg)
	case WorkflowNextComplete:
		w.complete(msg)
	}
}
