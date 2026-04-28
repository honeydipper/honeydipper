// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"testing"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestBuildRuleMapIncludesActivateRules(t *testing.T) {
	prevEngine := engine
	prevStore := sessionStore
	defer func() {
		engine = prevEngine
		sessionStore = prevStore
	}()

	cfg := &config.Config{DataSet: &config.DataSet{Rules: []config.Rule{
		{
			When: config.Trigger{Driver: "webhook", RawEvent: "message"},
			Activate: &config.Activation{
				Agent: "support-bot",
			},
		},
		{
			When: config.Trigger{Driver: "webhook", RawEvent: "message"},
			Do:   config.Workflow{Workflow: "legacy_workflow"},
		},
	}}}

	engine = &Service{config: cfg}

	buildRuleMap(cfg)

	rules, ok := ruleMap["webhook.message"]
	assert.True(t, ok)
	assert.Len(t, rules, 2)
	assert.Equal(t, "support-bot", rules[0].OriginalRule.Activate.Agent)
	assert.Equal(t, "legacy_workflow", rules[1].OriginalRule.Do.Workflow)
}
