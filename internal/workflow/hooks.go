// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

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
		// this line might be unreachable but leave it here for safety
		dipper.Logger.Panicf("[workflow] hooks overlapping: %s vs %s for session %s", name, w.currentHook, w.ID)
	}

	hookBlock, ok := dipper.GetMapData(w.ctx, "hooks."+name)
	if ok {
		w.currentHook = name
		w.savedMsg = msg
		if w.ID == "" {
			daemon.Children.Add(1)
			go func() {
				defer daemon.Children.Done()
				defer dipper.SafeExitOnError("Failed in execute hook %+v", hookBlock)
				w.save()
				defer w.onError()

				dipper.Logger.Infof("[workflow] firing hook %s for new session", name)
				w.executeHook(msg, hookBlock)
			}()
		} else {
			dipper.Logger.Infof("[workflow] firing hook %s for session [%s]", name, w.ID)
			w.executeHook(msg, hookBlock)
		}
	}
}

func (w *Session) executeHook(msg *dipper.Message, hookBlock interface{}) {
	var child *Session

	if hook, ok := hookBlock.(string); ok {
		child = w.createChildSession(&config.Workflow{
			Context:  SessionContextHooks,
			Workflow: hook,
		}, msg)
	} else {
		v := reflect.ValueOf(hookBlock)
		childWf := config.Workflow{
			Context: SessionContextHooks,
		}
		for i := 0; i < v.Len(); i++ {
			childWf.Threads = append(childWf.Threads, config.Workflow{Workflow: v.Index(i).Interface().(string)})
		}
		child = w.createChildSession(&childWf, msg)
	}
	child.execute(msg)
}
