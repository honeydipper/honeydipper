// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// CollapsedFunction has all the function exports, parameters, sysData allow with the final raw function
type CollapsedFunction struct {
	Driver     string
	RawAction  string
	SysData    map[string]interface{}
	Parameters map[string]interface{}
	Stack      []*Function
}

// CollapseFunction collapse the function and all its exporting contexts
func CollapseFunction(s *System, f *Function, cfg *Config) *CollapsedFunction {
	var ret *CollapsedFunction

	if len(f.Driver) == 0 {
		subSystem, ok := cfg.DataSet.Systems[f.Target.System]
		if !ok {
			dipper.Logger.Panicf("[operator] system not defined %s", f.Target.System)
		}
		subFunction, ok := subSystem.Functions[f.Target.Function]
		if !ok {
			dipper.Logger.Panicf("[operator] function not defined %s.%s", f.Target.System, f.Target.Function)
		}
		ret = CollapseFunction(&subSystem, &subFunction, cfg)
	} else {
		if len(f.Target.System) > 0 {
			dipper.Logger.Panicf("[operator] function cannot have both driver and target %s.%s %s", f.Target.System, f.Target.Function, f.Driver)
		}
		ret = &CollapsedFunction{
			Driver:    f.Driver,
			RawAction: f.RawAction,
		}
	}

	if s != nil && s.Data != nil {
		currentSysDataCopy, _ := dipper.DeepCopy(s.Data)
		if ret.SysData == nil {
			ret.SysData = map[string]interface{}{}
		}
		dipper.MergeMap(ret.SysData, currentSysDataCopy)
	}
	if f.Parameters != nil {
		currentParamCopy, _ := dipper.DeepCopy(f.Parameters)
		if ret.Parameters == nil {
			ret.Parameters = map[string]interface{}{}
		}
		dipper.MergeMap(ret.Parameters, currentParamCopy)
	}

	ret.Stack = append(ret.Stack, f)
	return ret
}

// ExportContext create a context data structure based on the collapsed function exports
func (f *CollapsedFunction) ExportContext(status string, envData map[string]interface{}) map[string]interface{} {
	newCtx, err := dipper.DeepCopy(envData["ctx"].(map[string]interface{}))
	if err != nil {
		panic(err)
	}
	envData["ctx"] = newCtx

	if status != "error" {
		for _, layer := range f.Stack {
			delta := dipper.Interpolate(layer.Export, envData)
			dipper.MergeMap(newCtx, delta)
			switch status {
			case "success":
				delta := dipper.Interpolate(layer.ExportOnSuccess, envData)
				dipper.MergeMap(newCtx, delta)
			case "failure":
				delta := dipper.Interpolate(layer.ExportOnFailure, envData)
				dipper.MergeMap(newCtx, delta)
			}
		}
	}

	return newCtx
}
