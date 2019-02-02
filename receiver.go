package main

import (
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/imdario/mergo"
	"strconv"
)

var receiver *Service
var numCollapsedEvents int
var numDynamicFeatures int

func startReceiver(cfg *Config) {
	receiver = NewService(cfg, "receiver")
	receiver.Route = receiverRoute
	receiver.DiscoverFeatures = ReceiverFeatures
	receiver.EmitMetrics = receiverMetrics
	Services["receiver"] = receiver
	receiver.start()
}

func receiverRoute(msg *dipper.Message) (ret []RoutedMessage) {
	log.Infof("[receiver] routing message %s.%s", msg.Channel, msg.Subject)
	if msg.Channel == "eventbus" && msg.Subject == "message" {
		rtmsg := RoutedMessage{
			driverRuntime: receiver.getDriverRuntime("eventbus"),
			message:       msg,
		}
		ret = append(ret, rtmsg)
	}
	return ret
}

func collapseTrigger(t Trigger, c *ConfigSet) (Trigger, interface{}) {
	current := t
	sysData := map[string]interface{}{}
	var stack []interface{}
	if current.Conditions != nil {
		stack = append(stack, current.Conditions)
	}
	for len(current.Source.System) > 0 {
		if len(current.Driver) > 0 {
			log.Panicf("[receiver] a trigger cannot have both driver and source %+v", current)
		}
		currentSys := c.Systems[current.Source.System]
		currentSysData, _ := dipper.DeepCopy(currentSys.Data)
		err := mergo.Merge(&sysData, currentSysData, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			panic(err)
		}
		current = currentSys.Triggers[current.Source.Trigger]
		if current.Conditions != nil {
			stack = append(stack, current.Conditions)
		}
	}
	if len(current.Driver) == 0 {
		log.Panicf("[receiver] a trigger should have a driver or a source %+v", current)
	}
	conditions := map[string]interface{}{}
	for i := len(stack) - 1; i >= 0; i-- {
		c, _ := stack[i].(map[string]interface{})
		cp, _ := dipper.DeepCopy(c)
		err := mergo.Merge(&conditions, cp, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			panic(err)
		}
	}
	if len(sysData) > 0 {
		conditions = dipper.Interpolate(conditions, map[string]interface{}{
			"sysData": sysData,
		}).(map[string]interface{})
	}
	return current, conditions
}

// ReceiverFeatures : Receiver needs to load the event drivers before hand based on the rules
func ReceiverFeatures(c *ConfigSet) map[string]interface{} {
	dynamicData := map[string]interface{}{}

	numCollapsedEvents = 0
	for _, rule := range c.Rules {
		func(rule Rule) {
			defer func() {
				if r := recover(); r != nil {
					log.Warningf("[receiver] failed to process rule.When %+v with error %+v", rule.When, r)
				}
			}()
			rawTrigger, conditions := collapseTrigger(rule.When, c)
			var driverData map[string]interface{}
			data, ok := dynamicData["driver:"+rawTrigger.Driver]
			if !ok {
				driverData = map[string]interface{}{"collapsedEvents": map[string]interface{}{}}
				dynamicData["driver:"+rawTrigger.Driver] = driverData
			} else {
				driverData, _ = data.(map[string]interface{})
			}

			var eventName string
			if len(rule.When.Driver) == 0 {
				eventName = rule.When.Source.System + "." + rule.When.Source.Trigger
			} else {
				eventName = "_." + rule.When.Driver + ":" + rule.When.RawEvent
			}

			list, found := driverData["collapsedEvents"].(map[string]interface{})[eventName]
			var collapsedEvent []interface{}
			if !found {
				collapsedEvent = []interface{}{}
			} else {
				collapsedEvent, _ = list.([]interface{})
			}
			collapsedEvent = append(collapsedEvent, conditions)
			numCollapsedEvents++

			driverData["collapsedEvents"].(map[string]interface{})[eventName] = collapsedEvent

			log.Debugf("[receiver] collapsed %+v total %+v", eventName, collapsedEvent)
		}(rule)
	}
	numDynamicFeatures = len(dynamicData)
	log.Debugf("[receiver] dynamicData return: %+v", dynamicData)
	return dynamicData
}

func receiverMetrics() {
	receiver.gaugeSet("honey.honeydipper.receiver.eventTriggers", strconv.Itoa(numCollapsedEvents), []string{})
	receiver.gaugeSet("honey.honeydipper.receiver.dynamicFeatures", strconv.Itoa(numDynamicFeatures), []string{})
}
