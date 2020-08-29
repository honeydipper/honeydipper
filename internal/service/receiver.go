// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package service

import (
	"strconv"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

var (
	receiver           *Service
	numCollapsedEvents int
	numDynamicFeatures int
)

// StartReceiver starts the receiver service.
func StartReceiver(cfg *config.Config) {
	receiver = NewService(cfg, "receiver")
	receiver.Route = receiverRoute
	receiver.DiscoverFeatures = ReceiverFeatures
	receiver.EmitMetrics = receiverMetrics
	Services["receiver"] = receiver
	setupReceiverAPIs()
	receiver.start()
}

func receiverRoute(msg *dipper.Message) (ret []RoutedMessage) {
	dipper.Logger.Infof("[receiver] routing message %s.%s", msg.Channel, msg.Subject)
	if msg.Channel == "eventbus" && msg.Subject == "message" {
		rtmsg := RoutedMessage{
			driverRuntime: receiver.getDriverRuntime("eventbus"),
			message:       msg,
		}
		ret = append(ret, rtmsg)
	}
	return ret
}

// ReceiverFeatures goes through the config data to figure out what driver/feature to start for receiving events.
func ReceiverFeatures(c *config.DataSet) map[string]interface{} {
	dynamicData := map[string]interface{}{}

	numCollapsedEvents = 0
	for _, rule := range c.Rules {
		func(rule config.Rule) {
			defer func() {
				if r := recover(); r != nil {
					dipper.Logger.Warningf("[receiver] failed to process rule.When %+v with error %+v", rule.When, r)
				}
			}()
			rawTrigger, collapsed := config.CollapseTrigger(&rule.When, c)
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
			collapsedEvent = append(collapsedEvent, collapsed)
			numCollapsedEvents++

			driverData["collapsedEvents"].(map[string]interface{})[eventName] = collapsedEvent

			dipper.Logger.Debugf("[receiver] collapsed %+v total %+v", eventName, collapsedEvent)
		}(rule)
	}
	numDynamicFeatures = len(dynamicData)
	dipper.Logger.Debugf("[receiver] dynamicData return: %+v", dynamicData)
	return dynamicData
}

func receiverMetrics() {
	receiver.GaugeSet("honey.honeydipper.receiver.eventTriggers", strconv.Itoa(numCollapsedEvents), []string{})
	receiver.GaugeSet("honey.honeydipper.receiver.dynamicFeatures", strconv.Itoa(numDynamicFeatures), []string{})
}
