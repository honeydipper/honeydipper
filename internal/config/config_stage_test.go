// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAdvanceStage(t *testing.T) {
	cfg := &Config{
		Services: []string{"testsvc1", "testsvc2"},
		Staged: &DataSet{
			Drivers: map[string]interface{}{
				"driver1": map[string]interface{}{
					"k1": "v1",
				},
			},
		},
	}
	cfg.ResetStage()

	processor := func(name string, val interface{}) (interface{}, bool) {
		if v, ok := val.(string); ok && v == "v1" {
			return "v1+", true
		}

		return nil, false
	}

	assert.PanicsWithValue(t, ErrConfigRollback, func() { cfg.AdvanceStage("testsvc1", StageDiscovering) }, "cannot skipping booting stage")
	go cfg.AdvanceStage("testsvc1", StageBooting, processor)
	time.Sleep(time.Millisecond)
	assert.NotEqual(t, StageBooting, cfg.Stage, "config stage not advanced")
	assert.Equal(t, "v1", cfg.Staged.Drivers["driver1"].(map[string]interface{})["k1"], "config not touched, waiting")
	cfg.AdvanceStage("testsvc2", StageBooting, processor)
	assert.Equal(t, StageBooting, cfg.Stage, "config stage advanced")
	assert.Equal(t, "v1+", cfg.Staged.Drivers["driver1"].(map[string]interface{})["k1"], "config touched after advance stage")
}

func TestRollBack(t *testing.T) {
	cfg := &Config{
		Services: []string{"testsvc1", "testsvc2"},
		Staged:   &DataSet{},
		LastRunningConfig: struct {
			DataSet *DataSet
			Loaded  map[RepoInfo]*Repo
		}{
			DataSet: &DataSet{
				Drivers: map[string]interface{}{
					"driver1": map[string]interface{}{
						"k1": "v1",
					},
				},
			},
			Loaded: map[RepoInfo]*Repo{},
		},
	}
	cfg.ResetStage()
	cfg.OnChange = func() {
		assert.Equal(t, "v1", cfg.Staged.Drivers["driver1"].(map[string]interface{})["k1"], "config set to last used data")
		go cfg.AdvanceStage("testsvc1", StageBooting)
		cfg.AdvanceStage("testsvc2", StageBooting)
		assert.Equal(t, StageBooting, cfg.Stage, "config stage advanced")
	}

	go cfg.AdvanceStage("testsvc1", StageBooting)
	cfg.AdvanceStage("testsvc2", StageBooting)
	go func() {
		assert.PanicsWithValue(t, ErrConfigRollback, func() { cfg.AdvanceStage("testsvc1", StageDiscovering) }, "cancelled due to rollback")
	}()
	cfg.RollBack()
}
