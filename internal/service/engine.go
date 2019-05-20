// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package service

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/internal/driver"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
)

// workflow type names
const (
	WorkflowTypeNamed    = ""
	WorkflowTypeSwitch   = "switch"
	WorkflowTypePipe     = "pipe"
	WorkflowTypeParallel = "parallel"
	WorkflowTypeIf       = "if"
	WorkflowTypeSuspend  = "suspend"
	WorkflowTypeFunction = "function"
	WorkflowTypeData     = "data"
)

// WorkflowSession is the data structure about a running workflow and its definition.
type WorkflowSession struct {
	work     []*config.Workflow
	step     int
	Type     string
	parent   string
	event    interface{}
	ctx      map[string]interface{}
	function *config.Function
	exported *map[string]interface{}
	export   map[string]interface{}
}

var sessions = map[string]*WorkflowSession{}
var suspendedSessions = map[string]string{}

// CollapsedRule maps the rule to its all collapsed match.
type CollapsedRule struct {
	Trigger      config.CollapsedTrigger
	OriginalRule *config.Rule
}

var ruleMapLock sync.Mutex
var ruleMap map[string]*[]*CollapsedRule

var engine *Service

// StartEngine Starts the engine service
func StartEngine(cfg *config.Config) {
	dipper.InitIDMap(&sessions)
	engine = NewService(cfg, "engine")
	engine.Route = engineRoute
	engine.ServiceReload = buildRuleMap
	engine.EmitMetrics = engineMetrics
	Services["engine"] = engine
	buildRuleMap(cfg)
	engine.responders["broadcast:resume_session"] = append(engine.responders["broadcast:resume_session"], resumeSession)
	engine.start()
}

func mergeWithOverride(dst *map[string]interface{}, src interface{}) {
	err := mergo.Merge(dst, src, mergo.WithOverride, mergo.WithAppendSlice)
	if err != nil {
		panic(err)
	}
	for k, v := range *dst {
		if k[0] == '*' {
			(*dst)[k[1:]] = v
			delete(*dst, k)
		}
	}
}

// putting raw data from event, function output into context as abstracted fields
func exportContext(source interface{}, envData map[string]interface{}) map[string]interface{} {
	var ctx map[string]interface{}
	var exports map[string]interface{}
	var parent interface{}

	cfg := engine.config.DataSet

	switch src := source.(type) {
	case config.Trigger:
		exports = src.Export
		if len(src.Source.System) > 0 {
			parentSys, ok := cfg.Systems[src.Source.System]
			if !ok {
				panic("unable to export context, parent system not defined")
			}
			t, ok := parentSys.Triggers[src.Source.Trigger]
			if !ok {
				panic("unable to export context, parent function not defined")
			}
			parent = t
		}
	case config.Function:
		exports = src.Export
		if len(src.Target.System) > 0 {
			parentSys, ok := cfg.Systems[src.Target.System]
			if !ok {
				panic("unable to export context, parent system not defined")
			}
			f, ok := parentSys.Functions[src.Target.Function]
			if !ok {
				panic("unable to export context, parent function not defined")
			}
			parent = f
		}
	default:
		dipper.Logger.Panicf("unable to export to context, not a trigger or function %+v", source)
	}

	if parent != nil {
		ctx = exportContext(parent, envData)
	}

	if len(exports) > 0 {
		if ctx == nil {
			ctx = map[string]interface{}{}
		}

		envData["ctx"] = ctx
		newCtx := dipper.Interpolate(exports, envData)
		mergeWithOverride(&ctx, newCtx)
	}

	return ctx
}

func engineRoute(msg *dipper.Message) (ret []RoutedMessage) {
	dipper.Logger.Infof("[engine] routing message %s.%s", msg.Channel, msg.Subject)

	if msg.Channel == dipper.ChannelEventbus && msg.Subject == "message" {
		msg = dipper.DeserializePayload(msg)
		eventsObj, _ := dipper.GetMapData(msg.Payload, "events")
		events := eventsObj.([]interface{})
		dipper.Logger.Infof("[engine] fired events %+v", events)

		data, _ := dipper.GetMapData(msg.Payload, "data")

		for _, eventObj := range events {
			event := eventObj.(string)
			crs, ok := ruleMap[event]
			if ok && crs != nil {
				for _, cr := range *crs {
					dipper.Recursive(cr.Trigger.Match, engine.decryptDriverData)
					if dipper.CompareAll(data, cr.Trigger.Match) {
						dipper.Logger.Infof("[engine] raw event triggers an event %s.%s",
							cr.OriginalRule.When.Source.System,
							cr.OriginalRule.When.Source.Trigger,
						)
						envData := map[string]interface{}{
							"event": data,
						}
						ctx := exportContext(cr.OriginalRule.When, envData)
						go executeWorkflow("", &cr.OriginalRule.Do, msg, &ctx)
					}
				}
			}
		}
	} else if msg.Channel == dipper.ChannelEventbus && msg.Subject == dipper.EventbusReturn {
		msg = dipper.DeserializePayload(msg)
		sessionID, ok := msg.Labels["sessionID"]
		if !ok {
			dipper.Logger.Panic("[enigne] command return without session id")
		}
		dipper.Logger.Infof("[engine] command return")
		go continueWorkflow(sessionID, msg, nil)
	}

	return ret
}

func continueWorkflow(sessionID string, msg *dipper.Message, exported *map[string]interface{}) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	session := dipper.IDMapGet(&sessions, sessionID).(*WorkflowSession)

	if session.function != nil {
		envData := map[string]interface{}{
			"event":  session.event,
			"ctx":    session.ctx,
			"wfdata": session.ctx,
			"labels": msg.Labels,
		}

		if msg.Payload == nil {
			envData["data"] = map[string]interface{}{}
		} else {
			envData["data"] = msg.Payload
		}

		newCtx := exportContext(*session.function, envData)
		session.function = nil
		mergeWithOverride(&session.ctx, newCtx)
		if session.exported == nil {
			session.exported = &newCtx
		} else {
			mergeWithOverride(session.exported, newCtx)
		}
	}

	if exported != nil {
		mergeWithOverride(&session.ctx, *exported)
		if session.exported == nil {
			session.exported = exported
		} else {
			mergeWithOverride(session.exported, *exported)
		}
	}

	switch session.Type {
	case WorkflowTypeNamed:
		dipper.Logger.Infof("[engine] named session completed %s", sessionID)
		terminateWorkflow(sessionID, msg)
		return

	case WorkflowTypePipe:
		session.step++
		if session.step >= len(session.work) {
			dipper.Logger.Infof("[engine] pipe session completed %s", sessionID)
			terminateWorkflow(sessionID, msg)
			return
		}
		executeWorkflow(sessionID, session.work[session.step], msg, nil)

	case WorkflowTypeIf:
		dipper.Logger.Infof("[engine] if session completed %s", sessionID)
		terminateWorkflow(sessionID, msg)
		return

	case WorkflowTypeSwitch:
		dipper.Logger.Infof("[engine] switch session completed %s", sessionID)
		terminateWorkflow(sessionID, msg)
		return

	case WorkflowTypeParallel:
		session.step++
		if session.step == len(session.work) {
			dipper.Logger.Infof("[engine] parallel session completed %s", sessionID)
			terminateWorkflow(sessionID, msg)
		}
		return
	}
}

// to be refactored to simpler function or functions
//nolint:gocyclo
func executeWorkflow(sessionID string, wf *config.Workflow, msg *dipper.Message, ctx *map[string]interface{}) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	if len(sessionID) > 0 {
		defer func() {
			if r := recover(); r != nil {
				dipper.Logger.Warningf("[engine] workflow session terminated abnormally %s", sessionID)
				terminateWorkflow(sessionID, &dipper.Message{
					Channel: dipper.ChannelEventbus,
					Subject: dipper.EventbusReturn,
					Labels: map[string]string{
						"status": "blocked",
						"reason": fmt.Sprintf("%+v", r),
					},
					Payload: map[string]interface{}{},
				})
				panic(r)
			}
		}()
	}

	data := msg.Payload
	if msg.Subject == dipper.EventbusMessage {
		data, _ = dipper.GetMapData(msg.Payload, "data")
	}

	if data == nil {
		data = map[string]interface{}{}
	}

	envData := map[string]interface{}{
		"data":   data,
		"labels": msg.Labels,
	}
	var parentSession *WorkflowSession
	if sessionID != "" {
		parentSession = dipper.IDMapGet(&sessions, sessionID).(*WorkflowSession)
		ctx = &parentSession.ctx
		envData["wfdata"] = *ctx
		envData["ctx"] = *ctx
		envData["event"] = parentSession.event
	} else {
		envData["event"] = data
		envData["wfdata"] = *ctx
		envData["ctx"] = *ctx
	}

	w := interpolateWorkflow(wf, envData)

	var session = &WorkflowSession{
		Type:   w.Type,
		parent: sessionID,
		export: w.Export,
	}
	if parentSession != nil {
		session.event = parentSession.event
		newCtx, _ := dipper.DeepCopy(parentSession.ctx)
		mergeWithOverride(&newCtx, w.Data)
		session.ctx = newCtx
	} else {
		session.event = envData["event"]
		session.ctx = *ctx
		mergeWithOverride(&session.ctx, w.Data)
	}

	envData["wfdata"] = session.ctx
	envData["ctx"] = session.ctx

	switch w.Type {
	case WorkflowTypeNamed:
		next, ok := engine.config.DataSet.Workflows[w.Content.(string)]
		if !ok {
			dipper.Logger.Panicf("[engine] named workflow not found: %s", w.Content.(string))
		}
		childID := dipper.IDMapPut(&sessions, session)
		dipper.Logger.Infof("[engine] starting named session %s %s", w.Content.(string), childID)

		executeWorkflow(childID, &next, msg, nil)

	case WorkflowTypeFunction:
		function := config.Function{}
		err := mapstructure.Decode(w.Content, &function)
		if err != nil {
			dipper.Logger.Panicf("[engine] invalid function definition %+v", err)
		}

		dipper.Logger.Infof("[engine] function from workflow %+v", function)

		worker := engine.getDriverRuntime(dipper.ChannelEventbus)
		payload := map[string]interface{}{}
		payload["function"] = function
		if msg.Payload != nil {
			if msg.Subject == dipper.EventbusReturn {
				payload["data"] = msg.Payload
			} else {
				payload["data"] = msg.Payload.(map[string]interface{})["data"]
			}
		}
		payload["event"] = session.event
		payload["ctx"] = session.ctx
		cmdmsg := &dipper.Message{
			Channel: dipper.ChannelEventbus,
			Subject: "command",
			Payload: payload,
			Labels:  msg.Labels,
		}
		if len(sessionID) > 0 {
			if cmdmsg.Labels == nil {
				cmdmsg.Labels = map[string]string{
					"sessionID": sessionID,
				}
			} else {
				cmdmsg.Labels["sessionID"] = sessionID
			}
			parentSession.function = &function
		}
		dipper.SendMessage(worker.Output, cmdmsg)

	case WorkflowTypePipe:
		for _, v := range w.Content.([]interface{}) {
			child := &config.Workflow{}
			err := mapstructure.Decode(v, child)
			if err != nil {
				panic(err)
			}
			session.work = append(session.work, child)
		}
		sessionID := dipper.IDMapPut(&sessions, session)
		// TODO: global session timeout should be handled

		dipper.Logger.Infof("[engine] starting pipe session %s", sessionID)
		executeWorkflow(sessionID, session.work[0], msg, nil)

	case WorkflowTypeSwitch:
		var choices = map[string]config.Workflow{}
		if w.Condition == "" {
			dipper.Logger.Panicf("[engine] no condition speicified for switch workflow")
		}
		err := mapstructure.Decode(w.Content, &choices)
		if err != nil {
			panic(err)
		}
		value := dipper.InterpolateStr(w.Condition, envData)
		dipper.Logger.Debugf("[engine] switch workflow %s condition : %s", sessionID, value)
		selected, ok := choices[value]
		if !ok {
			selected, ok = choices["*"]
		}

		if ok {
			childSessionID := dipper.IDMapPut(&sessions, session)
			dipper.Logger.Infof("[engine] starting switch session %s", childSessionID)
			executeWorkflow(childSessionID, &selected, msg, nil)
		}

	case WorkflowTypeIf:
		var choices []config.Workflow
		err := mapstructure.Decode(w.Content, &choices)
		if err != nil {
			panic(err)
		}
		for _, choice := range choices {
			var current = choice
			session.work = append(session.work, &current)
		}

		if w.Condition == "" {
			dipper.Logger.Panicf("[engine] no condition speicified for if workflow")
		}
		value := dipper.InterpolateStr(w.Condition, envData)
		dipper.Logger.Debugf("[engine] check condition workflow for %s : %s", sessionID, value)

		if test, err := strconv.ParseBool(value); err != nil || !test { // not true
			if len(choices) > 1 {
				childSessionID := dipper.IDMapPut(&sessions, session)
				dipper.Logger.Infof("[engine] starting if session %s", childSessionID)
				executeWorkflow(childSessionID, session.work[1], msg, nil)
			} else if sessionID != "" {
				continueWorkflow(sessionID, &dipper.Message{
					Labels: map[string]string{
						"status":          "skip",
						"previous_status": msg.Labels["status"],
						"reason":          msg.Labels["reason"],
					},
					Payload: msg.Payload,
				}, nil)
			}
		} else { // true
			childSessionID := dipper.IDMapPut(&sessions, session)
			dipper.Logger.Infof("[engine] starting if session %s", childSessionID)
			executeWorkflow(childSessionID, session.work[0], msg, nil)
		}

	case WorkflowTypeParallel:
		var threads []config.Workflow
		err := mapstructure.Decode(w.Content, &threads)
		if err != nil {
			panic(err)
		}
		for _, thread := range threads {
			var current = thread
			session.work = append(session.work, &current)
		}
		childSessionID := dipper.IDMapPut(&sessions, session)
		dipper.Logger.Infof("[engine] starting parallel session %s", childSessionID)
		for _, cw := range session.work {
			var current = cw
			mcopy, err := dipper.MessageCopy(msg)
			if err != nil {
				panic(err)
			}
			go executeWorkflow(childSessionID, current, mcopy, nil)
		}

	case WorkflowTypeSuspend:
		if len(sessionID) > 0 {
			key, ok := w.Content.(string)
			if !ok || len(key) == 0 {
				dipper.Logger.Panicf("[engine] suspending session requires a key for %+v", sessionID)
			}
			_, ok = suspendedSessions[key]
			if ok {
				dipper.Logger.Panicf("[engine] suspending session encounter a duplicate key for %+v", sessionID)
			}
			suspendedSessions[key] = sessionID
			dipper.Logger.Infof("[engine] suspending session %+v", sessionID)
			var d time.Duration
			if timeout, ok := dipper.GetMapData(w.Data, "timeout"); ok {
				if timeoutSec, ok := timeout.(int); ok {
					d = time.Duration(timeoutSec) * time.Second
				} else {
					var err error
					if d, err = time.ParseDuration(timeout.(string)); err != nil {
						dipper.Logger.Panicf("[engine] fail to time.ParseDuration timeout %+v", sessionID)
					}
				}
				if d > 0 {
					daemon.Children.Add(1)
					go func() {
						defer daemon.Children.Done()
						defer dipper.SafeExitOnError("[engine] resuming session on timeout failed %+v", sessionID)
						<-time.After(d)
						dipper.Logger.Infof("[engine] resuming session on timeout %+v", sessionID)
						payload, _ := dipper.GetMapData(w.Data, "payload")
						labels := dipper.MustGetMapData(w.Data, "labels")

						resumeSession(nil, &dipper.Message{
							Payload: map[string]interface{}{
								"key":     key,
								"payload": payload,
								"labels":  labels,
							},
						})
					}()
				}
			}
		} else {
			dipper.Logger.Panicf("[engine] can not suspend without a session")
		}

	case WorkflowTypeData:
		retData := dipper.Interpolate(w.Content, envData)
		continueWorkflow(sessionID, &dipper.Message{
			Labels: map[string]string{
				"status":          "success",
				"previous_status": msg.Labels["status"],
				"reason":          msg.Labels["reason"],
			},
			Payload: retData,
		}, nil)

	default:
		dipper.Logger.Panicf("[engine] unknown workflow type %s", w.Type)
	}
}

func terminateWorkflow(sessionID string, msg *dipper.Message) {
	session := dipper.IDMapGet(&sessions, sessionID).(*WorkflowSession)
	if session != nil {
		dipper.IDMapDel(&sessions, sessionID)
		if session.parent != "" {
			if len(session.export) > 0 {
				envData := map[string]interface{}{
					"event":  session.event,
					"labels": msg.Labels,
					"ctx":    session.ctx,
					"wfdata": session.ctx,
				}
				if msg.Payload == nil {
					envData["data"] = map[string]interface{}{}
				} else {
					envData["data"] = msg.Payload
				}
				wfExported := dipper.Interpolate(session.export, envData).(map[string]interface{})
				if session.exported == nil {
					session.exported = &wfExported
				} else {
					mergeWithOverride(session.exported, wfExported)
				}
			}
			go continueWorkflow(session.parent, msg, session.exported)
		}
	}
	if emitter, ok := engine.driverRuntimes["emitter"]; ok && emitter.State == driver.DriverAlive {
		engine.CounterIncr("honey.honeydipper.engine.workflows", []string{
			"status:" + msg.Labels["status"],
		})
	}
}

// buildRuleMap : the purpose is to build a quick map from event(system/trigger) to something that is operable
func buildRuleMap(cfg *config.Config) {
	ruleMapLock.Lock()
	ruleMap = map[string]*[]*CollapsedRule{}
	defer ruleMapLock.Unlock()

	for _, ruleInConfig := range cfg.DataSet.Rules {
		var rule = ruleInConfig
		func() {
			defer func() {
				if r := recover(); r != nil {
					dipper.Logger.Warningf("[engine] skipping invalid rule.When %+v with error %+v", rule.When, r)
				}
			}()
			rawTrigger, collapsedTrigger := config.CollapseTrigger(rule.When, cfg.DataSet)
			dipper.Recursive(collapsedTrigger.Match, dipper.RegexParser)

			rawTriggerKey := rawTrigger.Driver + "." + rawTrigger.RawEvent
			rawRules, ok := ruleMap[rawTriggerKey]
			if !ok {
				rawRules = &[]*CollapsedRule{}
			}
			*rawRules = append(*rawRules, &CollapsedRule{
				Trigger:      collapsedTrigger,
				OriginalRule: &rule,
			})
			if !ok {
				ruleMap[rawTriggerKey] = rawRules
			}
		}()
	}
}

func interpolateWorkflow(v *config.Workflow, data interface{}) *config.Workflow {
	ret := config.Workflow{
		Type:   v.Type,
		Export: v.Export,
	}
	switch v.Type {
	case WorkflowTypeNamed:
		ret.Content = dipper.InterpolateStr(v.Content.(string), data)
	case WorkflowTypeSuspend:
		ret.Content = dipper.InterpolateStr(v.Content.(string), data)
	case WorkflowTypeFunction:
		dipper.Logger.Debugf("[engine] interpolate run into function %+v", v)
		newContent, err := dipper.DeepCopy(v.Content.(map[string]interface{}))
		if err != nil {
			panic(fmt.Errorf("unable to copy function in workflow"))
		}
		if driverName, ok := newContent["driver"]; ok {
			newContent["driver"] = dipper.InterpolateStr(driverName.(string), data)
		}
		if rawAction, ok := newContent["rawAction"]; ok {
			newContent["rawAction"] = dipper.InterpolateStr(rawAction.(string), data)
		}
		if target, ok := newContent["target"]; ok {
			newContent["target"] = dipper.Interpolate(target, data)
		}
		ret.Content = newContent
	case WorkflowTypeSwitch:
		var branches = map[string]interface{}{}
		for key, branch := range v.Content.(map[string]interface{}) {
			if _, ok := branch.(string); ok {
				branches[key] = dipper.Interpolate(branch, data)
			} else {
				branches[key] = branch
			}
		}
		ret.Content = branches
	case WorkflowTypeData:
		// defer the interpolation for pure data workflow
		ret.Content = v.Content
	default:
		var worklist []interface{}
		for i, work := range v.Content.([]interface{}) {
			if _, ok := work.(string); ok {
				data.(map[string]interface{})["index"] = i
				worklist = append(worklist, dipper.Interpolate(work, data))
			} else {
				worklist = append(worklist, work)
			}
		}
		ret.Content = worklist
	}
	if v.Type == WorkflowTypeIf || v.Type == WorkflowTypeSwitch {
		ret.Condition = v.Condition
	}
	if v.Data != nil {
		ret.Data = dipper.Interpolate(v.Data, data).(map[string]interface{})
	}
	return &ret
}

func engineMetrics() {
	engine.GaugeSet("honey.honeydipper.engine.sessions", strconv.Itoa(len(sessions)), []string{})
}

func resumeSession(d *driver.Runtime, m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	key := dipper.MustGetMapDataStr(m.Payload, "key")
	sessionID, ok := suspendedSessions[key]
	if ok {
		delete(suspendedSessions, key)
		sessionPayload, _ := dipper.GetMapData(m.Payload, "payload")
		sessionLabels := map[string]string{}
		if labels, ok := dipper.GetMapData(m.Payload, "labels"); ok {
			err := mapstructure.Decode(labels, &sessionLabels)
			if err != nil {
				panic(err)
			}
		}
		go continueWorkflow(sessionID, &dipper.Message{
			Subject: dipper.EventbusReturn,
			Labels:  sessionLabels,
			Payload: sessionPayload,
		}, nil)
	}
}
