package service

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/honeyscience/honeydipper/internal/config"
	"github.com/honeyscience/honeydipper/internal/driver"
	"github.com/honeyscience/honeydipper/pkg/dipper"
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
	work   []*config.Workflow
	data   interface{}
	step   int
	Type   string
	parent string
	event  interface{}
	wfdata map[string]interface{}
}

var sessions = map[string]*WorkflowSession{}
var suspendedSessions = map[string]string{}

// CollapsedRule maps the rule to its all collapsed conditions.
type CollapsedRule struct {
	// The collapsed conditions
	Conditions   interface{}
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
					dipper.Recursive(cr.Conditions, engine.decryptDriverData)
					if dipper.CompareAll(data, cr.Conditions) {
						dipper.Logger.Infof("[engine] raw event triggers an event %s.%s",
							cr.OriginalRule.When.Source.System,
							cr.OriginalRule.When.Source.Trigger,
						)
						go executeWorkflow("", &cr.OriginalRule.Do, msg)
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
		go continueWorkflow(sessionID, msg)
	}

	return ret
}

func continueWorkflow(sessionID string, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	session := sessions[sessionID]

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
		executeWorkflow(sessionID, session.work[session.step], msg)

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
func executeWorkflow(sessionID string, wf *config.Workflow, msg *dipper.Message) {
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
	if msg.Subject != dipper.EventbusReturn {
		data, _ = dipper.GetMapData(msg.Payload, "data")
	}

	envData := map[string]interface{}{
		"data":   data,
		"labels": msg.Labels,
	}
	var parentSession *WorkflowSession
	if sessionID != "" {
		parentSession = sessions[sessionID]
		envData["wfdata"] = parentSession.wfdata
		envData["event"] = parentSession.event
	} else {
		envData["event"] = data
	}

	w := interpolateWorkflow(wf, envData)

	var session = &WorkflowSession{
		Type:   w.Type,
		parent: sessionID,
		data:   msg.Payload,
	}
	if parentSession != nil {
		session.event = parentSession.event
		wfdata, _ := dipper.DeepCopy(parentSession.wfdata)
		err := mergo.Merge(&wfdata, w.Data, mergo.WithOverride, mergo.WithAppendSlice)
		for k, v := range wfdata {
			if k[0] == '*' {
				wfdata[k[1:]] = v
				delete(wfdata, k)
			}
		}
		if err != nil {
			panic(err)
		}
		session.wfdata = wfdata
	} else {
		session.event = envData["event"]
		session.wfdata = w.Data
	}

	envData["wfdata"] = session.wfdata

	switch w.Type {
	case WorkflowTypeNamed:
		next, ok := engine.config.DataSet.Workflows[w.Content.(string)]
		if !ok {
			dipper.Logger.Panicf("[engine] named workflow not found: %s", w.Content.(string))
		}
		childID := dipper.IDMapPut(&sessions, session)
		dipper.Logger.Infof("[engine] starting named session %s %s", w.Content.(string), childID)

		executeWorkflow(childID, &next, msg)

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
		payload["wfdata"] = session.wfdata
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
		executeWorkflow(sessionID, session.work[0], msg)

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
			executeWorkflow(childSessionID, &selected, msg)
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
				executeWorkflow(childSessionID, session.work[1], msg)
			} else if sessionID != "" {
				continueWorkflow(sessionID, &dipper.Message{
					Labels: map[string]string{
						"status":          "skip",
						"previous_status": msg.Labels["status"],
						"reason":          msg.Labels["reason"],
					},
					Payload: msg.Payload,
				})
			}
		} else { // true
			childSessionID := dipper.IDMapPut(&sessions, session)
			dipper.Logger.Infof("[engine] starting if session %s", childSessionID)
			executeWorkflow(childSessionID, session.work[0], msg)
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
			go executeWorkflow(childSessionID, current, mcopy)
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
		})

	default:
		dipper.Logger.Panicf("[engine] unknown workflow type %s", w.Type)
	}
}

func terminateWorkflow(sessionID string, msg *dipper.Message) {
	session := sessions[sessionID]
	if session != nil {
		dipper.IDMapDel(&sessions, sessionID)
		if session.parent != "" {
			go continueWorkflow(session.parent, msg)
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
			rawTrigger, conditions := collapseTrigger(rule.When, cfg.DataSet)
			dipper.Recursive(conditions, dipper.RegexParser)
			// collpaseTrigger function is in receiver.go, might need to be moved

			rawTriggerKey := rawTrigger.Driver + "." + rawTrigger.RawEvent
			rawRules, ok := ruleMap[rawTriggerKey]
			if !ok {
				rawRules = &[]*CollapsedRule{}
			}
			*rawRules = append(*rawRules, &CollapsedRule{
				Conditions:   conditions,
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
		Type: v.Type,
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
		})
	}
}
