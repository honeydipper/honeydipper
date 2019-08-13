// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"errors"
	"strings"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// isTruey is a helper function to check if string represents Truey value
func isTruey(c string) bool {
	c = strings.ToLower(strings.TrimSpace(c))
	return len(c) != 0 && c != "false" && c != "nil" && c != "0" && c != "{}" && c != "[]"
}

// checkCondition check if meet the condition to execute the workflow
func (w *Session) checkCondition() bool {
	switch {
	case len(w.workflow.If) > 0:
		for _, c := range w.workflow.If {
			if !isTruey(c) {
				return false
			}
		}
		return true
	case len(w.workflow.IfAny) > 0:
		for _, c := range w.workflow.IfAny {
			if isTruey(c) {
				return true
			}
		}
		return false
	case len(w.workflow.Unless) > 0:
		for _, c := range w.workflow.Unless {
			if isTruey(c) {
				return false
			}
		}
		return true
	case len(w.workflow.UnlessAny) > 0:
		for _, c := range w.workflow.UnlessAny {
			if !isTruey(c) {
				return true
			}
		}
		return false
	case w.workflow.Match != nil:
		switch scenario := w.workflow.Match.(type) {
		case map[string]interface{}:
			if len(scenario) > 0 {
				return dipper.CompareAll(w.ctx, scenario)
			}
			return true
		case []interface{}:
			if len(scenario) > 0 {
				return dipper.CompareAll(w.ctx, scenario)
			}
			return true
		default:
			panic(errors.New("unsupported match condition"))
		}
	case w.workflow.UnlessMatch != nil:
		switch scenario := w.workflow.UnlessMatch.(type) {
		case map[string]interface{}:
			if len(scenario) > 0 {
				return !dipper.CompareAll(w.ctx, scenario)
			}
			return true
		case []interface{}:
			if len(scenario) > 0 {
				return !dipper.CompareAll(w.ctx, scenario)
			}
			return true
		default:
			panic(errors.New("unsupported unless_match condition"))
		}
	}
	return true
}

// checkLoopCondition check the looping conditions to see if we should continue the loop
func (w *Session) checkLoopCondition(msg *dipper.Message) bool {
	switch {
	case len(w.workflow.While) > 0:
		envData := w.buildEnvData(msg)
		for _, c := range w.workflow.While {
			c = dipper.InterpolateStr(c, envData)
			if !isTruey(c) {
				return false
			}
		}
		return true
	case len(w.workflow.WhileAny) > 0:
		envData := w.buildEnvData(msg)
		for _, c := range w.workflow.WhileAny {
			c = dipper.InterpolateStr(c, envData)
			if isTruey(c) {
				return true
			}
		}
		return false
	case len(w.workflow.Until) > 0:
		envData := w.buildEnvData(msg)
		for _, c := range w.workflow.Until {
			c = dipper.InterpolateStr(c, envData)
			if isTruey(c) {
				return false
			}
		}
		return true
	case len(w.workflow.UntilAny) > 0:
		envData := w.buildEnvData(msg)
		for _, c := range w.workflow.UntilAny {
			c = dipper.InterpolateStr(c, envData)
			if !isTruey(c) {
				return true
			}
		}
		return false
	}
	return true // not a loop
}
