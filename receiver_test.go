// +build !integration

package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReceiverCollapseTrigger(t *testing.T) {
	trigger := Trigger{
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

	config := &ConfigSet{
		Systems: map[string]System{
			"testsystem": System{
				Triggers: map[string]Trigger{
					"testtrigger": trigger,
				},
			},
		},
	}

	trigger = Trigger{
		Source: Event{
			System:  "testsystem",
			Trigger: "testtrigger",
		},
		Conditions: map[string]interface{}{
			"key3": "val3",
		},
	}

	finalTrigger, conditions = collapseTrigger(trigger, config)
	assert.Equal(t, config.Systems["testsystem"].Triggers["testtrigger"], finalTrigger, "should collapse trigger to the raw driver/rawevent trigger 'noname'")
	assert.Equal(t, map[string]interface{}{"key1": "val1", "key3": "val3"}, conditions, "should combine the trigger chain conditions")

	trigger.Conditions.(map[string]interface{})["key1"] = "newval1"
	finalTrigger, conditions = collapseTrigger(trigger, config)
	assert.Equal(t, map[string]interface{}{"key1": "newval1", "key3": "val3"}, conditions, "should override the conditions with inheriting trigger")
}

func TestReceiverFeatures(t *testing.T) {

}

func TestReceiverRoute(t *testing.T) {

}
