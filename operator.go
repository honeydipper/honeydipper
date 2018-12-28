package main

import (
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
)

var operator *Service

func startOperator(cfg *Config) {
	operator = NewService(cfg, "operator")
	operator.Route = operatorRoute
	Services["operator"] = operator
	operator.start()
}

func operatorRoute(msg *dipper.Message) (ret []RoutedMessage) {
	log.Infof("[operator] routing message %s.%s", msg.Channel, msg.Subject)
	defer dipper.SafeExitOnError("[operator] continue on processing messages")
	if msg.Channel == "eventbus" && msg.Subject == "command" {
		defer func() {
			if r := recover(); r != nil {
				if sessionID, ok := msg.Labels["sessionID"]; ok && sessionID != "" {
					newLabels := msg.Labels
					newLabels["status"] = "blocked"
					newLabels["reason"] = fmt.Sprintf("%+v", r)
					eventbus := operator.getDriverRuntime("eventbus")
					dipper.SendMessage(eventbus.output, &dipper.Message{
						Channel: "eventbus",
						Subject: "return",
						Labels:  newLabels,
					})
				}
				panic(r)
			}
		}()

		var driver string
		var params map[string]interface{}
		msg = dipper.DeserializePayload(msg)
		function := Function{}
		data, _ := dipper.GetMapData(msg.Payload, "data")
		event, _ := dipper.GetMapData(msg.Payload, "event")
		wfdata, _ := dipper.GetMapData(msg.Payload, "wfdata")
		funcDef, ok := dipper.GetMapData(msg.Payload, "function")
		if !ok {
			log.Panicf("[operator] no function received")
		}
		err := mapstructure.Decode(funcDef, &function)
		if err != nil {
			log.Panicf("[operator] invalid function received")
		}

		log.Debugf("[operator] collapsing function %s %s %+v", function.Target.System, function.Target.Function, function.Parameters)
		driver, rawaction, params, sysData := collapseFunction(nil, &function)
		log.Debugf("[operator] collapsed function %s %s %+v", driver, rawaction, params)
		dipper.Recursive(sysData, operator.decryptDriverData)

		worker := operator.getDriverRuntime("driver:" + driver)
		finalParams := params
		if params != nil {
			finalParams = dipper.Interpolate(params, map[string]interface{}{
				"sysData": sysData,
				"data":    data,
				"event":   event,
				"labels":  msg.Labels,
				"wfdata":  wfdata,
				"params":  params,
			}).(map[string]interface{})
		}
		dipper.Recursive(finalParams, operator.decryptDriverData)

		msg.Payload = finalParams
		if msg.Labels == nil {
			msg.Labels = map[string]string{}
		}
		msg.Labels["method"] = rawaction

		ret = []RoutedMessage{
			{
				driverRuntime: worker,
				message:       msg,
			},
		}
	} else if msg.Channel == "eventbus" && msg.Subject == "return" {
		ret = []RoutedMessage{
			{
				driverRuntime: operator.getDriverRuntime("eventbus"),
				message:       msg,
			},
		}
	}
	return ret
}

func collapseFunction(s *System, f *Function) (string, string, map[string]interface{}, map[string]interface{}) {
	var sysData map[string]interface{}
	var params map[string]interface{}
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
		driver, rawaction, params, sysData = collapseFunction(&subSystem, &subFunction)
	} else {
		driver = f.Driver
		rawaction = f.RawAction
		if len(f.Target.System) > 0 {
			log.Panicf("[operator] function cannot have both driver and target %s.%s %s", f.Target.System, f.Target.Function, driver)
		}
	}

	if s != nil && s.Data != nil {
		currentSysDataCopy, _ := dipper.DeepCopy(s.Data)
		if sysData == nil {
			sysData = map[string]interface{}{}
		}
		err := mergo.Merge(&sysData, currentSysDataCopy, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			log.Panicf("[operator] unable to merge parameters %+v", err)
		}
	}
	if f.Parameters != nil {
		currentParamCopy, _ := dipper.DeepCopy(f.Parameters)
		if params == nil {
			params = map[string]interface{}{}
		}
		err := mergo.Merge(&params, currentParamCopy, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			log.Panicf("[operator] unable to merge parameters %+v", err)
		}
	}

	return driver, rawaction, params, sysData
}
