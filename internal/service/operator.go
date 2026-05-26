// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"errors"
	"fmt"
	"html/template"
	"strings"
	"sync"

	"dario.cat/mergo"
	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/daemon"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

var (
	// ErrOperatorError is the base for all operator related error.
	ErrOperatorError = errors.New("operator error")
	drainingFuncs    = sync.WaitGroup{}

	operator *Service
)

// StartOperator starts the operator service.
func StartOperator(cfg *config.Config) {
	operator = NewService(cfg, "operator")
	operator.Route = operatorRoute
	operator.DiscoverFeatures = OperatorFeatures
	operator.Drain = drainingFuncs.Wait
	setupOperatorAPIs()
	operator.start()
}

// OperatorFeatures returns dynamic features needed by operator APIs.
func OperatorFeatures(_ *config.DataSet) map[string]interface{} {
	return map[string]interface{}{
		getSecretDriverFeature(): nil,
	}
}

func applyInterpolatedLabel(msg *dipper.Message, name, pattern string, ctx interface{}, params map[string]interface{}) {
	value := dipper.InterpolateStr("labels", pattern, map[string]interface{}{
		"ctx":    ctx,
		"params": params,
	})
	delete(msg.Labels, name)
	if value != "" {
		msg.Labels[name] = value
	}
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
			} else if agentSessionID := msg.Labels["agent_session_id"]; agentSessionID != "" {
				eventbus := operator.getDriverRuntime(dipper.ChannelEventbus)
				eventbus.SendMessage(&dipper.Message{
					Channel: dipper.ChannelEventbus,
					Subject: dipper.EventbusAgentContinue,
					Labels: map[string]string{
						"agent_session_id": agentSessionID,
						"turn_id":          msg.Labels["turn_id"],
						"tool_call_id":     msg.Labels["tool_call_id"],
						"status":           "failure",
						"reason":           fmt.Sprintf("%v", r),
					},
				})
			}
			panic(r)
		}
	}()

	<-operator.Ready()

	if msg.Labels["interrupted"] == "true" {
		delete(msg.Labels, "interrupted")
		feature := msg.Labels["feature"]
		worker := operator.getDriverRuntime(feature)

		return []RoutedMessage{
			{
				driverRuntime: worker,
				message:       msg,
			},
		}
	}

	msg = dipper.DeserializePayload(msg)
	dipper.Logger.Debugf("[operator] function call payload %+v", msg.Labels)
	function := config.Function{}
	data, _ := dipper.GetMapData(msg.Payload, "data")
	if data == nil {
		data = map[string]interface{}{}
	}
	event, _ := dipper.GetMapData(msg.Payload, "event")
	ctx, _ := dipper.GetMapData(msg.Payload, "ctx")
	funcDef := dipper.MustGetMapData(msg.Payload, "function")
	dipper.Must(mapstructure.Decode(funcDef, &function))

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
		finalParams = dipper.Interpolate("operator", params, map[string]interface{}{
			"sysData": sysData,
			"data":    data,
			"event":   event,
			"labels":  msg.Labels,
			"ctx":     ctx,
			"params":  params,
		}, template.FuncMap{
			"decrypt": func(s string) string {
				spec := ""
				switch {
				case strings.HasPrefix(s, "lookup:"):
					spec = "LOOKUP[" + s[7:] + "]"
				case strings.HasPrefix(s, "enc:"):
					spec = "ENC[" + s[4:] + "]"
				}
				if spec == "" {
					return s
				}

				d, _ := dipper.DecryptString(operator, "param", spec)

				return d
			},
		}).(map[string]interface{})
	}
	dipper.Logger.Debugf("[operator] interpolated function call %+v", finalParams)
	dipper.Recursive(finalParams, dipper.GetDecryptFunc(operator))

	msg.Payload = finalParams
	if msg.Labels == nil {
		msg.Labels = map[string]string{}
	}
	msg.Labels["method"] = rawaction
	msg.Labels["feature"] = feature
	applyInterpolatedLabel(msg, "retry", "$?ctx.retry,params.retry", ctx, finalParams)
	applyInterpolatedLabel(msg, "backoff_ms", "$?ctx.backoff_ms,params.backoff_ms", ctx, finalParams)
	applyInterpolatedLabel(msg, "timeout", "$?ctx.timeout,params.timeout", ctx, finalParams)

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
	case msg.Channel == dipper.ChannelEventbus && (msg.Subject == dipper.EventbusCommand || msg.Subject == dipper.EventbusAgentCommand):
		ret = handleEventbusCommand(msg)
	case msg.Channel == dipper.ChannelEventbus && msg.Subject == dipper.EventbusReturnInterrupted:
		retryInterruptedSession(msg)
	case msg.Channel == dipper.ChannelEventbus &&
		(msg.Subject == dipper.EventbusReturn ||
			msg.Subject == dipper.EventbusMessage ||
			msg.Subject == dipper.EventbusAgentContinue):
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
			panic(fmt.Errorf("[operator] system not defined %s", f.Target.System))
		}
		childFunction, ok := childSystem.Functions[f.Target.Function]
		if !ok {
			panic(fmt.Errorf("[operator] function not defined %s.%s", f.Target.System, f.Target.Function))
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
			panic(fmt.Errorf("[operator] function cannot have both driver and target %s.%s %s", f.Target.System, f.Target.Function, driver))
		}
	}

	if s != nil && s.Data != nil {
		currentSysDataCopy, _ := dipper.DeepCopy(s.Data)
		if sysData == nil {
			sysData = map[string]interface{}{}
		}
		err := mergo.Merge(&sysData, currentSysDataCopy, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			panic(fmt.Errorf("[operator] unable to merge parameters: %w", err))
		}
	}
	if f.Parameters != nil {
		currentParamCopy, _ := dipper.DeepCopy(f.Parameters)
		if params == nil {
			params = map[string]interface{}{}
		}
		err := mergo.Merge(&params, currentParamCopy, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			panic(fmt.Errorf("[operator] unable to merge parameters: %w", err))
		}
	}

	return driver, rawaction, params, sysData
}

func retryInterruptedSession(msg *dipper.Message) {
	drainingFuncs.Add(1)
	daemon.Go(func() {
		defer drainingFuncs.Done()
		resumeSubject := dipper.EventbusCommand
		if msg.Labels["dipper_call_subject"] == dipper.EventbusAgentCommand {
			resumeSubject = dipper.EventbusAgentCommand
		}
		delete(msg.Labels, "dipper_call_subject")
		msg.Subject = resumeSubject
		msg.Labels["interrupted"] = "true"
		if operator.drainingGroup != nil {
			// all drivers should be in draining mode before the message
			// is sent to the queue. The redisqueue driver will still
			// write to the queue but won't read from the queue. This
			// is to avoid the retried message gets picked up by the
			// redisqueue driver again.
			operator.drainingGroup.Wait()
		}
		operator.getDriverRuntime(dipper.ChannelEventbus).SendMessage(msg)
	})
}
