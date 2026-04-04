// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package dipper

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInterpolateStr(t *testing.T) {
	parsed := InterpolateStr("{{ index . \"hello\" }} {{ index . \"world\" }}", map[string]interface{}{
		"hello": "hello",
		"world": "world",
	})
	assert.Equal(t, "hello world", parsed, "parsing go template")
	assert.Panics(t, func() {
		InterpolateStr("{{ index . 'hello' }} {{ index . \"world\" }}", map[string]interface{}{"h": "hellow"})
	}, "parsing panics with wrong template")
}

func TestInterpolate(t *testing.T) {
	parsed := Interpolate(
		map[string]interface{}{
			"notmpl":    "raw",
			"templated": " this is used by {{ index . \"user\" }}",
			"map_with_template": map[string]interface{}{
				"deep": " another {{ index . \"type\" }}",
			},
			"default_user": "$ctx.v1,ctx.v2,'default, value with comma'",
			"item_in_list": "$list.{{ .ptr }}",
			"yaml_with_template": `:yaml:
---
test:
  - 1 {{ index (index . "list") "one" }}
  - 2 {{ index (index . "list") "two" }}`,
			"yaml_safe_with_template": `:yaml_safe:
---
test:
  - 1 {{ index (index . "list") "one" }}
  - $list.{{ .ptr }}`,
		},
		map[string]interface{}{
			"user": "test",
			"type": "direct",
			"list": map[string]interface{}{
				"one":   "one",
				"two":   "two",
				"three": "the last one",
			},
			"ptr": "three",
		})
	assert.EqualValues(t,
		map[string]interface{}{
			"notmpl":    "raw",
			"templated": " this is used by test",
			"map_with_template": map[string]interface{}{
				"deep": " another direct",
			},
			"item_in_list": "the last one",
			"yaml_with_template": map[string]interface{}{
				"test": []interface{}{
					"1 one",
					"2 two",
				},
			},
			"yaml_safe_with_template": map[string]interface{}{
				"test": []interface{}{
					"1 one",
					"$list.three",
				},
			},
			"default_user": "default, value with comma",
		},
		parsed,
		"interpolating a map of templates",
	)
}

func TestInterpolateGoTemplate(t *testing.T) {
	assert.Equal(t, "{% not interpolated %}", InterpolateGoTemplate(false, "go", "{% not interpolated %}", map[string]interface{}{}), "should not interpolate {%%} in non-loading time")
	assert.Equal(t, "{{ not interpolated }}", InterpolateGoTemplate(true, "test.yml", "{{ not interpolated }}", map[string]interface{}{}), "should not interpolate {{}} in loading time")
	assert.Equal(t, "test", InterpolateGoTemplate(false, "go", "{{ .env.TEST_ENV }}", map[string]interface{}{"env": map[string]interface{}{"TEST_ENV": "test"}}).(*bytes.Buffer).String(), "should interpolate {{}} in non-loading time")
	assert.Equal(t, "test", InterpolateGoTemplate(true, "test.yml", "{% .env.TEST_ENV %}", map[string]interface{}{"env": map[string]interface{}{"TEST_ENV": "test"}}).(*bytes.Buffer).String(), "should interpolate {%%} in loading time")
	assert.Equal(t, true, InterpolateGoTemplate(false, "go", "{{ return true }}", map[string]interface{}{}), "should return a boolean type")
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, InterpolateGoTemplate(false, "go", "{{ dict \"foo\" \"bar\" | return }}", map[string]interface{}{}), "should return a map type")
}

func TestFuncMap_FromPath(t *testing.T) {
	data := map[string]interface{}{
		"nested": map[string]interface{}{
			"key": "value",
		},
	}
	result := InterpolateStr(`{{ fromPath . "nested.key" }}`, data)
	assert.Equal(t, "value", result, "fromPath should extract nested value")
}

func TestFuncMap_Now(t *testing.T) {
	result := InterpolateGoTemplate(false, "test", "{{ now | ISO8601 }}", map[string]interface{}{})
	assert.NotEmpty(t, result.(*bytes.Buffer).String(), "now should return current time")
}

func TestFuncMap_Duration(t *testing.T) {
	result := InterpolateGoTemplate(false, "test", `{{ duration "1h" }}`, map[string]interface{}{})
	assert.Equal(t, time.Duration(3600000000000), result.(time.Duration), "duration should parse duration string")
}

func TestFuncMap_ISO8601(t *testing.T) {
	data := map[string]interface{}{
		"time": "2024-01-15T10:30:00Z",
	}
	result := InterpolateStr("{{ now | ISO8601 }}", data)
	assert.Contains(t, result, "T", "ISO8601 should format time in RFC3339")
	assert.Contains(t, result, ":", "ISO8601 should include time separators")
}

func TestFuncMap_ToYaml(t *testing.T) {
	data := map[string]interface{}{
		"obj": map[string]interface{}{
			"key": "value",
			"num": 42,
		},
	}
	result := InterpolateStr("{{ toYaml .obj }}", data)
	assert.Contains(t, result, "key: value", "toYaml should convert to YAML")
	assert.Contains(t, result, "num: 42", "toYaml should include all fields")
}

func TestFuncMap_CueValidateError(t *testing.T) {
	if Logger == nil {
		GetLogger("test", "ERROR")
	}
	schema := `{name: string, age: int}`

	result := InterpolateGoTemplate(false, "test", `{{ cue_validate_error .schema "test" .data }}`, map[string]interface{}{
		"schema": schema,
		"data":   map[string]interface{}{"name": "John", "age": 30},
	})
	assert.Equal(t, "", result.(*bytes.Buffer).String(), "cue_validate_error should return empty string for valid data")

	result = InterpolateGoTemplate(false, "test", `{{ cue_validate_error .schema "test" .data }}`, map[string]interface{}{
		"schema": schema,
		"data":   map[string]interface{}{"name": "John", "age": "thirty"},
	})
	assert.NotEqual(t, "", result.(*bytes.Buffer).String(), "cue_validate_error should return error for invalid data")
}
