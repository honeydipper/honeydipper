// Copyright 2025 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"fmt"
	"strings"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// processNoExport prevent exporting the data into parent workflow session.
func (w *Session) processNoExport(exported map[string]interface{}) {
	for _, key := range w.Workflow.NoExport {
		if key == "*" {
			for k := range exported {
				delete(exported, k)
			}

			break
		}
		delete(exported, key)
		delete(exported, key+"-")
		delete(exported, key+"+")
		delete(exported, key+"*")
	}
}

// processWorkflowExport populates the exported data to be used by parent workflows.
func (w *Session) processWorkflowExport() {
	if w.ElseBranch != nil {
		return
	}

	envData := w.buildEnvData()
	status := w.CurrentMsg.Labels["status"]

	defer dipper.SafeExitOnError("session [%s] error on exporting data", w.ID, func(r interface{}) {
		w.CurrentMsg.Labels["status"] = SessionStatusError
		if len(w.CurrentMsg.Labels["reason"]) > 0 {
			w.CurrentMsg.Labels["reason"] += "\n"
		}
		w.CurrentMsg.Labels["reason"] += fmt.Sprintf("Error on exporting data %+v", r)
		if w.CurrentMsg.Labels["performing"] == "" {
			w.CurrentMsg.Labels["performing"] = strings.Join(w.performingValues(), "\n")
		}
	})

	if w.InFlyFunction != nil && status != SessionStatusError {
		export := config.ExportFunctionContext(w.InFlyFunction, envData, w.store.GetConfig())
		w.Ctx = dipper.MergeMap(w.Ctx, dipper.MustDeepCopy(export))
		delete(envData, "sysData")
		if len(export) > 0 {
			w.Exported = append(w.Exported, export)
		}
	}
	if status != SessionStatusError {
		w.processExport(w.Workflow.Export, envData)
	} else {
		w.processExport(w.Workflow.ExportOnError, envData)
	}
	if status == SessionStatusSuccess {
		w.processExport(w.Workflow.ExportOnSuccess, envData)
	}
	if status == SessionStatusFailure {
		w.processExport(w.Workflow.ExportOnFailure, envData)
	}
}

// processExport interpolate the given data and add it to the export stack.
func (w *Session) processExport(exportMap map[string]interface{}, envData map[string]interface{}) {
	delta := dipper.Interpolate(exportMap, envData).(map[string]interface{})
	w.Ctx = dipper.MergeMap(w.Ctx, dipper.MustDeepCopy(delta))
	envData["ctx"] = w.Ctx
	if len(delta) > 0 {
		w.Exported = append(w.Exported, delta)
	}
}
