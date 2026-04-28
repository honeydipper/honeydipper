// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"sync"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/honeydipper/honeydipper/v4/pkg/workflow"
)

// CollapsedRule maps the rule to its all collapsed match and exports.
type CollapsedRule struct {
	Trigger      *config.CollapsedTrigger
	OriginalRule *config.Rule
}

var (
	ruleMapLock sync.Mutex
	ruleMap     map[string][]*CollapsedRule
)

var (
	engine       *Service
	sessionStore workflow.Store
)

const pendingActivationKeyPrefix = "engine:activation:"

type pendingActivation struct {
	Activate config.Activation      `json:"activate"`
	Labels   map[string]string      `json:"labels"`
	Event    interface{}            `json:"event"`
	Ctx      map[string]interface{} `json:"ctx"`
}

// WorkflowHelper enables workflow engine to load config and send messages.
type WorkflowHelper struct {
	dipper.RPCCaller
	engine *Service
}

// SendMessage method sends workflow messages to eventbus channle.
func (h *WorkflowHelper) SendMessage(msg *dipper.Message) {
	worker := h.engine.getDriverRuntime(dipper.ChannelEventbus)
	worker.SendMessage(msg)
}

// GetConfig method feed config from service to workflow engine.
func (h *WorkflowHelper) GetConfig() *config.Config {
	return h.engine.config
}

// OnSessionCompleted handles post-completion cleanup hooks from workflow store.
func (h *WorkflowHelper) OnSessionCompleted(w *workflow.Session) {
	defer dipper.SafeExitOnError("[engine] failed handling workflow completion callback")
	if w == nil || w.ID == "" {
		return
	}

	pa, ok := loadPendingActivation(w.ID)
	if !ok {
		return
	}

	msg := &dipper.Message{Labels: pa.Labels}
	dispatchActivate(&pa.Activate, msg, pa.Event, pa.Ctx, w)

	cleanupPendingActivation(w.ID)
}

// StartEngine Starts the engine service.
func StartEngine(cfg *config.Config) {
	engine = NewService(cfg, "engine")

	engine.ServiceReload = buildRuleMap
	engine.EmitMetrics = engineMetrics
	engine.addResponder("eventbus:message", createSessions)
	engine.addResponder("eventbus:return", continueSession)
	engine.addResponder("scheduler:session", continueSession)
	setupEngineAPIs()

	engine.start()
	if cfg.IsJobMode {
		go func() {
			<-engine.Ready()
			msg := &dipper.Message{
				Labels: map[string]string{
					"eventID": "main",
				},
			}
			wf, ok := engine.config.DataSet.Workflows["reserved/main"]
			if !ok {
				dipper.Logger.Panic("[engine] missing reserved/main workflow in job mode")
			}
			wf.Name = "reserved/main"
			sessionStore.StartSession(&wf, msg, map[string]interface{}{})
			defer dipper.IgnoreError(context.Canceled)
			workflow.Wait(engine.context, engine, "main", "engine_in_job_mode")
			StopAll()
		}()
	}
}

func createSessions(d *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	<-engine.Ready()
	msg = dipper.DeserializePayload(msg)

	if _, ok := dipper.GetMapData(msg.Payload, "do"); ok {
		go sessionStore.StartDynamicSession(msg, nil)

		return
	}

	eventsObj, _ := dipper.GetMapData(msg.Payload, "events")
	events, ok := eventsObj.([]interface{})
	if !ok || len(events) == 0 {
		return
	}
	dipper.Logger.Infof("[engine] fired events %+v", events)

	data, _ := dipper.GetMapData(msg.Payload, "data")

	for _, eventObj := range events {
		event := eventObj.(string)
		rules, ok := ruleMap[event]
		if ok && rules != nil {
			for _, rule := range rules {
				if dipper.CompareAll(data, rule.Trigger.Match) {
					dipper.Logger.Infof("[engine] raw event triggers an event %s.%s",
						rule.OriginalRule.When.Source.System,
						rule.OriginalRule.When.Source.Trigger,
					)

					envData := map[string]interface{}{
						"event": data,
					}

					firedEvent := "driver:" + event
					if rule.OriginalRule.When.Source.System != "" {
						firedEvent = rule.OriginalRule.When.Source.System + "." + rule.OriginalRule.When.Source.Trigger
					}
					ctx := rule.Trigger.ExportContext(firedEvent, envData)

					hasDo := hasWorkflowAction(rule.OriginalRule.Do)
					hasActivate := rule.OriginalRule.Activate != nil

					switch {
					case hasDo && !hasActivate:
						go sessionStore.StartSession(&rule.OriginalRule.Do, msg, ctx)
					case !hasDo && hasActivate:
						go dispatchActivate(rule.OriginalRule.Activate, msg, data, ctx, nil)
					case hasDo && hasActivate:
						ruleCopy := *rule.OriginalRule.Activate
						msgCopy := dipper.Must(dipper.MessageCopy(msg)).(*dipper.Message)
						sessionStore.StartSessionWithInitContextHook(&rule.OriginalRule.Do, msgCopy, ctx, nil, nil,
							func(created *workflow.Session) {
								persistPendingActivation(created.ID, ruleCopy, msgCopy.Labels, data, ctx)
							},
						)
					}
				}
			}
		}
	}
}

func hasWorkflowAction(wf config.Workflow) bool {
	return !reflect.DeepEqual(wf, config.Workflow{})
}

func pendingActivationKey(sessionID string) string {
	return pendingActivationKeyPrefix + sessionID
}

func persistPendingActivation(
	sessionID string,
	activate config.Activation,
	labels map[string]string,
	eventData interface{},
	eventCtx map[string]interface{},
) {
	cleanLabels := map[string]string{}
	for k, v := range labels {
		cleanLabels[k] = v
	}

	payload := pendingActivation{
		Activate: activate,
		Labels:   cleanLabels,
		Event:    eventData,
		Ctx:      eventCtx,
	}

	dipper.Must(engine.Call("cache", "save", map[string]any{
		"key":   pendingActivationKey(sessionID),
		"value": string(dipper.SerializeContent(payload)),
		"ttl":   "24h",
	}))
}

func loadPendingActivation(sessionID string) (*pendingActivation, bool) {
	raw, err := engine.Call("cache", "load", map[string]any{"key": pendingActivationKey(sessionID)})
	if err != nil || len(raw) == 0 {
		return nil, false
	}

	decoded := dipper.DeserializeContent(raw)
	data, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, false
	}

	result := &pendingActivation{}
	b := dipper.SerializeContent(data)
	dipper.Must(json.Unmarshal(b, result))

	return result, true
}

func cleanupPendingActivation(sessionID string) {
	_, _ = engine.Call("cache", "del", map[string]any{
		"key": pendingActivationKey(sessionID),
	})
}

func dispatchActivate(
	activate *config.Activation,
	msg *dipper.Message,
	eventData any,
	eventCtx map[string]interface{},
	session *workflow.Session,
) {
	if activate == nil {
		return
	}

	workflowStatus := "success"
	workflowReason := ""
	workflowOutput := any(nil)
	sourceSessionID := ""
	if session != nil {
		sourceSessionID = session.ID
		if session.CurrentMsg != nil && session.CurrentMsg.Labels != nil {
			if status, ok := session.CurrentMsg.Labels["status"]; ok && status != "" {
				workflowStatus = status
			}
			workflowReason = session.CurrentMsg.Labels["reason"]
		}
		if out, ok := session.Ctx["_output"]; ok {
			workflowOutput = out
		}
	}

	workflowData := map[string]any{
		"status":     workflowStatus,
		"reason":     workflowReason,
		"output":     workflowOutput,
		"session_id": sourceSessionID,
	}

	envData := map[string]any{
		"event":    eventData,
		"ctx":      eventCtx,
		"workflow": workflowData,
		"labels":   msg.Labels,
	}

	payload := map[string]any{
		"agent":    dipper.InterpolateStr("activate", activate.Agent, envData),
		"prompt":   dipper.InterpolateStr("activate", activate.Prompt, envData),
		"provider": dipper.InterpolateStr("activate", activate.Provider, envData),
		"event":    eventData,
		"ctx":      eventCtx,
		"workflow": workflowData,
	}
	if activate.Tools != nil {
		payload["tools"] = dipper.Interpolate("activate", activate.Tools, envData)
	}

	labels := map[string]string{}
	for k, v := range msg.Labels {
		labels[k] = v
	}
	if sourceSessionID != "" {
		labels["sourceSessionID"] = sourceSessionID
	}

	engine.getDriverRuntime(dipper.ChannelEventbus).SendMessage(&dipper.Message{
		Channel: "eventbus",
		Subject: "activate",
		Labels:  labels,
		Payload: payload,
	})
}

func continueSession(d *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	<-engine.Ready()
	msg = dipper.DeserializePayload(msg)
	sessionID, ok := msg.Labels["sessionID"]
	if !ok {
		dipper.Logger.Panic("[enigne] command return without session id")
	}
	dipper.Logger.Infof("[engine] command return for session %s", sessionID)
	go sessionStore.ContinueSession(sessionID, msg, nil)
}

// buildRuleMap : the purpose is to build a quick map from event(system/trigger) to something that is operable.
func buildRuleMap(cfg *config.Config) {
	if sessionStore == nil {
		helper := &WorkflowHelper{engine: engine, RPCCaller: engine}
		sessionStore = workflow.NewStore(helper)
		engine.Drain = sessionStore.Stop
	}

	ruleMapLock.Lock()
	defer ruleMapLock.Unlock()
	ruleMap = map[string][]*CollapsedRule{}

	for _, rule := range cfg.DataSet.Rules {
		func(rule config.Rule) {
			defer func() {
				if r := recover(); r != nil {
					dipper.Logger.Warningf("[engine] skipping invalid rule.When %+v with error %+v", rule.When, r)
				}
			}()
			rawTrigger, collapsedTrigger := config.CollapseTrigger(&rule.When, cfg.DataSet)
			dipper.Recursive(collapsedTrigger.Match, dipper.RegexParser)

			rawTriggerKey := rawTrigger.Driver + "." + rawTrigger.RawEvent
			rawRules := ruleMap[rawTriggerKey]
			rawRules = append(rawRules, &CollapsedRule{
				Trigger:      collapsedTrigger,
				OriginalRule: &rule,
			})
			ruleMap[rawTriggerKey] = rawRules
		}(rule)
	}
}

func engineMetrics() {
	if sessionStore != nil {
		engine.GaugeSet("honey.honeydipper.engine.sessions", strconv.Itoa(sessionStore.GetNumSessions(false)), []string{})
	}
}
