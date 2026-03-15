// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package workflow implements workflow execution and state management.
package workflow

// determineNextState calculates the next state based on current state and conditions.
// It routes the state transition through appropriate handler functions.
func (w *Session) determineNextState() int {
	switch w.State {
	case SessionStateCheckCondition:
		return w.routeCheckConditionState()
	case SessionStateCheckCache:
		return w.routeCheckCacheState()
	case SessionStateCheckLoopCondition:
		return w.routeCheckLoopConditionState()
	case SessionStateElse:
		return SessionStateUpdate
	case SessionStateCheckIteration:
		return w.routeCheckIterationState()
	case SessionStateNextItem:
		return w.routNextItemState()
	case SessionStateCheckSteps:
		return w.routeCheckStepsState()
	case SessionStateCheckAction:
		return w.routeCheckActionState()
	case SessionStateUpdate:
		return w.routeUpdateState()
	case SessionStateCheckEndSteps:
		return w.routeCheckEndStepsState()
	case SessionStateEndStep:
		return w.routeEndStepState()
	case SessionStateCheckEndIterations:
		return w.routeCheckEndIterationsState()
	case SessionStateEndItem:
		return w.routeEndItemState()
	case SessionStateCheckEndRounds:
		return w.routeCheckEndRoundsState()
	case SessionStateEndRound:
		return w.routeEndRoundState()
	case SessionStateExport:
		return w.routeExportState()
	case SessionStateSaveCache:
		return SessionStateSuccess
	case SessionStateFailure, SessionStateError, SessionStateSuccess:
		return SessionStateDone
	default:
		return w.State + 1
	}
}

// routeCheckConditionState determines next state based on condition check results.
func (w *Session) routeCheckConditionState() int {
	meet := w.checkCondition()
	switch {
	case !meet && w.Workflow.Else != nil:
		return SessionStateElse
	case !meet:
		w.CurrentMsg.Labels["status"] = SessionStatusSuccess

		return SessionStateDone
	case w.Workflow.CacheKey != "":
		return SessionStateCheckCache
	case w.isLoop():
		return SessionStateCheckLoopCondition
	}

	return SessionStateCheckIteration
}

// routeCheckCacheState handles cache-related state transitions.
func (w *Session) routeCheckCacheState() int {
	if _, ok := w.CurrentMsg.Labels[LabelFromCache]; ok {
		w.CurrentMsg.Labels["status"] = SessionStatusSuccess

		return SessionStateDone
	}

	if w.isLoop() {
		return SessionStateCheckLoopCondition
	}

	return SessionStateCheckIteration
}

// routeCheckLoopConditionState handles loop condition evaluation and routing.
func (w *Session) routeCheckLoopConditionState() int {
	if !w.checkLoopCondition() {
		if w.LoopCount == 0 {
			return SessionStateElse
		}

		return SessionStateExport
	}

	return SessionStateFirstRound
}

// routeCheckIterationState determines if and how to handle iterations.
func (w *Session) routeCheckIterationState() int {
	if w.Workflow.Iterate == nil {
		return SessionStateCheckSteps
	}
	if len(w.Workflow.Iterate.([]any)) == 0 {
		return SessionStateElse
	}

	return SessionStateFirstItem
}

// routeNextItemState handles iteration item transitions.
func (w *Session) routNextItemState() int {
	nextState := SessionStateCheckSteps
	if w.Workflow.IterateParallel != nil {
		nextState = SessionStateCheckAction
	}

	return nextState
}

// routeCheckStepsState evaluates if there are steps to execute.
func (w *Session) routeCheckStepsState() int {
	if w.Workflow.Steps == nil {
		return SessionStateCheckAction
	}
	if len(w.Workflow.Steps) > 0 {
		return SessionStateFirstStep
	}

	return SessionStateUpdate
}

// routeCheckActionState determines the appropriate action state.
func (w *Session) routeCheckActionState() int {
	if w.LoopCount == 0 && w.Iteration == 0 && w.Current == 0 {
		if w.checkIsNoop() {
			return SessionStateUpdate
		}

		return SessionStateFirstAction
	}

	return SessionStateAction
}

// routeUpdateState handles transition after update operations.
func (w *Session) routeUpdateState() int {
	if w.ElseBranch != nil {
		return SessionStateExport
	}

	if w.CurrentMsg.Labels["status"] == SessionStatusFailure && w.Workflow.OnFailure == "exit" {
		return SessionStateExport
	}

	if w.CurrentMsg.Labels["status"] == SessionStatusError && w.Workflow.OnError != "continue" {
		return SessionStateExport
	}

	return SessionStateCheckEndSteps
}

// routeCheckEndStepsState manages step completion transitions.
func (w *Session) routeCheckEndStepsState() int {
	if w.Workflow.Steps == nil && w.Workflow.Threads == nil {
		return SessionStateCheckEndIterations
	}
	if w.Current == 0 {
		return SessionStateEndFirstStep
	}

	return SessionStateEndStep
}

// routeEndStepState handles step completion and determines next step.
func (w *Session) routeEndStepState() int {
	w.Current++
	nextState := SessionStateCheckEndIterations
	if w.Current < len(w.Workflow.Steps) {
		nextState = SessionStateNextStep
	} else if w.Current < len(w.Workflow.Threads) {
		nextState = SessionStateAction
	}

	return nextState
}

// routeCheckEndIterationsState manages iteration completion.
func (w *Session) routeCheckEndIterationsState() int {
	if !w.isIteration() {
		return SessionStateCheckEndRounds
	}
	if w.Iteration == 0 {
		return SessionStateEndFirstItem
	}

	return SessionStateEndItem
}

// routeEndItemState handles completion of iteration items.
func (w *Session) routeEndItemState() int {
	w.Iteration++
	nextState := SessionStateCheckEndRounds
	if w.Iteration < w.lenOfIterate() {
		nextState = SessionStateNextItem
		w.Current = 0
	}

	return nextState
}

// routeCheckEndRoundsState determines if loop rounds are complete.
func (w *Session) routeCheckEndRoundsState() int {
	if !w.isLoop() {
		return SessionStateExport
	}
	if w.LoopCount == 0 {
		return SessionStateEndFirstRound
	}

	return SessionStateEndRound
}

// routeEndRoundState handles completion of loop rounds.
func (w *Session) routeEndRoundState() int {
	nextState := SessionStateExport
	if w.checkLoopCondition() {
		nextState = SessionStateNextRound
		w.Iteration = 0
		w.Current = 0
	}
	w.LoopCount++

	return nextState
}

// routeExportState determines final state based on execution status.
func (w *Session) routeExportState() int {
	if w.CurrentMsg.Labels["status"] == SessionStatusFailure {
		return SessionStateFailure
	}
	if w.CurrentMsg.Labels["status"] == SessionStatusError {
		return SessionStateError
	}
	if w.Workflow.CacheKey != "" {
		return SessionStateSaveCache
	}

	return SessionStateSuccess
}
