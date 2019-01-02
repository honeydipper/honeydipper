package main

import (
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
	"strconv"
	"sync"
)

// WorkflowSession : store the workflow session data
type WorkflowSession struct {
	work   []*Workflow
	data   interface{}
	step   int
	Type   string
	parent string
	event  interface{}
	wfdata map[string]interface{}
}

var sessions = map[string]*WorkflowSession{}

// CollapsedRule : mapping the raw trigger event to rules for testing
type CollapsedRule struct {
	Conditions   interface{}
	OriginalRule *Rule
}

var ruleMapLock sync.Mutex
var ruleMap map[string]*[]*CollapsedRule

var engine *Service

func startEngine(cfg *Config) {
	dipper.InitIDMap(&sessions)
	engine = NewService(cfg, "engine")
	engine.Route = engineRoute
	engine.ServiceReload = buildRuleMap
	Services["engine"] = engine
	buildRuleMap(cfg)
	engine.start()
}

func engineRoute(msg *dipper.Message) (ret []RoutedMessage) {
	log.Infof("[engine] routing message %s.%s", msg.Channel, msg.Subject)

	if msg.Channel == "eventbus" && msg.Subject == "message" {
		msg = dipper.DeserializePayload(msg)
		eventsObj, _ := dipper.GetMapData(msg.Payload, "events")
		events := eventsObj.([]interface{})
		log.Infof("[engine] fired events %+v", events)

		data, _ := dipper.GetMapData(msg.Payload, "data")

		for _, eventObj := range events {
			event := eventObj.(string)
			crs, ok := ruleMap[event]
			if ok && crs != nil {
				for _, cr := range *crs {
					dipper.Recursive(cr.Conditions, engine.decryptDriverData)
					if dipper.CompareAll(data, cr.Conditions) {
						log.Infof("[engine] raw event triggers an event %s.%s",
							(*cr.OriginalRule).When.Source.System,
							(*cr.OriginalRule).When.Source.Trigger,
						)
						go executeWorkflow("", &(*cr.OriginalRule).Do, msg)
					}
				}
			}
		}
	} else if msg.Channel == "eventbus" && msg.Subject == "return" {
		msg = dipper.DeserializePayload(msg)
		sessionID, ok := msg.Labels["sessionID"]
		if !ok {
			log.Panic("[enigne] command return without session id")
		}
		log.Infof("[engine] command return")
		go continueWorkflow(sessionID, msg)
	}

	return ret
}

func continueWorkflow(sessionID string, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	session := sessions[sessionID]

	switch session.Type {
	case "":
		log.Infof("[engine] named session completed %s", sessionID)
		terminateWorkflow(sessionID, msg)
		return

	case "pipe":
		session.step++
		if session.step >= len(session.work) {
			log.Infof("[engine] pipe session completed %s", sessionID)
			terminateWorkflow(sessionID, msg)
			return
		}
		executeWorkflow(sessionID, session.work[session.step], msg)

	case "if":
		log.Infof("[engine] if session completed %s", sessionID)
		terminateWorkflow(sessionID, msg)
		return

	case "parallel":
		session.step++
		if session.step == len(session.work) {
			log.Infof("[engine] parallel session completed %s", sessionID)
			terminateWorkflow(sessionID, msg)
		}
		return
	}
}

func executeWorkflow(sessionID string, wf *Workflow, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[engine] continue processing rules")
	if len(sessionID) > 0 {
		defer func() {
			if r := recover(); r != nil {
				terminateWorkflow(sessionID, &dipper.Message{
					Labels: map[string]string{
						"status": "blocked",
						"reason": fmt.Sprintf("%+v", r),
					},
				})
				panic(r)
			}
		}()
	}

	data := msg.Payload
	if msg.Subject != "return" {
		data = msg.Payload.(map[string]interface{})["data"]
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
		if err != nil {
			panic(err)
		}
		session.wfdata = wfdata
	} else {
		session.event = msg.Payload
		session.wfdata = w.Data
	}

	envData["wfdata"] = session.wfdata

	switch w.Type {
	case "":
		next, ok := engine.config.config.Workflows[w.Content.(string)]
		if !ok {
			log.Panicf("[engine] named workflow not found: %s", w.Content.(string))
		}
		childID := dipper.IDMapPut(&sessions, session)
		log.Infof("[engine] starting named session %s %s", w.Content.(string), childID)

		executeWorkflow(childID, &next, msg)

	case "function":
		function := Function{}
		err := mapstructure.Decode(w.Content, &function)
		if err != nil {
			log.Panicf("[engine] invalid function definition %+v", err)
		}

		log.Infof("[engine] function from workflow %+v", function)

		worker := engine.getDriverRuntime("eventbus")
		payload := map[string]interface{}{}
		payload["function"] = function
		if msg.Payload != nil {
			if msg.Subject == "return" {
				payload["data"] = msg.Payload
			} else {
				payload["data"] = msg.Payload.(map[string]interface{})["data"]
			}
		}
		payload["event"] = session.event
		payload["wfdata"] = session.wfdata
		cmdmsg := &dipper.Message{
			Channel: "eventbus",
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
		dipper.SendMessage(worker.output, cmdmsg)

	case "pipe":
		for _, v := range w.Content.([]interface{}) {
			child := &Workflow{}
			err := mapstructure.Decode(v, child)
			if err != nil {
				panic(err)
			}
			session.work = append(session.work, child)
		}
		sessionID := dipper.IDMapPut(&sessions, session)
		// TODO: global session timeout should be handled

		log.Infof("[engine] starting pipe session %s", sessionID)
		executeWorkflow(sessionID, session.work[0], msg)

	case "if":
		var choices []Workflow
		err := mapstructure.Decode(w.Content, &choices)
		if err != nil {
			panic(err)
		}
		for _, choice := range choices {
			session.work = append(session.work, &choice)
		}

		if w.Condition == "" {
			log.Panicf("[engine] no condition speicified for if workflow")
		}
		value := dipper.InterpolateStr(w.Condition, envData)
		log.Debugf("[engine] check condition workflow for %s : %s", sessionID, value)

		if test, err := strconv.ParseBool(value); err != nil || !test { // not true
			if len(choices) > 1 {
				childSessionID := dipper.IDMapPut(&sessions, session)
				log.Infof("[engine] starting if session %s", childSessionID)
				executeWorkflow(childSessionID, session.work[1], msg)
			} else {
				if sessionID != "" {
					continueWorkflow(sessionID, &dipper.Message{
						Labels: map[string]string{
							"status": "skip",
						},
					})
				}
			}
		} else { // true
			childSessionID := dipper.IDMapPut(&sessions, session)
			log.Infof("[engine] starting if session %s", childSessionID)
			executeWorkflow(childSessionID, session.work[0], msg)
		}

	case "parallel":
		var threads []Workflow
		err := mapstructure.Decode(w.Content, &threads)
		if err != nil {
			panic(err)
		}
		for _, thread := range threads {
			var current = thread
			session.work = append(session.work, &current)
		}
		childSessionID := dipper.IDMapPut(&sessions, session)
		log.Infof("[engine] parallel pipe session %s", childSessionID)
		for _, cw := range session.work {
			mcopy, err := dipper.MessageCopy(msg)
			if err != nil {
				panic(err)
			}
			go executeWorkflow(childSessionID, cw, mcopy)
		}

	default:
		log.Panicf("[engine] unknown workflow type %s", w.Type)
	}
}

func terminateWorkflow(sessionID string, msg *dipper.Message) {
	session, _ := sessions[sessionID]
	if session != nil {
		dipper.IDMapDel(&sessions, sessionID)
		if session.parent != "" {
			go continueWorkflow(session.parent, msg)
		}
	}
	log.Warningf("[engine] workflow session terminated %s", sessionID)
}

// buildRuleMap : the purpose is to build a quick map from event(system/trigger) to something that is operable
func buildRuleMap(cfg *Config) {
	ruleMapLock.Lock()
	ruleMap = map[string]*[]*CollapsedRule{}
	defer ruleMapLock.Unlock()

	for _, ruleInConfig := range cfg.config.Rules {
		var rule = ruleInConfig
		rawTrigger, conditions := collapseTrigger(rule.When, cfg.config)
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
	}
}

func interpolateWorkflow(v *Workflow, data interface{}) *Workflow {
	ret := Workflow{
		Type: v.Type,
	}
	switch v.Type {
	case "":
		ret.Content = dipper.InterpolateStr(v.Content.(string), data)
	case "function":
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
	if v.Type == "if" {
		ret.Condition = v.Condition
	}
	if v.Data != nil {
		ret.Data = dipper.Interpolate(v.Data, data).(map[string]interface{})
	}
	return &ret
}
