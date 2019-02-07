package service

import (
	"fmt"
	"github.com/honeyscience/honeydipper/internal/config"
	"github.com/honeyscience/honeydipper/pkg/dipper"
	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
)

var operator *Service

// StartOperator starts the operator service
func StartOperator(cfg *config.Config) {
	operator = NewService(cfg, "operator")
	operator.Route = operatorRoute
	Services["operator"] = operator
	operator.start()
}

func operatorRoute(msg *dipper.Message) (ret []RoutedMessage) {
	dipper.Logger.Infof("[operator] routing message %s.%s", msg.Channel, msg.Subject)
	defer dipper.SafeExitOnError("[operator] continue on processing messages")
	if msg.Channel == "eventbus" && msg.Subject == "command" {
		defer func() {
			if r := recover(); r != nil {
				if sessionID, ok := msg.Labels["sessionID"]; ok && sessionID != "" {
					newLabels := msg.Labels
					newLabels["status"] = "blocked"
					newLabels["reason"] = fmt.Sprintf("%+v", r)
					eventbus := operator.getDriverRuntime("eventbus")
					dipper.SendMessage(eventbus.Output, &dipper.Message{
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
		dipper.Logger.Debugf("[operator] function call payload %+v", msg.Payload)
		function := config.Function{}
		data, _ := dipper.GetMapData(msg.Payload, "data")
		event, _ := dipper.GetMapData(msg.Payload, "event")
		wfdata, _ := dipper.GetMapData(msg.Payload, "wfdata")
		funcDef, ok := dipper.GetMapData(msg.Payload, "function")
		if !ok {
			dipper.Logger.Panicf("[operator] no function received")
		}
		err := mapstructure.Decode(funcDef, &function)
		if err != nil {
			dipper.Logger.Panicf("[operator] invalid function received")
		}

		dipper.Logger.Debugf("[operator] collapsing function %s %s %+v", function.Target.System, function.Target.Function, function.Parameters)
		driver, rawaction, params, sysData := collapseFunction(nil, &function)
		dipper.Logger.Debugf("[operator] collapsed function %s %s %+v", driver, rawaction, params)

		worker := operator.getDriverRuntime("driver:" + driver)
		finalParams := params
		if params != nil {
			// interpolate twice for giving an chance for using sysData in wfdata
			if wfdata != nil {
				wfdata = dipper.Interpolate(wfdata, map[string]interface{}{
					"sysData": sysData,
					"data":    data,
					"event":   event,
					"labels":  msg.Labels,
					"wfdata":  wfdata,
					"params":  params,
				}).(map[string]interface{})
			}
			// use interpolated wfdata to assemble final params
			finalParams = dipper.Interpolate(params, map[string]interface{}{
				"sysData": sysData,
				"data":    data,
				"event":   event,
				"labels":  msg.Labels,
				"wfdata":  wfdata,
				"params":  params,
			}).(map[string]interface{})
		}
		dipper.Logger.Debugf("[operator] interpolated function call %+v", finalParams)
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

func collapseFunction(s *config.System, f *config.Function) (string, string, map[string]interface{}, map[string]interface{}) {
	var sysData map[string]interface{}
	var params map[string]interface{}
	var driver string
	var rawaction string
	if len(f.Driver) == 0 {
		subSystem, ok := operator.config.DataSet.Systems[f.Target.System]
		if !ok {
			dipper.Logger.Panicf("[operator] system not defined %s", f.Target.System)
		}
		subFunction, ok := subSystem.Functions[f.Target.Function]
		if !ok {
			dipper.Logger.Panicf("[operator] function not defined %s.%s", f.Target.System, f.Target.Function)
		}
		driver, rawaction, params, sysData = collapseFunction(&subSystem, &subFunction)
	} else {
		driver = f.Driver
		rawaction = f.RawAction
		if len(f.Target.System) > 0 {
			dipper.Logger.Panicf("[operator] function cannot have both driver and target %s.%s %s", f.Target.System, f.Target.Function, driver)
		}
	}

	if s != nil && s.Data != nil {
		dipper.Recursive(s.Data, operator.decryptDriverData)
		currentSysDataCopy, _ := dipper.DeepCopy(s.Data)
		if sysData == nil {
			sysData = map[string]interface{}{}
		}
		err := mergo.Merge(&sysData, currentSysDataCopy, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			dipper.Logger.Panicf("[operator] unable to merge parameters %+v", err)
		}
	}
	if f.Parameters != nil {
		currentParamCopy, _ := dipper.DeepCopy(f.Parameters)
		if params == nil {
			params = map[string]interface{}{}
		}
		err := mergo.Merge(&params, currentParamCopy, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			dipper.Logger.Panicf("[operator] unable to merge parameters %+v", err)
		}
	}

	return driver, rawaction, params, sysData
}
