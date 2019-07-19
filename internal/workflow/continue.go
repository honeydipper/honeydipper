// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"fmt"
	"sync/atomic"

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

// routeNext determines what to do next for the workflow
func (w *Session) routeNext(msg *dipper.Message) int {
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
	dipper.MergeMap(w.ctx, export)
	dipper.MergeMap(w.exported, export)
}

// processExport export the data into parent workflow session
func (w *Session) processExport(msg *dipper.Message) {
	if w.elseBranch != nil {
		var exports interface{}
		envData := w.buildEnvData(msg)
		status := msg.Labels["status"]

		if w.collapsedFunction != nil && status != SessionStatusError {
			exports = w.collapsedFunction.ExportContext(status, envData)
		}
		if status != SessionStatusError {
			exports = dipper.Interpolate(w.workflow.Export, envData)
		}
		if status == SessionStatusSuccess {
			exports = dipper.Interpolate(w.workflow.ExportOnSuccess, envData)
		}
		if status == SessionStatusFailure {
			exports = dipper.Interpolate(w.workflow.ExportOnFailure, envData)
		}

		if exports != nil {
			w.mergeContext(exports.(map[string]interface{}))
		}
	}
}

// fireCompleteHooks fires all the hooks at completion time asychronously
func (w *Session) fireCompleteHooks(msg *dipper.Message) {
	switch msg.Labels["status"] {
	case SessionStatusError:
		w.fireHook("on_success", msg)
	case SessionStatusFailure:
		w.fireHook("on_failure", msg)
	default:
		w.fireHook("on_success", msg)
	}
	w.fireHook("on_complete", msg)

	if w.parent == "" {
		switch msg.Labels["status"] {
		case SessionStatusError:
			w.fireHook("on_exit_success", msg)
		case SessionStatusFailure:
			w.fireHook("on_exit_failure", msg)
		default:
			w.fireHook("on_exit_success", msg)
		}
		w.fireHook("on_exit", msg)
	}
}

// complete gracefully terminates a session and return exported data to parent
func (w *Session) complete(msg *dipper.Message) {
	if w.ID != "" {
		if _, ok := w.store.sessions[w.ID]; ok {
			dipper.IDMapDel(&w.store.sessions, w.ID)
			w.processExport(msg)

			go w.fireCompleteHooks(msg)

			if w.parent != "" {
				go w.store.ContinueSession(w.parent, msg, w.exported)
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
	}
}

// continueExec resume a session with given dipper message
func (w *Session) continueExec(msg *dipper.Message, export map[string]interface{}) {
	if w.currentHook != "" {
		if msg.Labels["status"] == SessionStatusSuccess {
			switch w.currentHook {
			case "on_session":
				w.execute(w.savedMsg)
			case "on_round":
				w.executeRound(w.savedMsg)
			case "on_item":
				w.executeIteration(w.savedMsg)
			case "on_action":
				w.executeAction(w.savedMsg)
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
			w.complete(&dipper.Message{
				Channel: dipper.ChannelEventbus,
				Subject: dipper.EventbusReturn,
				Labels: map[string]string{
					"status": SessionStatusError,
					"reason": fmt.Sprintf("hook [%s] failed with status '%s' due to: %s", w.currentHook, msg.Labels["status"], msg.Labels["reason"]),
				},
				Payload: map[string]interface{}{},
			})
		}
	} else {
		w.mergeContext(export)
		switch w.routeNext(msg) {
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
