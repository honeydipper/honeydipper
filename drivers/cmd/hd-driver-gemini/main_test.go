package main

import (
	"os"
	"testing"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		f, _ := os.Create("test.log")
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}

func TestGeminiInit(t *testing.T) {
	// Create a new gemini instance with mock driver
	g := &gemini{
		driver: &dipper.Driver{
			Options: map[string]interface{}{
				"data": map[string]interface{}{
					"tools": map[string]interface{}{
						"test_tool": map[string]interface{}{
							"tool": map[string]any{
								"functionDeclarations": []map[string]any{
									{
										"Name":       "test_function",
										"Parameters": map[string]any{},
										"Definition": "test function definition",
									},
								},
							},
							"workflow": map[string]any{
								"name": "test_workflow",
								"steps": []map[string]any{
									{
										"call_workflow": "test_action",
									},
								},
							},
						},
						"invalid_tool": "invalid",
					},
				},
			},
		},
	}

	// Call init
	g.init(nil)

	// Get the processed data from driver options
	data, ok := g.driver.GetOption("data")
	assert.True(t, ok, "data should exist in driver options")

	// Verify tools_list was created and stored
	dataMap := data.(map[string]interface{})
	toolsList, ok := dataMap["tools_list"]
	assert.True(t, ok, "tools_list should exist in data")

	// Verify tools were processed correctly
	tools := toolsList.([]*genai.Tool)
	assert.Equal(t, 1, len(tools), "should have processed one valid tool")

	// Verify the tool was processed correctly
	assert.Equal(t, "test_function", tools[0].FunctionDeclarations[0].Name)

	// Verify workflow was preserved in original tool map
	toolMap := dataMap["tools"].(map[string]interface{})
	testTool := toolMap["test_tool"].(map[string]interface{})
	workflow, ok := testTool["workflow"].(*config.Workflow)
	assert.True(t, ok, "workflow should exist and be of correct type")
	assert.Equal(t, "test_workflow", workflow.Name)
	assert.Equal(t, 1, len(workflow.Steps))
	assert.Equal(t, "test_action", workflow.Steps[0].Workflow)
}

func TestGeminiInitEmptyTools(t *testing.T) {
	// Test with no tools configured
	g := &gemini{
		driver: &dipper.Driver{
			Options: map[string]interface{}{},
		},
	}

	// Should not panic
	g.init(nil)

	// Verify empty tools list was created
	data, ok := g.driver.GetOption("data")
	assert.True(t, ok, "data should exist in driver options")

	dataMap := data.(map[string]interface{})
	toolsList, ok := dataMap["tools_list"]
	assert.True(t, ok, "tools_list should exist in data")
	assert.Equal(t, 0, len(toolsList.([]*genai.Tool)), "tools list should be empty")
}

func TestGeminiInitInvalidData(t *testing.T) {
	// Test with invalid data structure
	g := &gemini{
		driver: &dipper.Driver{
			Options: map[string]interface{}{
				"data": map[string]interface{}{
					"tools": "invalid",
				},
			},
		},
	}

	// Should not panic
	g.init(nil)

	// Verify empty tools list was created
	data, ok := g.driver.GetOption("data")
	assert.True(t, ok, "data should exist in driver options")

	dataMap := data.(map[string]interface{})
	toolsList, ok := dataMap["tools_list"]
	assert.True(t, ok, "tools_list should exist in data")
	assert.Equal(t, 0, len(toolsList.([]*genai.Tool)), "tools list should be empty")
}
