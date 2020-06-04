// Copyright 2020 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"strings"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// ExportFunctionContext create a context data structure based on the collapsed function exports
func ExportFunctionContext(f *Function, envData map[string]interface{}, cfg *Config) map[string]interface{} {
	var exported map[string]interface{}
	status := "success"
	if labelsData, ok := envData["labels"]; ok {
		if labels, ok := labelsData.(map[string]string); ok {
			if s, ok := labels["status"]; ok {
				status = s
			}
		}
	}

	if status != "error" {
		var sysData map[string]interface{}
		if d, ok := envData["sysData"]; ok {
			sysData, ok = d.(map[string]interface{})
			if !ok {
				dipper.Logger.Panicf("[operator] sysData is not a map")
			}
		}

		// the parameters need to be squashed before interpolation
		// we will also need to provide the squashed sysData for interpolation
		// the outer parameters should override inner parameters, but
		// the inner sysData should override outer sysData
		outerParam, _ := envData["params"]
		envData["params"] = dipper.MergeMap(dipper.MustDeepCopyMap(f.Parameters), outerParam)

		if f.Target.System != "" {
			childSystem, ok := cfg.DataSet.Systems[f.Target.System]
			if !ok {
				dipper.Logger.Panicf("[operator] system not defined %s", f.Target.System)
			}
			childFunction, ok := childSystem.Functions[f.Target.Function]
			if !ok {
				dipper.Logger.Panicf("[operator] function not defined %s.%s", f.Target.System, f.Target.Function)
			}

			// merging system data
			mergedSysData := dipper.MergeMap(dipper.MustDeepCopyMap(sysData), dipper.MustDeepCopy(childSystem.Data))

			// split subsystem from system
			subsystems := strings.Split(f.Target.Function, ".")
			for _, subsystem := range subsystems[:len(subsystems)-1] {
				parent := mergedSysData
				mergedSysData = parent[subsystem].(map[string]interface{})
				mergedSysData["parent"] = parent
			}

			envData["sysData"] = mergedSysData
			exported = ExportFunctionContext(&childFunction, envData, cfg)
		} else {
			if len(f.Target.System) > 0 {
				dipper.Logger.Panicf("[operator] function cannot have both driver and target %s.%s %s", f.Target.System, f.Target.Function, f.Driver)
			}

			// here comes the interpolation for the squashed parameters. This interpolation only happens
			// once, in the inner most function. After that, the parameters can be used for export in all outer
			// layers.
			squashedParams, _ := envData["params"]
			envData["params"] = dipper.Interpolate(squashedParams, envData)
		}

		// here we abandon the squashed sysData after it is consumed, and use a clean sysData for
		// interpolating the exported data.
		envData["sysData"] = sysData
		var newCtx map[string]interface{}

		if newCtxData, ok := envData["ctx"]; ok && newCtxData != nil {
			newCtx = newCtxData.(map[string]interface{})
		}

		delta := dipper.Interpolate(f.Export, envData)
		exported = dipper.MergeMap(exported, dipper.MustDeepCopy(delta))
		newCtx = dipper.MergeMap(newCtx, delta)
		switch status {
		case "success":
			delta := dipper.Interpolate(f.ExportOnSuccess, envData)
			exported = dipper.MergeMap(exported, dipper.MustDeepCopy(delta))
			newCtx = dipper.MergeMap(newCtx, delta)
		case "failure":
			delta := dipper.Interpolate(f.ExportOnFailure, envData)
			exported = dipper.MergeMap(exported, dipper.MustDeepCopy(delta))
			newCtx = dipper.MergeMap(newCtx, delta)
		}
		envData["ctx"] = newCtx
	}

	return exported
}
