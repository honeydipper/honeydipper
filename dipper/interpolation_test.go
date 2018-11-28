// +build !integration

package dipper

import (
	"github.com/stretchr/testify/assert"
	"testing"
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
	parsed := Interpolate(map[string]interface{}{
		"notmpl":    "raw",
		"templated": " this is used by {{ index . \"user\" }}",
		"map_with_template": map[string]interface{}{
			"deep": " another {{ index . \"type\" }}",
		},
		"yaml_with_template": `:yaml:
---
test:
  - 1 {{ index (index . "list") "one" }}
  - 2 {{ index (index . "list") "two" }}`},
		map[string]interface{}{
			"user": "test",
			"type": "direct",
			"list": map[string]interface{}{
				"one": "one",
				"two": "two",
			},
		})
	assert.EqualValues(t,
		map[string]interface{}{
			"notmpl":    "raw",
			"templated": " this is used by test",
			"map_with_template": map[string]interface{}{
				"deep": " another direct",
			},
			"yaml_with_template": map[string]interface{}{
				"test": []interface{}{
					"1 one",
					"2 two",
				},
			},
		},
		parsed,
		"interpolating a map of templates",
	)
}
