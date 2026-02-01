// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package workflow implements workflow execution and state management.
package workflow

import (
	"fmt"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/v3/internal/daemon"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

// Session state constants representing different stages of workflow execution.
const (
	// SessionStateInit represents the initial state of a workflow session.
	SessionStateInit = iota
	// SessionStateCheckCondition means the session is checking the conditions.
	SessionStateCheckCondition
	// SessionCheckLoopCondition means the session is checking the loop condition.
	SessionStateCheckLoopCondition
	// SessionStateFirstRound means the session is starting the first round of the loop.
	SessionStateFirstRound
	// SessionStateNextRound means the session is starting the next round of the loop.
	SessionStateNextRound
	// SessionStateElse represents the state when condition is not met, the alternative branch is being executed.
	SessionStateElse
	// SessionStateCheckIteration means the session is checking the iteration.
	SessionStateCheckIteration
	// SessionStateFirstItem means the session is starting the first item of the iteration.
	SessionStateFirstItem
	// SessionStateNextItem means the session is starting the next item of the iteration.
	SessionStateNextItem
	// SessionStateCheckSteps means the session is checking if there are steps to execute.
	SessionStateCheckSteps
	// SessionStateFirstStep means the session is starting the first step of the iteration.
	SessionStateFirstStep
	// SessionStateNextStep means the session is starting the next step of the iteration.
	SessionStateNextStep
	// SessionStateCheckAction represents the state when the session is checking if there is an action to execute.
	SessionStateCheckAction
	// SessionStateFirstAction represents the state when the session is executing the first action.
	SessionStateFirstAction
	// SessionStateAction represents the state when the session is executing an action.
	SessionStateAction
	// SessionStateUpdate represents the state when the session is updating its data based on round and iteration.
	SessionStateUpdate
	// SessionStateCheckEndSteps represents the state when the session is checking if all steps are completed.
	SessionStateCheckEndSteps
	// SessionStateEndFirstStep represents the state when the session is ending the first step.
	SessionStateEndFirstStep
	// SessionStateEndStep represents the state when the session is ending the current step.
	SessionStateEndStep
	// SessionStateCheckEndIterations represents the state when the session is checking if all iterations are completed.
	SessionStateCheckEndIterations
	// SessionStateEndFirstItem represents the state when the session is ending the first iteration item.
	SessionStateEndFirstItem
	// SessionStateEndItem represents the state when the session is ending the current iteration item.
	SessionStateEndItem
	// SessionStateCheckEndRounds represents the state when the session is checking if all rounds are completed.
	SessionStateCheckEndRounds
	// SessionStateEndFirstRound represents the state when the session is ending the first round.
	SessionStateEndFirstRound
	// SessionStateEndRound represents the state when the session is ending the current round.
	SessionStateEndRound
	// SessionStateExport represents the state when the session is injecting exported data into context.
	SessionStateExport
	// SessionStateFailure represents the state when the session has completed with failure.
	SessionStateFailure
	// SessionStateError represents the state when the session has completed with error.
	SessionStateError
	// SessionStateSuccess represents the state when the session has completed successfully.
	SessionStateSuccess
	// SessionStateDone represents the state when the session has fully completed.
	SessionStateDone
)

// SessionStates maps state constants to human-readable names for logging purposes.
var SessionStates = []string{
	"init",
	"check-condition",
	"else",
	"check-loop-condition",
	"first-round",
	"next-round",
	"check-iteration",
	"first-item",
	"next-item",
	"check-steps",
	"first-step",
	"next-step",
	"check-action",
	"first-action",
	"action",
	"update",
	"check-end-steps",
	"end-first-step",
	"end-step",
	"check-end-iterations",
	"end-first-item",
	"end-item",
	"check-end-rounds",
	"end-first-round",
	"end-round",
	"export",
	"failure",
	"error",
	"success",
	"done",
}

// activateChild activates and waits for a child session to complete, then injects its results.
func (w *Session) activateChild() {
	w.child.activate()
	w.child.Wait()
	if w.child.pending || w.child.CurrentHook != "" {
		return
	}
	if w.CurrentHook == "" {
		w.injectMsg(w.child.CurrentMsg)
		if _, ok := w.CurrentMsg.Labels["status"]; !ok {
			w.CurrentMsg.Labels["status"] = SessionStatusSuccess
		}
	}
}

// activate initializes a new session or reactivates an existing session to
// progress based on its current state. This runs in a separate goroutine.
func (w *Session) activate() {
	w.threads.Add(1)
	go func() {
		defer w.threads.Done()
		defer dipper.SafeExitOnError("[%s] panic progressing through state [%s]", w.ID, SessionStates[w.State], func(r any) {
			w.pending = false
			w.CurrentMsg.Labels["status"] = SessionStatusError
			w.CurrentMsg.Labels["reason"] = fmt.Sprintf("panic progressing through state [%s]: %v", SessionStates[w.State], r)
			w.CurrentMsg.Labels["performing"] = strings.Join(w.Performing, "\n")
			w.CurrentHook = ""

			w.State = SessionStateExport
			defer dipper.SafeExitOnError("[%s] panic exporting error", w.ID)
			w.progress()
		})

		if w.child != nil && (w.child.pending || w.child.CurrentHook != "") {
			w.activateChild()
			if w.child.pending || w.child.CurrentHook != "" {
				w.pending = true

				return
			}
		}

		w.progress()
	}()
}

// progress operates the session based on its current state then transitions the state forward.
// It handles hooks, state transitions, and execution of workflow operations.
func (w *Session) progress() {
	if status := w.CurrentMsg.Labels["status"]; status != SessionStatusSuccess && status != "" && w.CurrentMsg.Labels["performing"] == "" {
		w.CurrentMsg.Labels["performing"] = strings.Join(w.Performing, "\n")
	}
	isEntryHook := w.fireOrClearHook(true) // Fire or clear entry hooks for current state.
	switch {
	case w.CurrentHook != "" && isEntryHook:
		return
	case w.CurrentHook != "" && !isEntryHook:
		fallthrough
	case w.pending:
		w.resume()

		return
	}

	w.store.GetLogger().Debugf(
		"session [%s.%s] depth %d progressing through state %s",
		w.ID,
		w.CurrentMsg.Labels["cursor"],
		w.depth,
		SessionStates[w.State],
	)
	switch w.State {
	case SessionStateElse:
		w.processElseState()
	case SessionStateNextItem:
		w.setPerforming("processing next iteration item")
		if w.Workflow.Iterate != nil {
			w.Ctx["current"] = w.Workflow.Iterate.([]any)[w.Iteration]
			if w.Workflow.IterateAs != "" {
				w.Ctx[w.Workflow.IterateAs] = w.Ctx["current"]
			}
		}
	case SessionStateNextRound:
		w.setPerforming("processing next round of loop")
		w.Ctx["loop_count"] = w.LoopCount
	case SessionStateAction:
		w.setPerforming("executing actions")
		if len(w.Workflow.Threads) > 0 && w.Current > 0 {
			w.pending = true

			break
		}
		w.execute()
		w.store.GetLogger().Debugf("[%s.%d] action execution launched with status", w.ID, w.depth)
	case SessionStateUpdate:
		w.processUpdateState()
	case SessionStateExport:
		w.setPerforming("exporting workflow data to context")

		if w.ElseBranch != nil {
			break
		}

		w.processWorkflowExport()
	default:
		w.setPerforming("processing state: " + SessionStates[w.State])
	}

	if w.pending {
		w.store.GetLogger().Debugf("session [%s.%s] depth %d pending in state [%s]",
			w.ID,
			w.CurrentMsg.Labels["cursor"],
			w.depth,
			SessionStates[w.State],
		)
	} else {
		w.resume()
	}
}

// processElseBranch handles the else branch of a conditional workflow.
func (w *Session) processElseState() {
	w.setPerforming("executing else branch")
	dipper.Must(mapstructure.Decode(w.Workflow.Else, &w.ElseBranch))
	w.child = w.store.CreateChildSession(w, w.ElseBranch, w.CurrentMsg)
	w.store.ActivateSession(w.child)

	w.child.Wait()

	if w.child.pending || w.child.CurrentHook != "" {
		w.pending = true
	}
}

// processUpdateState updates the session's context with exported data from the child session.
func (w *Session) processUpdateState() {
	w.setPerforming("updating workflow context with child exported data")
	if w.child == nil {
		if w.checkIsNoop() {
			w.CurrentMsg.Labels["status"] = SessionStatusSuccess
		}

		return
	}

	for _, e := range w.child.Exported {
		if w.ElseBranch == nil {
			w.Ctx = dipper.MergeMap(w.Ctx, e)
		}
		w.processNoExport(e)
		if len(e) > 0 {
			w.Exported = append(w.Exported, e)
		}
	}
	w.child = nil
}

// resume transitions the session to the next state based on its current state.
// It handles hook firing and clearing before state transition.
func (w *Session) resume() {
	w.store.GetLogger().Debugf("session [%s.%s] depth %d resuming after state [%s]",
		w.ID,
		w.CurrentMsg.Labels["cursor"],
		w.depth,
		SessionStates[w.State],
	)
	w.pending = false
	w.fireOrClearHook(false) // Fire or clear exit hooks for current state.
	if w.CurrentHook != "" {
		return
	}

	w.State = w.determineNextState()

	if w.State != SessionStateDone {
		w.activate()

		return
	}

	w.store.GetLogger().Debugf("session [%s.%s] depth %d done",
		w.ID,
		w.CurrentMsg.Labels["cursor"],
		w.depth,
	)

	if w.parent != nil {
		return
	}

	w.CompletionTime = time.Now()
	w.CurrentMsg.Labels["start"] = w.StartTime.Format(time.RFC3339Nano)
	w.CurrentMsg.Labels["end"] = w.CompletionTime.Format(time.RFC3339Nano)

	if w.Parent != "" {
		w.store.GetLogger().Infof("session [%s.%s] depth %d return to parent [%s]",
			w.ID,
			w.CurrentMsg.Labels["cursor"],
			w.depth,
			w.Parent,
		)
		pos := strings.Index(w.Parent, ".")
		if pos <= 0 {
			w.store.GetLogger().Panicf("[%s.%s] invalid parent format [%s]",
				w.ID,
				w.CurrentMsg.Labels["cursor"],
				w.Parent,
			)
		}
		parentID := w.Parent[:pos]
		msg := *w.CurrentMsg
		msg.Labels = map[string]string{}
		for k, v := range w.CurrentMsg.Labels {
			msg.Labels[k] = v
		}
		msg.Labels["cursor"] = w.Parent[pos+1:]
		daemon.Go(func() {
			w.store.ContinueSession(parentID, &msg)
		})
	} else {
		daemon.Go(func() {
			w.store.EmitResult(w)
		})
	}
}
