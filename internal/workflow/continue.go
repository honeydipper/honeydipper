// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"fmt"
	"sync/atomic"

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
	case len(w.workflow.Threads) > 0 && int(w.current) < len(w.workflow.Threads)-1:
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
func (w *Session) mergeContext(export map[string]interface{}) {
	w.ctx = dipper.MergeMap(w.ctx, export)
	w.exported = dipper.CombineMap(w.exported, export)
}

// processExport export the data into parent workflow session
func (w *Session) processExport(msg *dipper.Message) {
	if w.elseBranch == nil {
		var exports map[string]interface{}
		envData := w.buildEnvData(msg)
		status := msg.Labels["status"]

		if w.collapsedFunction != nil && status != SessionStatusError {
			exports, w.ctx = w.collapsedFunction.ExportContext(status, envData)
			envData["ctx"] = w.ctx
		}
		if status != SessionStatusError {
			delta := dipper.Interpolate(w.workflow.Export, envData)
			exports = dipper.MergeMap(exports, delta)
			w.ctx = dipper.MergeMap(w.ctx, delta)
			envData["ctx"] = w.ctx
		}
		if status == SessionStatusSuccess {
			delta := dipper.Interpolate(w.workflow.ExportOnSuccess, envData)
			exports = dipper.MergeMap(exports, delta)
			w.ctx = dipper.MergeMap(w.ctx, delta)
			envData["ctx"] = w.ctx
		}
		if status == SessionStatusFailure {
			delta := dipper.Interpolate(w.workflow.ExportOnFailure, envData)
			exports = dipper.MergeMap(exports, delta)
			w.ctx = dipper.MergeMap(w.ctx, delta)
			envData["ctx"] = w.ctx
		}

		if exports != nil {
			w.exported = dipper.MergeMap(w.exported, exports)
		}

		for _, key := range w.workflow.NoExport {
			if key == "*" {
				w.exported = nil
				break
			}
			delete(w.exported, key)
			delete(w.exported, key+"-")
			delete(w.exported, key+"+")
		}
	}
}

// fireCompleteHooks fires all the hooks at completion time asychronously
func (w *Session) fireCompleteHooks(msg *dipper.Message) {
	defer dipper.SafeExitOnError("session [%s] error on running completion hooks", w.ID)
	var hookName string
	switch msg.Labels["status"] {
	case SessionStatusError:
		hookName = "on_error"
	case SessionStatusFailure:
		hookName = "on_failure"
	default:
		hookName = "on_success"
	}

	if w.currentHook == "" || w.currentHook == hookName {
		w.fireHook(hookName, msg)
	}
	if w.currentHook == "" || w.currentHook == "on_exit" {
		w.fireHook("on_exit", msg)
	}
}

// complete gracefully terminates a session and return exported data to parent
func (w *Session) complete(msg *dipper.Message) {
	if msg.Labels == nil {
		msg.Labels = map[string]string{}
	}
	if msg.Labels["status"] != SessionStatusSuccess && msg.Labels["performing"] == "" {
		msg.Labels["performing"] = w.performing
	}
	dipper.Logger.Debugf("[workflow] session [%s] completing with msg labels %+v", w.ID, msg.Labels)
	if w.ID != "" {
		if _, ok := w.store.sessions[w.ID]; ok {
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
		dipper.Logger.Infof("[workflow] session [%s] completed", w.ID)
		w.ID = ""
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

// continueExec resume a session with given dipper message
func (w *Session) continueExec(msg *dipper.Message, export map[string]interface{}) {
	w.mergeContext(export)
	if w.currentHook != "" {
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
			case "on_exit":
				fallthrough
			case "on_failure":
				fallthrough
			case "on_success":
				fallthrough
			case "on_error":
				w.complete(w.savedMsg)
			default:
				w.complete(&dipper.Message{
					Channel: dipper.ChannelEventbus,
					Subject: dipper.EventbusReturn,
					Labels: map[string]string{
						"status": SessionStatusError,
						"reason": fmt.Sprintf("unknown hook [%s] for session: %s", w.currentHook, w.ID),
					},
					Payload: map[string]interface{}{},
				})
			}
		} else {
			reason := fmt.Sprintf("hook [%s] failed with status '%s' due to: %s", w.currentHook, msg.Labels["status"], msg.Labels["reason"])
			w.currentHook = ""
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
	} else {
		route := w.routeNext(msg)
		dipper.Logger.Debugf("[workflow] session [%s] routing with '%s'", w.ID, WorkflowNextStrings[route])
		switch route {
		case WorkflowNextStep:
			w.current++
			w.executeStep(msg)
		case WorkflowNextThread:
			atomic.AddInt32(&w.current, 1)
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
}
