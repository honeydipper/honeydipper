// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package service

import (
	"testing"

	"github.com/honeyscience/honeydipper/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestReceiverCollapseTrigger(t *testing.T) {
	trigger := config.Trigger{
		Driver: "noname",
		Conditions: map[string]interface{}{
			"key1": "val1",
		},
	}

	finalTrigger, conditions := collapseTrigger(trigger, nil)
	assert.Equal(t, trigger, finalTrigger, "should collapse trigger to the raw driver/rawevent trigger 'noname'")
	assert.Equal(t, conditions, trigger.Conditions, "the collapsed condition should be the same as the raw trigger condition")
	conditions.(map[string]interface{})["key2"] = "val2"
	assert.NotEqual(t, conditions, trigger.Conditions, "the collapsed trigger should be a copy so original one wont be affected")

	cfg := &config.DataSet{
		Systems: map[string]config.System{
			"testsystem": {
				Triggers: map[string]config.Trigger{
					"testtrigger": trigger,
				},
			},
		},
	}

	trigger = config.Trigger{
		Source: config.Event{
			System:  "testsystem",
			Trigger: "testtrigger",
		},
		Conditions: map[string]interface{}{
			"key3": "val3",
		},
	}

	finalTrigger, conditions = collapseTrigger(trigger, cfg)
	assert.Equal(t, cfg.Systems["testsystem"].Triggers["testtrigger"], finalTrigger, "should collapse trigger to the raw driver/rawevent trigger 'noname'")
	assert.Equal(t, map[string]interface{}{"key1": "val1", "key3": "val3"}, conditions, "should combine the trigger chain conditions")

	trigger.Conditions.(map[string]interface{})["key1"] = "newval1"
	_, conditions = collapseTrigger(trigger, cfg)
	assert.Equal(t, map[string]interface{}{"key1": "newval1", "key3": "val3"}, conditions, "should override the conditions with inheriting trigger")
}

func TestReceiverFeatures(t *testing.T) {

}

func TestReceiverRoute(t *testing.T) {

}
