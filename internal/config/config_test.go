// +build !integration

package config

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
		DataSet: &DataSet{
			Drivers: mockdata,
		},
	}

	string1, ok := config.GetDriverDataStr("test1")
	assert.True(t, ok, "GetDriverDataStr should be able to find test1")
	assert.Equal(t, "string1", string1, "GetDriverDataStr should find path 'test1' point to 'string1'")
	string2, ok := config.GetDriverDataStr("test2.test2_1")
	assert.True(t, ok, "GetDriverDataStr should be able to find test2.test2_1")
	assert.Equal(t, "string2", string2, "GetDriverDataStr should find path 'test2.test2_1' point to 'string2'")
	obj, ok := config.GetDriverData("test2")
	assert.True(t, ok, "GetDriverData should be able to find test2")
	objMap, ok := obj.(map[string]interface{})
	assert.True(t, ok, "GetDriverData should be able to fetch an obj from map test2")
	assert.Equal(t, "string2", objMap["test2_1"], "GetDriverData fetched object test2 should be useable")
	nonexist, ok := config.GetDriverData("test3")
	assert.False(t, ok, "GetDriverData should set ok to false when 'test3' is not found")
	assert.Nil(t, nonexist, "GetDriverData should return nil when 'test3' is not found")
}
