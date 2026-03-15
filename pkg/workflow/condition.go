// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"reflect"
	"strings"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// isTruthy is a helper function to check if string represents Truey value.
func isTruthy(c string) bool {
	c = strings.ToLower(strings.TrimSpace(c))

	return len(c) != 0 && c != "false" && c != "nil" && c != "0" && c != "{}" && c != "[]" && c != "<no value>"
}

// checkCondition check if meet the condition to execute the Workflow.
func (w *Session) checkCondition() bool {
	if len(w.Workflow.If) > 0 {
		for _, c := range w.Workflow.If {
			if !isTruthy(c) {
				return false
			}
		}
	}

	if len(w.Workflow.IfAny) > 0 {
		ret := false
		for _, c := range w.Workflow.IfAny {
			if isTruthy(c) {
				ret = true

				break
			}
		}
		if !ret {
			return false
		}
	}

	if len(w.Workflow.Unless) > 0 {
		for _, c := range w.Workflow.Unless {
			if isTruthy(c) {
				return false
			}
		}
	}

	if len(w.Workflow.UnlessAll) > 0 {
		ret := false
		for _, c := range w.Workflow.UnlessAll {
			if !isTruthy(c) {
				ret = true

				break
			}
		}
		if !ret {
			return false
		}
	}

	if w.Workflow.Match != nil && !dipper.CompareAll(w.Ctx, w.Workflow.Match) {
		return false
	}

	if w.Workflow.UnlessMatch != nil && reflect.ValueOf(w.Workflow.UnlessMatch).Len() > 0 {
		return !dipper.CompareAll(w.Ctx, w.Workflow.UnlessMatch)
	}

	return true
}

// checkLoopCondition check the looping conditions to see if we should continue the loop.
func (w *Session) checkLoopCondition() bool {
	envData := w.buildEnvData()

	switch {
	case w.Workflow.WhileMatch != nil:
		scenario := dipper.Interpolate(w.Workflow.WhileMatch, envData)

		return dipper.CompareAll(w.Ctx, scenario)
	case w.Workflow.UntilMatch != nil:
		scenario := dipper.Interpolate(w.Workflow.UntilMatch, envData)
		if scenario != nil && reflect.ValueOf(scenario).Len() > 0 {
			return !dipper.CompareAll(w.Ctx, scenario)
		}

		return true
	case len(w.Workflow.While) > 0:
		for _, c := range w.Workflow.While {
			c = dipper.InterpolateStr(c, envData)
			if !isTruthy(c) {
				return false
			}
		}

		return true
	case len(w.Workflow.WhileAny) > 0:
		for _, c := range w.Workflow.WhileAny {
			c = dipper.InterpolateStr(c, envData)
			if isTruthy(c) {
				return true
			}
		}

		return false
	case len(w.Workflow.Until) > 0:
		for _, c := range w.Workflow.Until {
			c = dipper.InterpolateStr(c, envData)
			if isTruthy(c) {
				return false
			}
		}

		return true
	case len(w.Workflow.UntilAll) > 0:
		for _, c := range w.Workflow.UntilAll {
			c = dipper.InterpolateStr(c, envData)
			if !isTruthy(c) {
				return true
			}
		}

		return false
	}

	return true // not a loop
}
