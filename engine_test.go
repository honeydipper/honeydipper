// +build !integration

package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEngineBuildRuleMap(t *testing.T) {
	config := &Config{
		config: &ConfigSet{
			Rules: []Rule{
				Rule{When: Trigger{Driver: "webhook", RawEvent: "hit"}, Do: Workflow{Type: "action", Content: "some action"}},
				Rule{When: Trigger{Driver: "webhook2", RawEvent: "hit2"}, Do: Workflow{Content: "real_work"}},
				Rule{When: Trigger{Source: Event{System: "testsystem", Trigger: "testtrigger2"}}, Do: Workflow{Content: "real_work2"}},
			},
			Workflows: map[string]Workflow{
				"real_work": Workflow{Content: "some other stuff"},
			},
		},
	}

	buildRuleMap(config)
	assert.Equal(t, []*Workflow{&Workflow{Type: "action", Content: "some action"}}, ruleMap["_.hit"], "should be able to map rawevent to a action")
	assert.Equal(t, []*Workflow{&Workflow{Content: "real_work"}}, ruleMap["_.hit2"], "should be able to map rawevent to a workflow def")
}
