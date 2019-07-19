// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"reflect"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

func (w *Session) fireHook(name string, msg *dipper.Message) {
	if w.currentHook != "" {
		if w.currentHook == name {
			// hook is cleared when called again
			w.currentHook = ""
			return
		}
		dipper.Logger.Panicf("[workflow] hooks overlapping: %s vs %s for session %s", name, w.currentHook, w.ID)
	}

	hookBlock, ok := dipper.GetMapData(w.ctx, "hooks."+name)
	if ok {
		w.currentHook = name
		w.savedMsg = msg
		if w.ID == "" {
			defer daemon.Children.Add(1)
			go func() {
				defer daemon.Children.Done()
				defer dipper.SafeExitOnError("Failed in execute hook %+v", hookBlock)
				w.save()
				defer w.onError()

				w.executeHook(msg, hookBlock)
			}()
		} else {
			w.executeHook(msg, hookBlock)
		}
	}
}

func (w *Session) executeHook(msg *dipper.Message, hookBlock interface{}) {
	var child *Session

	if hook, ok := hookBlock.(string); ok {
		child = w.createChildSession(&config.Workflow{
			Context:  "_hook",
			Workflow: hook,
		}, msg)
	} else {
		v := reflect.ValueOf(hookBlock)
		childWf := config.Workflow{
			Context: "_hook",
		}
		for i := 0; i < v.Len(); i++ {
			childWf.Steps = append(childWf.Steps, config.Workflow{Workflow: v.Index(i).Interface().(string)})
		}
		child = w.createChildSession(&childWf, msg)
	}
	child.execute(msg)
}
