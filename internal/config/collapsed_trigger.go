// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/imdario/mergo"
)

// CollapsedTrigger is a trigger with collapsed matching criteria, merged sysData and stack of exports
type CollapsedTrigger struct {
	Match   map[string]interface{}   `json:"match"`
	Exports []map[string]interface{} `json:"exports"`
	SysData map[string]interface{}   `json:"sysData"`
}

// CollapseTrigger collapses matching criteria, exports and sysData of a trigger and its inheritted triggers
func CollapseTrigger(t Trigger, c *DataSet) (Trigger, CollapsedTrigger) {
	current := t
	sysData := map[string]interface{}{}
	var stack []Trigger
	if current.Match != nil {
		stack = append(stack, current)
	}
	for len(current.Source.System) > 0 {
		if len(current.Driver) > 0 {
			dipper.Logger.Panicf("[receiver] a trigger cannot have both driver and source %+v", current)
		}
		currentSys := c.Systems[current.Source.System]
		currentSysData, _ := dipper.DeepCopy(currentSys.Data)
		err := mergo.Merge(&sysData, currentSysData, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			panic(err)
		}
		current = currentSys.Triggers[current.Source.Trigger]
		if current.Match != nil {
			stack = append(stack, current)
		}
	}
	if len(current.Driver) == 0 {
		dipper.Logger.Panicf("[receiver] a trigger should have a driver or a source %+v", current)
	}
	match := map[string]interface{}{}
	var exports []map[string]interface{}
	for i := len(stack) - 1; i >= 0; i-- {
		cp, _ := dipper.DeepCopy(stack[i].Match)
		err := mergo.Merge(&match, cp, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			panic(err)
		}
		exports = append(exports, stack[i].Export)
	}
	if len(sysData) > 0 {
		match = dipper.Interpolate(match, map[string]interface{}{
			"sysData": sysData,
		}).(map[string]interface{})
	}
	return current, CollapsedTrigger{
		Match:   match,
		Exports: exports,
		SysData: sysData,
	}
}
