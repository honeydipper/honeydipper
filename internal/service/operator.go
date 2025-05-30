// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"errors"
	"fmt"
	"strings"

	"dario.cat/mergo"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

var (
	// ErrOperatorError is the base for all operator related error.
	ErrOperatorError = errors.New("operator error")

	operator *Service
)

// StartOperator starts the operator service.
func StartOperator(cfg *config.Config) {
	operator = NewService(cfg, "operator")
	operator.Route = operatorRoute
	operator.start()
}

// handleEventbusCommand.
func handleEventbusCommand(msg *dipper.Message) []RoutedMessage {
	defer func() {
		if r := recover(); r != nil {
			if sessionID, ok := msg.Labels["sessionID"]; ok && sessionID != "" {
				newLabels := msg.Labels
				newLabels["status"] = "error"
				newLabels["reason"] = fmt.Sprintf("%+v", r)
				eventbus := operator.getDriverRuntime(dipper.ChannelEventbus)
				eventbus.SendMessage(&dipper.Message{
					Channel: dipper.ChannelEventbus,
					Subject: dipper.EventbusReturn,
					Labels:  newLabels,
				})
			}
			panic(r)
		}
	}()

	msg = dipper.DeserializePayload(msg)
	dipper.Logger.Debugf("[operator] function call payload %+v", msg.Payload)
	function := config.Function{}
	data, _ := dipper.GetMapData(msg.Payload, "data")
	if data == nil {
		data = map[string]interface{}{}
	}
	event, _ := dipper.GetMapData(msg.Payload, "event")
	ctx, _ := dipper.GetMapData(msg.Payload, "ctx")
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

	feature := "driver:" + driver
	if strings.HasPrefix(driver, "feature:") {
		feature = driver[8:]
	}
	worker := operator.getDriverRuntime(feature)

	if worker == nil {
		panic(fmt.Errorf("%w: not defined: %s", ErrOperatorError, driver))
	}
	finalParams := params
	if params != nil {
		// interpolate twice for giving an chance for using sysData in ctx
		if ctx != nil {
			ctx = dipper.Interpolate(ctx, map[string]interface{}{
				"sysData": sysData,
				"data":    data,
				"event":   event,
				"labels":  msg.Labels,
				"ctx":     ctx,
				"params":  params,
			}).(map[string]interface{})
		}
		// use interpolated ctx to assemble final params
		finalParams = dipper.Interpolate(params, map[string]interface{}{
			"sysData": sysData,
			"data":    data,
			"event":   event,
			"labels":  msg.Labels,
			"ctx":     ctx,
			"params":  params,
		}).(map[string]interface{})
	}
	dipper.Logger.Debugf("[operator] interpolated function call %+v", finalParams)
	dipper.Recursive(finalParams, dipper.GetDecryptFunc(operator))

	msg.Payload = finalParams
	if msg.Labels == nil {
		msg.Labels = map[string]string{}
	}
	msg.Labels["method"] = rawaction
	retry := dipper.InterpolateStr("$?ctx.retry,params.retry", map[string]interface{}{
		"ctx":    ctx,
		"params": finalParams,
	})
	delete(msg.Labels, "retry")
	if retry != "" {
		msg.Labels["retry"] = retry
	}
	backoff := dipper.InterpolateStr("$?ctx.backoff_ms,params.backoff_ms", map[string]interface{}{
		"ctx":    ctx,
		"params": finalParams,
	})
	delete(msg.Labels, "backoff_ms")
	if backoff != "" {
		msg.Labels["backoff_ms"] = backoff
	}
	timeout := dipper.InterpolateStr("$?ctx.timeout,params.timeout", map[string]interface{}{
		"ctx":    ctx,
		"params": finalParams,
	})
	delete(msg.Labels, "timeout")
	if timeout != "" {
		msg.Labels["timeout"] = timeout
	}

	return []RoutedMessage{
		{
			driverRuntime: worker,
			message:       msg,
		},
	}
}

func operatorRoute(msg *dipper.Message) (ret []RoutedMessage) {
	dipper.Logger.Infof("[operator] routing message %s.%s", msg.Channel, msg.Subject)
	defer dipper.SafeExitOnError("[operator] continue on processing messages")
	switch {
	case msg.Channel == dipper.ChannelEventbus && msg.Subject == dipper.EventbusCommand:
		ret = handleEventbusCommand(msg)
	case msg.Channel == dipper.ChannelEventbus && (msg.Subject == dipper.EventbusReturn || msg.Subject == dipper.EventbusMessage):
		ret = []RoutedMessage{
			{
				driverRuntime: operator.getDriverRuntime(dipper.ChannelEventbus),
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
		childSystem, ok := operator.config.DataSet.Systems[f.Target.System]
		if !ok {
			dipper.Logger.Panicf("[operator] system not defined %s", f.Target.System)
		}
		childFunction, ok := childSystem.Functions[f.Target.Function]
		if !ok {
			dipper.Logger.Panicf("[operator] function not defined %s.%s", f.Target.System, f.Target.Function)
		}
		driver, rawaction, params, sysData = collapseFunction(&childSystem, &childFunction)

		// split subsystem data from system
		subsystems := strings.Split(f.Target.Function, ".")
		for _, subsystem := range subsystems[:len(subsystems)-1] {
			parent := sysData
			sysData = parent[subsystem].(map[string]interface{})
			sysData["parent"] = parent
		}
	} else {
		driver = f.Driver
		rawaction = f.RawAction
		if len(f.Target.System) > 0 {
			dipper.Logger.Panicf("[operator] function cannot have both driver and target %s.%s %s", f.Target.System, f.Target.Function, driver)
		}
	}

	if s != nil && s.Data != nil {
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
