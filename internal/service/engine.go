// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package service

import (
	"strconv"
	"sync"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/driver"
	"github.com/honeydipper/honeydipper/internal/workflow"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// CollapsedRule maps the rule to its all collapsed match and exports.
type CollapsedRule struct {
	Trigger      *config.CollapsedTrigger
	OriginalRule *config.Rule
}

var ruleMapLock sync.Mutex
var ruleMap map[string][]*CollapsedRule

var engine *Service
var sessionStore *workflow.SessionStore

// StartEngine Starts the engine service
func StartEngine(cfg *config.Config) {
	sessionStore = workflow.NewSessionStore()
	sessionStore.GetConfig = func() *config.Config { return engine.config }
	sessionStore.SendMessage = func(msg *dipper.Message) {
		worker := engine.getDriverRuntime(dipper.ChannelEventbus)
		dipper.SendMessage(worker.Output, msg)
	}

	engine = NewService(cfg, "engine")
	Services["engine"] = engine

	engine.ServiceReload = buildRuleMap
	engine.EmitMetrics = engineMetrics
	engine.addResponder("broadcast:resume_session", resumeSession)
	engine.addResponder("eventbus:message", createSessions)
	engine.addResponder("eventbus:return", continueSession)

	buildRuleMap(cfg)
	engine.start()
}

func createSessions(d *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	msg = dipper.DeserializePayload(msg)
	eventsObj, _ := dipper.GetMapData(msg.Payload, "events")
	events := eventsObj.([]interface{})
	dipper.Logger.Infof("[engine] fired events %+v", events)

	data, _ := dipper.GetMapData(msg.Payload, "data")

	for _, eventObj := range events {
		event := eventObj.(string)
		rules, ok := ruleMap[event]
		if ok && rules != nil {
			for _, rule := range rules {
				dipper.Recursive(rule.Trigger.Match, engine.decryptDriverData)
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
					go sessionStore.StartSession(&rule.OriginalRule.Do, msg, ctx)
				}
			}
		}
	}
}

func continueSession(d *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	msg = dipper.DeserializePayload(msg)
	sessionID, ok := msg.Labels["sessionID"]
	if !ok {
		dipper.Logger.Panic("[enigne] command return without session id")
	}
	dipper.Logger.Infof("[engine] command return")
	go sessionStore.ContinueSession(sessionID, msg, nil)
}

// buildRuleMap : the purpose is to build a quick map from event(system/trigger) to something that is operable
func buildRuleMap(cfg *config.Config) {
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
	engine.GaugeSet("honey.honeydipper.engine.sessions", strconv.Itoa(sessionStore.Len()), []string{})
}

func resumeSession(d *driver.Runtime, m *dipper.Message) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	m = dipper.DeserializePayload(m)
	key := dipper.MustGetMapDataStr(m.Payload, "key")
	go sessionStore.ResumeSession(key, m)
}
