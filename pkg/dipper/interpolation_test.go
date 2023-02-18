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
