// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
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
		w.setPerforming("processing exit hooks for state: " + SessionStates[w.State])
		hooks = SessionExitHooks
	} else {
		w.setPerforming("processing entry hooks for state: " + SessionStates[w.State])
	}

	ret := false

	for _, hook := range hooks[w.State] {
		if w.CurrentHook == "" {
			w.setPerforming("processing hook:" + hook)
			ret = true
			w.CurrentHook = hook
			if pending := w.executeHook(hook); pending {
				w.pending = true

				break
			}
		}
		if hook == w.CurrentHook {
			w.setPerforming("exiting hook:" + hook)
			ret = true
			w.CurrentHook = ""
			w.child = nil
		}
	}

	return ret
}

// executeHook executes the hook and set session to pending if needed.
func (w *Session) executeHook(name string) bool {
	hookBlock, ok := dipper.GetMapData(w.Ctx, "hooks."+name)
	if !ok {
		return false
	}

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

	msg := *w.CurrentMsg
	msg.Labels = map[string]string{}
	for k, v := range w.CurrentMsg.Labels {
		msg.Labels[k] = v
	}
	w.child = w.store.CreateChildSession(w, work, &msg)
	w.store.ActivateSession(w.child)
	w.child.Wait()

	return w.child.pending || w.child.CurrentHook != ""
}
