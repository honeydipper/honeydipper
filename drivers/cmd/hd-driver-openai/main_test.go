package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
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
	opts := map[string]any{}
	dipper.Must(json.Unmarshal([]byte(`
	{
		"data": {
			"engine": {
				"gpt-3_5-turbo": {
					"base_url": "https://api.openai.com/v1",
					"api_key": "testkey",
					"model": "gpt-3.5-turbo",
					"temperature": 0.7,
					"streaming": true
				}
			},
			"tools": {
			   "example_tool": {
			   		"workflow": {},
					"tool":	{
						"name": "example_tool",
						"description": "An example tool for testing.",
						"parameters": {
							"type": "object",
							"properties": {
								"input": {
									"type": "string",
									"description": "Input for the tool."
								}
							},
							"required": ["input"]
						}
					}
				}
			}
		}
	}
	`), &opts))
	drv.driver.Options = opts
	drv.init(&dipper.Message{})
	// Should not panic and should add data.tools_list
	opt, ok := drv.driver.Options.(map[string]any)["data"]
	assert.True(t, ok)
	data := opt.(map[string]any)
	_, exists := data["tools_list"]
	assert.True(t, exists)
}
