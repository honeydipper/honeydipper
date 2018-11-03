package main

import (
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/mitchellh/mapstructure"
	"sync"
)

var engine *Service
var ruleMapLock sync.Mutex
var ruleMap map[string][]*Workflow

func startEngine(cfg *Config) {
	engine = NewService(cfg, "engine")
	engine.Route = engineRoute
	engine.ServiceReload = buildRuleMap
	Services["engine"] = engine
	buildRuleMap(cfg)
	go engine.start()
}

func engineRoute(msg *dipper.Message) (ret []RoutedMessage) {
	log.Infof("[engine] routing message %s.%s", msg.Channel, msg.Subject)
	if msg.Channel == "eventbus" && msg.Subject == "message" {
		msg = dipper.DeserializePayload(msg)
		eventsObj, _ := dipper.GetMapData(msg.Payload, "events")
		events := eventsObj.([]interface{})
		log.Infof("[engine] fired events %+v", events)
		for _, eventObj := range events {
			event, _ := eventObj.(string)
			workflows, _ := ruleMap[event]
			for _, workflow := range workflows {
				go startWorkflow(workflow, msg)
			}
		}
	}
	return ret
}

func startWorkflow(w *Workflow, msg *dipper.Message) {
	if len(w.Type) == 0 {
		next, _ := engine.config.config.Workflows[w.Content.(string)]
		startWorkflow(&next, msg)
	} else if w.Type == "function" {
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
		payload["data"], _ = dipper.GetMapData(msg.Payload, "data")
		dipper.SendMessage(worker.output, "eventbus", "command", payload)
	}
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
