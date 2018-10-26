package main

import (
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/imdario/mergo"
	"log"
)

var receiver *Service

func startReceiver(cfg *Config) {
	receiver = NewService(cfg, "receiver")
	receiver.Route = receiverRoute
	receiver.DiscoverFeatures = ReceiverFeatures
	Services["receiver"] = receiver
	go receiver.start()
}

func receiverRoute(msg *dipper.Message) (ret []RoutedMessage) {
	log.Printf("[receiver] routing message %s.%s", msg.Channel, msg.Subject)
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
	var stack []interface{}
	if current.Conditions != nil {
		stack = append(stack, current.Conditions)
	}
	for len(current.Source.System) > 0 {
		if len(current.Driver) > 0 {
			log.Panicf("[receiver] a trigger cannot have both driver and source %+v", current)
		}
		current = c.Systems[current.Source.System].Triggers[current.Source.Trigger]
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
	return current, conditions
}

// ReceiverFeatures : Receiver needs to load the event drivers before hand based on the rules
func ReceiverFeatures(c *ConfigSet) map[string]interface{} {
	var dynamicData = map[string]interface{}{}
	log.Printf("rules %+v", c.Rules)
	for _, rule := range c.Rules {
		trigger, conditions := collapseTrigger(rule.When, c)

		systemName := "_"
		triggerName := rule.When.RawEvent
		if len(rule.When.Driver) == 0 {
			systemName = rule.When.Source.System
			triggerName = rule.When.Source.Trigger
		}
		if len(triggerName) == 0 {
			log.Panicf("[receiver] trigger should have a source trigger or a raw event name %+v", rule.When)
		}
		delta := map[string]interface{}{
			"driver:" + trigger.Driver: map[string]interface{}{
				systemName: map[string]interface{}{
					triggerName: conditions,
				},
			},
		}

		log.Printf("[receiver] collapsed %+v total %+v", delta, dynamicData)
		err := mergo.Merge(&dynamicData, delta, mergo.WithOverride, mergo.WithAppendSlice)
		if err != nil {
			panic(err)
		}
	}
	return dynamicData
}
