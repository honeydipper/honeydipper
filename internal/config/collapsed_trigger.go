// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package config

import (
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// CollapsedTrigger is a trigger with collapsed matching criteria, parameters, merged sysData and stack of exports.
type CollapsedTrigger struct {
	Match      map[string]interface{}   `json:"match"`
	Exports    []map[string]interface{} `json:"exports"`
	SysData    map[string]interface{}   `json:"sysData"`
	Parameters map[string]interface{}   `json:"parameters"`
	SysName    string                   `json:"sysName"`
}

// CollapseTrigger collapses matching criteria, exports and sysData of a trigger and its inheritted triggers.
func CollapseTrigger(t *Trigger, c *DataSet) (*Trigger, *CollapsedTrigger) {
	var stack []*Trigger
	current := t

	for len(current.Source.System) > 0 {
		stack = append(stack, current)
		sourceSys := c.Systems[current.Source.System]
		currentTrigger := sourceSys.Triggers[current.Source.Trigger]
		current = &currentTrigger
	}

	match := dipper.MustDeepCopyMap(current.Match)
	params := dipper.MustDeepCopyMap(current.Parameters)
	sysData := map[string]interface{}{}
	exports := []map[string]interface{}{current.Export}

	for i := len(stack) - 1; i >= 0; i-- {
		trigger := stack[i]
		sourceSys := c.Systems[trigger.Source.System]

		sysData = dipper.MergeMap(sysData, dipper.MustDeepCopyMap(sourceSys.Data))
		params = dipper.MergeMap(params, dipper.MustDeepCopyMap(trigger.Parameters))
		match = dipper.MergeMap(match, dipper.MustDeepCopyMap(trigger.Match))
		exports = append(exports, trigger.Export)

		if current.Driver == "webhook" {
			if sigsec, ok := sourceSys.Data["signatureSecret"]; ok && sigsec.(string) != "" {
				match["verifiedSystem"] = trigger.Source.System
			}
		}
	}

	if len(sysData) > 0 {
		envData := map[string]interface{}{
			"sysData": sysData,
		}
		match = dipper.Interpolate(match, envData).(map[string]interface{})
		params = dipper.Interpolate(params, envData).(map[string]interface{})
	}

	return current, &CollapsedTrigger{
		Match:      match,
		Exports:    exports,
		SysData:    sysData,
		Parameters: params,
		SysName:    t.Source.System,
	}
}

// ExportContext putting raw data from event into context as abstracted fields.
func (t *CollapsedTrigger) ExportContext(eventName string, envData map[string]interface{}) map[string]interface{} {
	newCtx := map[string]interface{}{}
	envData["ctx"] = newCtx
	envData["sysData"] = t.SysData

	for _, layer := range t.Exports {
		delta := dipper.Interpolate(layer, envData)
		newCtx = dipper.MergeMap(newCtx, delta)
	}

	if _, ok := newCtx["_meta_event"]; !ok {
		newCtx["_meta_event"] = eventName
	}

	return newCtx
}
