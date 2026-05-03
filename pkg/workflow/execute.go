// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package workflow

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

// execute takes actions for a single iteration in a single loop round.
// It handles different workflow types including parallel iterations, nested workflows,
// function calls, driver calls, steps, threads and switch cases.
func (w *Session) execute() {
	switch {
	case w.Workflow.IterateParallel != nil:
		w.setPerforming("launching parallel iterations")
		w.launchAllParallelIterations()
		w.pending = true
	case w.Workflow.Workflow != "":
		w.callWorkflow()
	case w.isFunction():
		// Handle direct function execution.
		f := w.interpolateFunction(&w.Workflow.Function)
		w.setPerforming("running function: " + f.Target.System + "." + f.Target.Function)
		w.Action++
		w.callFunction(f)
		w.pending = true
	case w.Workflow.CallDriver != "":
		// Handle driver function calls.
		w.setPerforming("invoking driver function: " + w.Workflow.CallDriver)
		w.Action++
		w.callDriver(w.Workflow.CallDriver)
		w.pending = true
	case w.Workflow.CallFunction != "":
		// Handle shorthand function calls.
		w.setPerforming("calling function: " + w.Workflow.CallFunction)
		w.Action++
		w.callShorthandFunction(w.Workflow.CallFunction)
		w.pending = true
	case w.Workflow.SendEvent != nil:
		w.setPerforming("sending eventbus:message")
		w.Action++
		w.sendEventbusMessage()
	case w.Workflow.Steps != nil:
		// Execute workflow steps sequentially.
		w.setPerforming("step - " + strconv.Itoa(w.Current))
		w.child = w.store.CreateChildSession(w, &w.Workflow.Steps[w.Current], w.CurrentMsg)
		w.child.Ctx["step_number"] = w.Current
		w.store.ActivateSession(w.child)
		w.child.Wait()
		if w.child.pending {
			w.pending = true
		}
	case w.Workflow.Threads != nil:
		// Handle parallel thread execution.
		if w.Current == 0 {
			w.incCursor()
			w.setPerforming("launching threads")
			for i, t := range w.Workflow.Threads {
				child := w.store.CreateAsyncChildSession(w, &t, w.CurrentMsg)
				child.Ctx["thread_number"] = i
				w.store.ActivateSession(child)
			}
		}
		w.setPerforming(fmt.Sprintf("Waiting for threads (%d/%d done)", w.Current, len(w.Workflow.Threads)))
		w.pending = true
	case w.Workflow.Wait != "":
		w.Action++
		w.enterWait()
	case w.Workflow.Switch != "":
		// Handle conditional branching.
		w.executeSwitch()
		if w.child != nil {
			w.child.Wait()
			if w.child.pending {
				w.pending = true
			}
		}
	case w.Workflow.Resume != "":
		w.Action++
		// Handle session resumption.
		w.triggerResume()
	}
}

// callWorkflow invokes the named workflow.
func (w *Session) callWorkflow() {
	w.store.GetLogger().Infof("session [%s.%s] depth %d executing nested workflow: %s",
		w.ID,
		w.CurrentMsg.Labels["cursor"],
		w.depth,
		w.Workflow.Workflow,
	)
	// Handle named workflow execution.
	e := w.buildEnvData()
	name := dipper.InterpolateStr(w.Workflow.Workflow, e)
	w.setPerforming("calling workflow: " + name)

	src, ok := w.store.GetConfig().DataSet.Workflows[name]
	if !ok {
		w.store.GetLogger().Panicf("[%s.%s] depth %d workflow %s not found", w.ID, w.CurrentMsg.Labels["cursor"], w.depth, name)
	}
	src.Name = name
	w.child = w.store.CreateChildSession(w, &src, w.CurrentMsg)
	w.store.ActivateSession(w.child)
	w.child.Wait()
	if w.child.pending {
		w.pending = true
	}
}

// triggerResume sends a message to resume another workflow.
func (w *Session) triggerResume() {
	w.setPerforming("resuming session: " + w.Workflow.Resume)
	msg := &dipper.Message{}
	if m, ok := w.Ctx["resume_message"]; ok {
		dipper.Must(mapstructure.Decode(m, msg))
	}
	failIfMissing, _ := dipper.GetMapDataBool(w.Ctx, "fail_if_missing")

	backoffLimits, _ := dipper.GetMapDataInt(w.Ctx, "backoff_limits")
	interval, _ := dipper.GetMapDataInt(w.Ctx, "interval")
	if interval <= 0 {
		interval = 1
	}
	succeed := w.store.ResumeSession(w.Workflow.Resume, msg)
	for !succeed && backoffLimits > 0 {
		duration := time.Duration(interval) * time.Second
		time.Sleep(duration)
		interval *= 2
		succeed = w.store.ResumeSession(w.Workflow.Resume, msg)
		backoffLimits--
	}

	if !succeed && failIfMissing {
		w.CurrentMsg.Labels["status"] = SessionStatusFailure
		w.CurrentMsg.Labels["reason"] = "session not found"
	} else {
		w.CurrentMsg.Labels["status"] = SessionStatusSuccess
	}
}

// enterWait sets the session state to waiting and increments the cursor.
func (w *Session) enterWait() {
	w.incCursor()
	duration := dipper.Must(time.ParseDuration(w.Workflow.Wait)).(time.Duration)
	due := time.Now().Add(duration).UnixMicro()
	w.setPerforming("waiting")
	key := w.ID + "." + w.CurrentMsg.Labels["cursor"]
	if k, ok := w.Ctx["resume_key"]; ok && k != nil && k.(string) != "" {
		key = k.(string)
	}
	dueMsg := map[string]any{}
	if data, ok := w.Ctx["timeout_message"]; ok && data != nil {
		if m, ok := data.(map[string]any); ok {
			dueMsg = m
		}
	}
	if _, ok := dueMsg["labels"].(map[string]any); !ok {
		dueMsg["labels"] = map[string]any{}
	}
	dueMsg["labels"].(map[string]any)["sessionID"] = w.ID
	dueMsg["labels"].(map[string]any)["cursor"] = w.CurrentMsg.Labels["cursor"]
	dipper.Must(w.store.Call("scheduler", "once", map[string]any{
		"due":         strconv.FormatInt(due, 10),
		"type":        "session",
		"key":         key,
		"due_message": dueMsg,
	}))
	w.pending = true
}

// callFunction makes a call to a function by sending a message through the eventbus channel.
func (w *Session) callFunction(f *config.Function) {
	// Store function for context export later.
	w.InFlyFunction = f

	w.incCursor()

	payload := w.buildEnvData()
	payload["function"] = *f

	// Prepare labels for the message, excluding status-related ones.
	labels := map[string]string{}
	for k, v := range w.CurrentMsg.Labels {
		labels[k] = v
	}
	delete(labels, "status")
	delete(labels, "reason")
	delete(labels, "performing")
	labels["sessionID"] = w.ID
	labels["cursor"] = w.CurrentMsg.Labels["cursor"]
	if w.EventID != "" {
		labels["eventID"] = w.EventID
	}

	cmdmsg := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: "command",
		Payload: payload,
		Labels:  labels,
	}

	w.store.SendMessage(cmdmsg)
}

// callDriver makes a call to a driver function defined in short hand fashion.
// It parses the driver name and action, processes local parameters, and calls the function.
func (w *Session) callDriver(f string) {
	interpolatedNames := strings.SplitN(f, ".", 2)
	if len(interpolatedNames) < 2 {
		w.store.GetLogger().Panicf("[%s] call_driver must be in 'driver.action' format, got: %s", w.ID, f)
	}
	driverName, rawActionName := interpolatedNames[0], interpolatedNames[1]

	var locals map[string]interface{}
	if w.Workflow.Local != nil {
		envData := w.buildEnvData()
		if layers, ok := w.Workflow.Local.([]any); ok {
			for _, layer := range layers {
				delta := dipper.Interpolate(layer, envData).(map[string]any)
				locals = dipper.MergeMap(locals, delta)
			}
		} else {
			locals = dipper.Interpolate(w.Workflow.Local, envData).(map[string]interface{})
		}
	}

	w.callFunction(&config.Function{
		Driver:     driverName,
		RawAction:  rawActionName,
		Parameters: locals,
	})
}

// callShorthandFunction makes a call to a function defined in short hand fashion.
// It splits the function string into system and function names.
func (w *Session) callShorthandFunction(f string) {
	interpolatedNames := strings.SplitN(f, ".", 2)
	if len(interpolatedNames) < 2 {
		w.store.GetLogger().Panicf("[%s] call_function must be in 'system.function' format, got: %s", w.ID, f)
	}
	systemName, funcName := interpolatedNames[0], interpolatedNames[1]

	w.callFunction(&config.Function{
		Target: config.Action{
			System:   systemName,
			Function: funcName,
		},
	})
}

// sendEventbusMessage emits an eventbus:message for engine rule processing.
func (w *Session) sendEventbusMessage() {
	id, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}

	msg := &dipper.Message{
		Channel: dipper.ChannelEventbus,
		Subject: dipper.EventbusMessage,
		Payload: w.Workflow.SendEvent,
		Labels: map[string]string{
			"eventID": id.String(),
		},
	}

	w.store.SendMessage(msg)
	w.CurrentMsg.Labels["status"] = SessionStatusSuccess
}

// executeSwitch selects and executes a branch based on the switch condition.
// If no matching case is found, it executes the default branch if present.
func (w *Session) executeSwitch() {
	envData := w.buildEnvData()
	match := dipper.InterpolateStr(w.Workflow.Switch, envData)
	for key, branch := range w.Workflow.Cases {
		if key == match {
			w.setPerforming("executing branch: " + key)
			wf := &config.Workflow{}
			dipper.Must(mapstructure.Decode(branch, wf))
			w.child = w.store.CreateChildSession(w, wf, w.CurrentMsg)
			w.store.ActivateSession(w.child)

			return
		}
	}
	if w.Workflow.Default != nil {
		w.setPerforming("executing default branch")
		wf := &config.Workflow{}
		dipper.Must(mapstructure.Decode(w.Workflow.Default, wf))
		w.child = w.store.CreateChildSession(w, wf, w.CurrentMsg)
		w.store.ActivateSession(w.child)

		return
	}
}

// launchAllParallelIterations starts all the parallel iterations with pool size control.
// The pool size is limited to 100 or the value specified in IteratePool.
func (w *Session) launchAllParallelIterations() {
	poolCount := 100
	if w.Workflow.IteratePool != "" {
		poolCount = min(poolCount, dipper.Must(strconv.Atoi(w.Workflow.IteratePool)).(int))
		if poolCount <= 0 {
			w.store.GetLogger().Panicf("invalid iterate_pool %s", w.Workflow.IteratePool)
		}
	}

	if w.Iteration != 0 {
		if poolCount+w.Iteration < len(w.Workflow.IterateParallel.([]any)) {
			w.launchParallelIteration(poolCount + w.Iteration)
		}

		return
	}

	w.incCursor()
	for i := range w.Workflow.IterateParallel.([]any) {
		w.launchParallelIteration(i)
		poolCount--
		if poolCount == 0 {
			break
		}
	}
}

// launchParallelIteration starts one iteration of the parallel workflow.
// It creates a new workflow instance and sets up the iteration context.
func (w *Session) launchParallelIteration(i int) {
	single := config.Workflow{
		Workflow:     w.Workflow.Workflow,
		Function:     w.Workflow.Function,
		CallFunction: w.Workflow.CallFunction,
		CallDriver:   w.Workflow.CallDriver,
		Switch:       w.Workflow.Switch,
		Cases:        w.Workflow.Cases,
		Default:      w.Workflow.Default,
		Steps:        w.Workflow.Steps,
		Threads:      w.Workflow.Threads,
		Description:  w.Workflow.Description,
	}

	w.Ctx["current"] = w.Workflow.IterateParallel.([]any)[i]
	if w.Workflow.IterateAs != "" {
		w.Ctx[w.Workflow.IterateAs] = w.Ctx["current"]
	}
	child := w.store.CreateAsyncChildSession(w, &single, w.CurrentMsg)
	child.Ctx["thread_number"] = i
	delete(w.Ctx, "current")
	if w.Workflow.IterateAs != "" {
		delete(w.Ctx, w.Workflow.IterateAs)
	}
	w.store.GetLogger().Debugf("session [%s.%s] depth %d launching parallel iteration %d:\n %+v",
		w.ID,
		w.CurrentMsg.Labels["cursor"],
		w.depth,
		i,
		child,
	)

	w.store.ActivateSession(child)
}
