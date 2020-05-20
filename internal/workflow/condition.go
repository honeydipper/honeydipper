// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"reflect"
	"strings"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// isTruthy is a helper function to check if string represents Truey value
func isTruthy(c string) bool {
	c = strings.ToLower(strings.TrimSpace(c))
	return len(c) != 0 && c != "false" && c != "nil" && c != "0" && c != "{}" && c != "[]" && c != "<no value>"
}

// checkCondition check if meet the condition to execute the workflow
func (w *Session) checkCondition() bool {
	switch {
	case len(w.workflow.If) > 0:
		for _, c := range w.workflow.If {
			if !isTruthy(c) {
				return false
			}
		}
		return true
	case len(w.workflow.IfAny) > 0:
		for _, c := range w.workflow.IfAny {
			if isTruthy(c) {
				return true
			}
		}
		return false
	case len(w.workflow.Unless) > 0:
		for _, c := range w.workflow.Unless {
			if isTruthy(c) {
				return false
			}
		}
		return true
	case len(w.workflow.UnlessAll) > 0:
		for _, c := range w.workflow.UnlessAll {
			if !isTruthy(c) {
				return true
			}
		}
		return false
	case w.workflow.Match != nil:
		return dipper.CompareAll(w.ctx, w.workflow.Match)
	case w.workflow.UnlessMatch != nil:
		if reflect.ValueOf(w.workflow.UnlessMatch).Len() > 0 {
			return !dipper.CompareAll(w.ctx, w.workflow.UnlessMatch)
		}
		return true
	}
	return true
}

// checkLoopCondition check the looping conditions to see if we should continue the loop
func (w *Session) checkLoopCondition(msg *dipper.Message) bool {
	switch {
	case w.workflow.WhileMatch != nil:
		envData := w.buildEnvData(msg)
		scenario := dipper.Interpolate(w.workflow.WhileMatch, envData)
		return dipper.CompareAll(w.ctx, scenario)
	case w.workflow.UntilMatch != nil:
		envData := w.buildEnvData(msg)
		scenario := dipper.Interpolate(w.workflow.UntilMatch, envData)
		if scenario != nil && reflect.ValueOf(scenario).Len() > 0 {
			return !dipper.CompareAll(w.ctx, scenario)
		}
		return true
	case len(w.workflow.While) > 0:
		envData := w.buildEnvData(msg)
		for _, c := range w.workflow.While {
			c = dipper.InterpolateStr(c, envData)
			if !isTruthy(c) {
				return false
			}
		}
		return true
	case len(w.workflow.WhileAny) > 0:
		envData := w.buildEnvData(msg)
		for _, c := range w.workflow.WhileAny {
			c = dipper.InterpolateStr(c, envData)
			if isTruthy(c) {
				return true
			}
		}
		return false
	case len(w.workflow.Until) > 0:
		envData := w.buildEnvData(msg)
		for _, c := range w.workflow.Until {
			c = dipper.InterpolateStr(c, envData)
			if isTruthy(c) {
				return false
			}
		}
		return true
	case len(w.workflow.UntilAll) > 0:
		envData := w.buildEnvData(msg)
		for _, c := range w.workflow.UntilAll {
			c = dipper.InterpolateStr(c, envData)
			if !isTruthy(c) {
				return true
			}
		}
		return false
	}
	return true // not a loop
}
