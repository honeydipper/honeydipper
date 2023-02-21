// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/ghodss/yaml"
)

// ErrInterpolationError represents all errors thrown due to failure to interpolate.
var ErrInterpolationError = errors.New("config error")

// FuncMap : used to add functions to the go templates.
var FuncMap = template.FuncMap{
	"fromPath": MustGetMapData,
	"now":      time.Now,
	"duration": time.ParseDuration,
	"ISO8601":  func(t time.Time) string { return t.Format(time.RFC3339) },
	"toYaml": func(v interface{}) string {
		s, err := yaml.Marshal(v)
		if err != nil {
			panic(err)
		}

		return string(s)
	},
}

// InterpolateStr : interpolate a string and return a string.
func InterpolateStr(pattern string, data interface{}) string {
	ret := Interpolate(pattern, data)
	if ret != nil {
		return fmt.Sprintf("%+v", ret)
	}

	return ""
}

// InterpolateGoTemplate : parse the string as go template.
func InterpolateGoTemplate(isLoading bool, title string, pattern string, data interface{}) interface{} {
	ldelim := "{{"
	rdelim := "}}"
	if isLoading {
		ldelim = "{%"
		rdelim = "%}"
	}
	if strings.Contains(pattern, ldelim) {
		var ret interface{}
		useRet := false
		returnFuncMap := template.FuncMap{
			"return": func(v interface{}) string {
				useRet = true
				ret = v

				return ""
			},
		}

		tmpl := template.New(title)
		tmpl = tmpl.Funcs(FuncMap)
		tmpl = tmpl.Funcs(sprig.TxtFuncMap())
		tmpl = tmpl.Funcs(returnFuncMap)
		tmpl = tmpl.Delims(ldelim, rdelim)
		parsed := template.Must(tmpl.Parse(pattern))

		buf := new(bytes.Buffer)
		if err := parsed.Execute(buf, data); err != nil {
			Logger.Warningf("interpolation pattern failed: %+v", pattern)
			Logger.Panicf("failed to interpolate: %+v", err)
		}

		if useRet {
			return ret
		}

		return buf
	}

	return pattern
}

// ParseYaml : load the data in the string as yaml.
func ParseYaml(pattern string) interface{} {
	var data interface{}
	err := yaml.Unmarshal([]byte(pattern), &data)
	if err != nil {
		panic(err)
	}

	return data
}

// InterpolateDollarStr handles dollar interpolation.
func InterpolateDollarStr(v string, data interface{}) interface{} {
	allowNull := (v[1] == '?')
	var parsed string
	if allowNull {
		parsed = InterpolateStr(v[2:], data)
	} else {
		parsed = InterpolateStr(v[1:], data)
	}

	quote := strings.IndexAny(parsed, "\"'`")
	if allowNull && quote >= 0 {
		panic(fmt.Errorf("%w: allow nil combine with default value: %s", ErrInterpolationError, v))
	}

	var keys []string
	if quote > 0 {
		if parsed[quote-1] != ',' {
			panic(fmt.Errorf("%w: missing comma: %s", ErrInterpolationError, v))
		}
		keys = strings.Split(parsed[:quote-1], ",")
	} else if quote < 0 {
		keys = strings.Split(parsed, ",")
	}

	for _, key := range keys {
		ret, _ := GetMapData(data, key)
		if ret != nil {
			if strings.HasPrefix(key, "sysData.") {
				return Interpolate(ret, data)
			}

			return ret
		}
	}

	if quote >= 0 {
		if parsed[quote] != parsed[len(parsed)-1] {
			panic(fmt.Errorf("%w: quotes not matching: %s", ErrInterpolationError, parsed))
		}

		return parsed[quote+1 : len(parsed)-1]
	}

	if allowNull {
		return nil
	}
	panic(fmt.Errorf("%w: invalid path: %s", ErrInterpolationError, v[1:]))
}

// Interpolate : go through the map data structure to find and parse all the templates.
func Interpolate(source interface{}, data interface{}) interface{} {
	switch v := source.(type) {
	case string:
		if strings.HasPrefix(v, "$") {
			return InterpolateDollarStr(v, data)
		}

		var ret string

		switch retAnything := InterpolateGoTemplate(false, "go", v, data).(type) {
		case *bytes.Buffer:
			ret = retAnything.String()
		case string:
			ret = retAnything
		default:
			return retAnything
		}

		if strings.HasPrefix(ret, ":yaml:") {
			defer func() {
				if r := recover(); r != nil {
					Logger.Warningf("loading yaml string: %s", ret[6:])
					panic(r)
				}
			}()

			return ParseYaml(ret[6:])
		}

		return strings.TrimPrefix(ret, "\\")
	case map[string]interface{}:
		ret := map[string]interface{}{}
		for k, val := range v {
			ret[k] = Interpolate(val, data)
		}

		return ret
	case []string:
		ret := []string{}
		for _, val := range v {
			ret = append(ret, InterpolateStr(val, data))
		}

		return ret
	case []interface{}:
		ret := []interface{}{}
		for _, val := range v {
			ret = append(ret, Interpolate(val, data))
		}

		return ret
	}

	return source
}
