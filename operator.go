package main

import (
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
	"log"
)

var operator *Service

func startOperator(cfg *Config) {
	operator = NewService(cfg, "operator")
	operator.Route = operatorRoute
	Services["operator"] = operator
	go operator.start()
}

func operatorRoute(msg *dipper.Message) (ret []RoutedMessage) {
	log.Printf("[operator] routing message %s.%s", msg.Channel, msg.Subject)
	if msg.Channel == "eventbus" && msg.Subject == "command" {
		var driver string
		var params map[string]interface{}
		msg = dipper.DeserializePayload(msg)
		function := Function{}
		eventData, _ := dipper.GetMapData(msg.Payload, "data")
		funcDef, ok := dipper.GetMapData(msg.Payload, "function")
		if !ok {
			log.Panicf("[operator] no function received")
		}
		err := mapstructure.Decode(funcDef, &function)
		if err != nil {
			log.Panicf("[operator] invalid function received")
		}

		log.Printf("[operator] collapsing function %s %s %+v", function.Target.System, function.Target.Function, function.Parameters)
		driver, rawaction, params := collapseFunction(nil, &function)
		log.Printf("[operator] collapsed function %s %s %+v", driver, rawaction, params)

		worker := operator.getDriverRuntime("driver:" + driver)
		ret = []RoutedMessage{
			{
				driverRuntime: worker,
				message: &dipper.Message{
					Channel: "execute",
					Subject: rawaction,
					Payload: map[string]interface{}{
						"param": params,
						"data":  eventData,
					},
					IsRaw: false,
				},
			},
		}
	}
	return ret
}

func collapseFunction(s *System, f *Function) (string, string, map[string]interface{}) {
	var subData map[string]interface{}
	var driver string
	var rawaction string
	if len(f.Driver) == 0 {
		subSystem, ok := operator.config.config.Systems[f.Target.System]
		if !ok {
			log.Panicf("[operator] system not defined %s", f.Target.System)
		}
		subFunction, ok := subSystem.Functions[f.Target.Function]
		if !ok {
			log.Panicf("[operator] function not defined %s.%s", f.Target.System, f.Target.Function)
		}
		driver, rawaction, subData = collapseFunction(&subSystem, &subFunction)
	} else {
		driver = f.Driver
		rawaction = f.RawAction
		if len(f.Target.System) > 0 {
			log.Panicf("[operator] function cannot have both driver and target %s.%s %s", f.Target.System, f.Target.Function, driver)
		}
	}

	if subData == nil {
		subData = map[string]interface{}{}
	}
	if s != nil {
		sysData, ok := dipper.GetMapData(s.Data, driver+".parameters")
		if ok {
			sysDataCopy, _ := dipper.DeepCopy(sysData.(map[string]interface{}))
			err := mergo.Merge(&subData, sysDataCopy, mergo.WithOverride, mergo.WithAppendSlice)
			if err != nil {
				log.Panicf("[operator] unable to merge parameters %+v", err)
			}
		}
	}
	if f.Parameters != nil {
		dataCopy, _ := dipper.DeepCopy(f.Parameters)
		err := mergo.Merge(&subData, dataCopy, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			log.Panicf("[operator] unable to merge parameters %+v", err)
		}
	}

	return driver, rawaction, subData
}
