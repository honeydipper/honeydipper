// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReceiverCollapseTrigger(t *testing.T) {
	trigger := Trigger{
		Driver: "noname",
		Match: map[string]interface{}{
			"key1": "val1",
		},
	}

	finalTrigger, collapsed := CollapseTrigger(&trigger, nil)
	assert.Equal(t, trigger, finalTrigger, "should collapse trigger to the raw driver/rawevent trigger 'noname'")
	assert.Equal(t, collapsed.Match, trigger.Match, "the collapsed condition should be the same as the raw trigger condition")
	collapsed.Match["key2"] = "val2"
	assert.NotEqual(t, collapsed.Match, trigger.Match, "the collapsed trigger should be a copy so original one wont be affected")

	cfg := &DataSet{
		Systems: map[string]System{
			"testsystem": {
				Triggers: map[string]Trigger{
					"testtrigger": trigger,
				},
			},
		},
	}

	extendingTrigger := Trigger{
		Source: Event{
			System:  "testsystem",
			Trigger: "testtrigger",
		},
		Match: map[string]interface{}{
			"key3": "val3",
		},
	}

	finalTrigger, collapsed = CollapseTrigger(&extendingTrigger, cfg)
	assert.Equal(t, trigger, finalTrigger, "should collapse trigger to the raw driver/rawevent trigger 'noname'")
	assert.Equal(t, map[string]interface{}{"key1": "val1", "key3": "val3"}, collapsed.Match, "should combine the trigger chain match")

	extendingTrigger.Match["key1"] = "newval1"
	_, collapsed = CollapseTrigger(&extendingTrigger, cfg)
	assert.Equal(t, map[string]interface{}{"key1": "newval1", "key3": "val3"}, collapsed.Match, "should override the match with extending trigger")
}
