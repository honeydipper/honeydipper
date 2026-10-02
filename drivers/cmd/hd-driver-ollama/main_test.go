package main

import (
	"io"
	"os"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		f, _ := os.Create("test.log")
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}

func TestDriverSetupPreservesYAMLToolSchema(t *testing.T) {
	dataSet := config.DataSet{}
	require.NoError(t, yaml.Unmarshal([]byte(`
drivers:
  ollama:
    tools:
      test_tool:
        tool:
          type: function
          function:
            name: test_function
            description: test function description
            parameters:
              type: object
              properties:
                query:
                  type: string
                  description: query to process
              required:
                - query
        workflow:
          name: test_workflow
          steps:
            - call_workflow: test_action
      invalid_tool: invalid
`), &dataSet))

	driver = &dipper.Driver{Options: map[string]interface{}{
		"data": dataSet.Drivers["ollama"],
	}}

	setup(nil)

	data, ok := driver.GetOption("data")
	require.True(t, ok, "data should exist in driver options")

	dataMap := data.(map[string]interface{})
	toolsList, ok := dataMap["tools_list"]
	require.True(t, ok, "tools_list should exist in data")
	tools := toolsList.([]api.Tool)
	require.Len(t, tools, 1, "should have one valid tool")

	properties := tools[0].Function.Parameters.Properties
	require.NotNil(t, properties)
	require.Equal(t, 1, properties.Len())
	query, ok := properties.Get("query")
	require.True(t, ok)
	assert.Equal(t, api.PropertyType{"string"}, query.Type)
	assert.Equal(t, "query to process", query.Description)
}

func TestDriverSetupEmptyTools(t *testing.T) {
	i, feed := io.Pipe()
	resp, o := io.Pipe()

	initDriver()
	driver.In = i
	driver.Out = o

	fin := make(chan bool)
	go func() {
		assert.NotPanics(t, driver.Run, "main should not panic")
		close(fin)
	}()

	dipper.SendMessage(feed, &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		},
	})
	dipper.SendMessage(feed, &dipper.Message{
		Channel: "command",
		Subject: "start",
	})

	dipper.FetchMessage(resp)

	// Verify commands were registered even without tools
	assert.NotNil(t, driver.CommandProvider.Commands["chat"], "chat command should be registered")
	assert.NotNil(t, driver.CommandProvider.Commands["chatContinue"], "chatContinue command should be registered")
	assert.NotNil(t, driver.CommandProvider.Commands["chatStop"], "chatStop command should be registered")
	assert.NotNil(t, driver.CommandProvider.Commands["chatListen"], "chatListen command should be registered")

	// Verify empty tools list was created
	data, ok := driver.GetOption("data")
	assert.True(t, ok, "data should exist in driver options")

	dataMap := data.(map[string]interface{})
	toolsList, ok := dataMap["tools_list"]
	assert.True(t, ok, "tools_list should exist in data")
	assert.Equal(t, 0, len(toolsList.([]api.Tool)), "tools list should be empty")

	driver.State = dipper.DriverStateCompleted
	feed.Close()
	<-fin
}

func TestDriverSetupInvalidData(t *testing.T) {
	i, feed := io.Pipe()
	resp, o := io.Pipe()

	initDriver()
	driver.In = i
	driver.Out = o

	fin := make(chan bool)
	go func() {
		assert.NotPanics(t, driver.Run, "main should not panic")
		close(fin)
	}()

	dipper.SendMessage(feed, &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"tools": "invalid",
			},
		},
	})
	dipper.SendMessage(feed, &dipper.Message{
		Channel: "command",
		Subject: "start",
	})

	dipper.FetchMessage(resp)

	// Verify commands were registered despite invalid data
	assert.NotNil(t, driver.CommandProvider.Commands["chat"], "chat command should be registered")
	assert.NotNil(t, driver.CommandProvider.Commands["chatContinue"], "chatContinue command should be registered")
	assert.NotNil(t, driver.CommandProvider.Commands["chatStop"], "chatStop command should be registered")
	assert.NotNil(t, driver.CommandProvider.Commands["chatListen"], "chatListen command should be registered")

	// Verify empty tools list was created
	data, ok := driver.GetOption("data")
	assert.True(t, ok, "data should exist in driver options")

	dataMap := data.(map[string]interface{})
	toolsList, ok := dataMap["tools_list"]
	assert.True(t, ok, "tools_list should exist in data")
	assert.Equal(t, 0, len(toolsList.([]api.Tool)), "tools list should be empty")

	driver.State = dipper.DriverStateCompleted
	feed.Close()
	<-fin
}
