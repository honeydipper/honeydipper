package main

import (
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/mitchellh/mapstructure"
	"sync"
)

// WorkflowSession : store the workflow session data
type WorkflowSession struct {
	work   []*Workflow
	data   interface{}
	step   int
	Type   string
	parent string
}

var sessions = map[string]*WorkflowSession{}

var engine *Service
var ruleMapLock sync.Mutex
var ruleMap map[string][]*Workflow

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

	if msg.Subject == "state" {
		return ret
	}

	if msg.Channel == "eventbus" && msg.Subject == "message" {
		msg = dipper.DeserializePayload(msg)
		eventsObj, _ := dipper.GetMapData(msg.Payload, "events")
		events := eventsObj.([]interface{})
		log.Infof("[engine] fired events %+v", events)

		for _, eventObj := range events {
			event, _ := eventObj.(string)
			// TODO: ruleMap is very premitive, needs to check condition here
			workflows, _ := ruleMap[event]
			for _, workflow := range workflows {
				go executeWorkflow("", workflow, msg)
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

func executeWorkflow(sessionID string, w *Workflow, msg *dipper.Message) {
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

	switch w.Type {
	case "":
		next := engine.config.config.Workflows[w.Content.(string)]
		executeWorkflow(sessionID, &next, msg)

	case "function":
		function := Function{}
		err := mapstructure.Decode(w.Content, &function)
		if err != nil {
			log.Panicf("[engine] invalid function definition %+v", err)
		}
		log.Debugf("[engine] workflow content %+v", w.Content)
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
		var session = &WorkflowSession{
			Type:   w.Type,
			parent: sessionID,
			step:   0,
			data:   msg.Payload,
		}
		for _, v := range w.Content.([]interface{}) {
			w := &Workflow{}
			err := mapstructure.Decode(v, w)
			if err != nil {
				panic(err)
			}
			session.work = append(session.work, w)
		}
		sessionID := dipper.IDMapPut(&sessions, session)
		// TODO: global session timeout should be handled

		executeWorkflow(sessionID, session.work[0], msg)

	case "if":
		var session = &WorkflowSession{
			Type:   w.Type,
			parent: sessionID,
			step:   0,
			data:   msg.Payload,
		}
		if w.Condition == "" {
			log.Panicf("[engine] no condition speicified for if workflow")
		}

		idata := msg.Payload
		if msg.Subject != "return" {
			idata = msg.Payload.(map[string]interface{})["data"]
		}
		value := dipper.InterpolateStr(w.Condition, map[string]interface{}{
			"data":   idata,
			"labels": msg.Labels,
		})
		log.Debugf("[engine] check condition workflow for %s : %s", sessionID, value)
		var choices []Workflow
		err := mapstructure.Decode(w.Content, &choices)
		if err != nil {
			panic(err)
		}
		for _, choice := range choices {
			session.work = append(session.work, &choice)
		}
		if value == "false" || value == "0" || value == "nil" || value == "" {
			if len(choices) > 1 {
				childSessionID := dipper.IDMapPut(&sessions, session)
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
		} else {
			childSessionID := dipper.IDMapPut(&sessions, session)
			executeWorkflow(childSessionID, session.work[0], msg)
		}

	case "parallel":
		var session = &WorkflowSession{
			Type:   w.Type,
			parent: sessionID,
			step:   0,
			data:   msg.Payload,
			work:   []*Workflow{},
		}
		var threads []Workflow
		err := mapstructure.Decode(w.Content, &threads)
		if err != nil {
			panic(err)
		}
		log.Debugf("%+v", threads)
		for _, thread := range threads {
			var current = thread
			session.work = append(session.work, &current)
		}
		childSessionID := dipper.IDMapPut(&sessions, session)
		for _, cw := range session.work {
			mcopy, err := dipper.MessageCopy(msg)
			if err != nil {
				panic(err)
			}
			log.Debugf("%+v", *cw)
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
	ruleMap = map[string][]*Workflow{}
	defer ruleMapLock.Unlock()
	for _, rule := range cfg.config.Rules {
		system := rule.When.Source.System
		trigger := rule.When.Source.Trigger
		if len(system) == 0 {
			system = "_"
			trigger = rule.When.RawEvent
		}

		todo := rule.Do
		if len(rule.Do.Type) == 0 {
			todoName, ok := rule.Do.Content.(string)
			if !ok {
				log.Warningf("workflow without type should have a name in content pointing to real workflow")
				break
			}
			todo, ok = cfg.config.Workflows[todoName]
			if !ok {
				log.Warningf("workflow points to a non-exist workflow %s", todoName)
				break
			}
		}

		ruleMap[system+"."+trigger] = append(ruleMap[system+"."+trigger], &todo)
	}
}
