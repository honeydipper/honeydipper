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

// CollapseTrigger collapses matching criteria, exports and sysData of a trigger and its inheritted triggers
func CollapseTrigger(t Trigger, c *DataSet) (Trigger, dipper.CollapsedTrigger) {
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
	params := map[string]interface{}{}
	var exports []map[string]interface{}
	for i := len(stack) - 1; i >= 0; i-- {
		cp, _ := dipper.DeepCopy(stack[i].Match)
		err := mergo.Merge(&match, cp, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			panic(err)
		}
		cpParams, _ := dipper.DeepCopy(stack[i].Parameters)
		err = mergo.Merge(&params, cpParams, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			panic(err)
		}
		exports = append(exports, stack[i].Export)
	}
	if len(sysData) > 0 {
		envData := map[string]interface{}{
			"sysData": sysData,
		}
		match = dipper.Interpolate(match, envData).(map[string]interface{})
		params = dipper.Interpolate(params, envData).(map[string]interface{})
	}
	return current, dipper.CollapsedTrigger{
		Match:      match,
		Exports:    exports,
		SysData:    sysData,
		Parameters: params,
	}
}
