package main

import (
	"os"
	"testing"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestInitFlags(t *testing.T) {
	// Just ensure it runs without panic
	assert.NotPanics(t, func() { initFlags() })
}

func TestNewOpenAI(t *testing.T) {
	os.Args = []string{"cmd", "serviceName"}
	drv := newOpenAI()
	assert.NotNil(t, drv)
	assert.NotNil(t, drv.driver)
	assert.Equal(t, "openai", drv.driver.Name)
	assert.Equal(t, "serviceName", drv.driver.Service)
}

func TestOpenAIDriverInit_NoTools(t *testing.T) {
	os.Args = []string{"cmd", "serviceName"}
	drv := newOpenAI()
	drv.driver.Options = map[string]any{}
	drv.init(&dipper.Message{})
	// Should not panic and should add data.tools_list
	opt, ok := drv.driver.Options.(map[string]any)["data"]
	assert.True(t, ok)
	data := opt.(map[string]any)
	_, exists := data["tools_list"]
	assert.True(t, exists)
}
