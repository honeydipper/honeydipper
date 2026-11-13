// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"errors"
	"fmt"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

var (
	ErrHookFailed = errors.New("hook failed")
	ErrHookError  = errors.New("hook error")
)

// SessionEntryHooks maps hooks to start of the session states.
var SessionEntryHooks = map[int][]string{
	SessionStateCheckCondition: {"on_session"},
	SessionStateFirstRound:     {"on_first_round"},
	SessionStateNextRound:      {"on_round"},
	SessionStateFirstItem:      {"on_first_item"},
	SessionStateNextItem:       {"on_item"},
	SessionStateFirstAction:    {"on_first_action"},
}

// SessionExitHooks maps hooks to exit of the session states.
var SessionExitHooks = map[int][]string{
	SessionStateUpdate:  {"on_update"},
	SessionStateFailure: {"on_failure", "on_exit"},
	SessionStateError:   {"on_error", "on_exit"},
	SessionStateSuccess: {"on_success", "on_exit"},
}

// fireOrClearHook fires or clears the hooks for the session.
func (w *Session) fireOrClearHook(entry bool) bool {
	hooks := SessionEntryHooks
	if !entry {
		hooks = SessionExitHooks
	}

	handled := false

	for _, hook := range hooks[w.State] {
		if w.CurrentHook == "" {
			// try entering hook
			if !w.pending {
				handled = true
				w.CurrentHook = hook
				if w.pending = w.executeHook(hook); w.pending {
					break
				}
			}
		}
		if hook == w.CurrentHook {
			// exiting hook
			w.CurrentHook = ""
			handled = true
			if w.pending {
				w.setPerforming("exiting hook:" + hook)
				w.restoreFromHook()
				w.pending = false
			}
		}
	}

	return handled
}

// executeHook executes the hook and set session to pending if needed.
func (w *Session) executeHook(name string) bool {
	hookBlock, ok := dipper.GetMapData(w.Ctx, "hooks."+name)
	if !ok {
		return false
	}
	w.setPerforming("entering hook:" + name)

	work := &config.Workflow{
		Context: SessionContextHooks,
	}
	switch hook := hookBlock.(type) {
	case string:
		work.Workflow = hook
	case []interface{}:
		if len(hook) == 1 {
			work.Workflow = hook[0].(string)

			break
		}
		for _, h := range hook {
			work.Threads = append(work.Threads, config.Workflow{Workflow: h.(string)})
		}
	}

	if work.Workflow == "" && len(work.Threads) == 0 {
		return false
	}

	w.OrigMsg = w.CurrentMsg
	w.OrigChild = w.child

	msg := *w.CurrentMsg
	msg.Labels = map[string]string{}
	for k, v := range w.CurrentMsg.Labels {
		msg.Labels[k] = v
	}
	w.child = w.store.CreateChildSession(w, work, &msg)
	w.store.ActivateSession(w.child)
	w.child.Wait()

	pending := w.child.pending || w.child.CurrentHook != ""

	if !pending {
		w.restoreFromHook()
	}

	return pending
}

// restoreFromHook restores the session state from the hook if the session is still pending.
func (w *Session) restoreFromHook() {
	// allowing hooks to export data.
	for _, e := range w.child.Exported {
		if w.ElseBranch == nil {
			w.Ctx = dipper.MergeMap(w.Ctx, dipper.MustDeepCopy(e))
		}
		w.processNoExport(e)
		if len(e) > 0 {
			w.store.GetLogger().Debugf("session [%s.%s] depth %d from hook [%s.%s] depth %d exported: %+v",
				w.ID,
				w.CurrentMsg.Labels["cursor"],
				w.depth,
				w.child.ID,
				w.child.CurrentMsg.Labels["cursor"],
				w.child.depth,
				e)
			w.Exported = append(w.Exported, e)
		}
	}

	// moving forward with cursor but without carrying over the message from the hook.
	cursor := w.child.CurrentMsg.Labels["cursor"]

	w.CurrentMsg = w.OrigMsg
	w.CurrentMsg.Labels["cursor"] = cursor
	w.OrigMsg = nil
	child := w.child
	w.child = w.OrigChild
	w.OrigChild = nil

	if w.CurrentMsg.Labels["status"] != "failure" && w.CurrentMsg.Labels["status"] != "error" {
		// report hook failure and error if parent is not already in failure or error status.
		// if the child is already in failure or error status, preserve the parent status.
		switch child.CurrentMsg.Labels["status"] {
		case "failure":
			panic(fmt.Errorf("%w: %s", ErrHookFailed, child.CurrentMsg.Labels["reason"]))
		case "error":
			panic(fmt.Errorf("%w: %s", ErrHookError, child.CurrentMsg.Labels["reason"]))
		}
	}

	w.trimPerformingToCurrentDepth()
}
