// +build !integration

package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConfigGetDriverData(t *testing.T) {
	mockdata := map[string]interface{}{
		"test1": "string1",
		"test2": map[string]interface{}{
			"test2_1": "string2",
		},
	}

	config := &Config{
		config: &ConfigSet{
			Drivers: mockdata,
		},
	}

	string1, ok := config.getDriverDataStr("test1")
	assert.True(t, ok, "GetDriverDataStr should be able to find test1")
	assert.Equal(t, "string1", string1, "GetDriverDataStr should find path 'test1' point to 'string1'")
	string2, ok := config.getDriverDataStr("test2.test2_1")
	assert.True(t, ok, "GetDriverDataStr should be able to find test2.test2_1")
	assert.Equal(t, "string2", string2, "GetDriverDataStr should find path 'test2.test2_1' point to 'string2'")
	obj, ok := config.getDriverData("test2")
	assert.True(t, ok, "GetDriverData should be able to find test2")
	objMap, ok := obj.(map[string]interface{})
	assert.True(t, ok, "GetDriverData should be able to fetch an obj from map test2")
	assert.Equal(t, "string2", objMap["test2_1"], "GetDriverData fetched object test2 should be useable")
	nonexist, ok := config.getDriverData("test3")
	assert.False(t, ok, "GetDriverData should set ok to false when 'test3' is not found")
	assert.Nil(t, nonexist, "GetDriverData should return nil when 'test3' is not found")
}

func TestConfigReload(t *testing.T) {
	service1reloaded := false
	service2reloaded := false
	var service1desiredConfig *ConfigSet
	var service2desiredConfig *ConfigSet

	mockConfig := &Config{
		config: &ConfigSet{
			Drivers: map[string]interface{}{
				"driver1": "driver1data",
				"driver2": "driver2data",
			},
		},
	}
	Services = map[string]*Service{
		"service1": &Service{
			ServiceReload: func(config *Config) {
				assert.Equal(t, service1desiredConfig, config.config, "service1 should reload with new config")
				service1reloaded = true
			},
			config: mockConfig,
		},
		"service2": &Service{
			ServiceReload: func(config *Config) {
				assert.Equal(t, service2desiredConfig, config.config, "service2 should reload with new config")
				service2reloaded = true
			},
			config: mockConfig,
		},
	}

	oldConfigSet := mockConfig.config
	service1desiredConfig = oldConfigSet
	service2desiredConfig = oldConfigSet
	mockConfig.rollBack()
	assert.Equal(t, oldConfigSet, mockConfig.config, "should not change the config when there is no lastRunningConfig")
	assert.False(t, service1reloaded, "service1 should not reload when there is no last running config")
	assert.False(t, service2reloaded, "service2 should not reload when there is no last running config")

	mockConfig.lastRunningConfig.config = &ConfigSet{
		Drivers: map[string]interface{}{
			"driver3": "driver3data",
			"driver1": "olddriver1data",
		},
	}
	mockConfig.lastRunningConfig.loaded = map[RepoInfo]*ConfigRepo{}

	oldConfigSet = mockConfig.config
	service1desiredConfig = mockConfig.lastRunningConfig.config
	service2desiredConfig = mockConfig.lastRunningConfig.config
	mockConfig.rollBack()
	assert.NotEqual(t, oldConfigSet, mockConfig.config, "should change the config when reload with lastRunningConfig")
	assert.True(t, service1reloaded, "service1 should reload when reloading with last running config")
	assert.True(t, service2reloaded, "service2 should reload when reloading with last running config")

	Services = map[string]*Service{}
}
