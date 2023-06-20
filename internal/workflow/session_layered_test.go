// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package workflow

import (
	"testing"

	"github.com/ghodss/yaml"
	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/workflow/mock_workflow"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestSessionLayered(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelper := mock_workflow.NewMockSessionStoreHelper(ctrl)
	s := NewSessionStore(mockHelper)

	// tear down
	defer delete(dipper.IDMapMetadata, &s.sessions)

	testDataSet := &config.DataSet{}
	testConfig := &config.Config{DataSet: testDataSet}
	mockHelper.EXPECT().GetConfig().AnyTimes().Return(testConfig)

	wf1 := &config.Workflow{}
	err := yaml.Unmarshal([]byte(`
with:
  - foo: bar
  - foo+: " and "
    var1: $ctx.foo
  - foo+: bar2
  - var2: $ctx.foo
    `), wf1)
	assert.Nil(t, err, "test config")
	w := s.newSession("", "uuid1", wf1).(*Session)
	w.prepare(&dipper.Message{}, nil, map[string]interface{}{})
	assert.Equal(t, "bar and bar2", w.ctx["foo"], "inheriting from previous layer")
	assert.Equal(t, "bar", w.ctx["var1"], "using value from previous layer")
	assert.Equal(t, "bar and bar2", w.ctx["var2"], "using value from previous multiple layers")
}
