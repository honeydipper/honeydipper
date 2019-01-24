package dipper

import (
	"bytes"
	"fmt"
	"github.com/Masterminds/sprig"
	"github.com/ghodss/yaml"
	"strings"
	"text/template"
	"time"
)

// FuncMap : used to add functions to the go templates
var FuncMap = template.FuncMap{
	"fromPath": MustGetMapData,
	"now":      time.Now,
	"duration": time.ParseDuration,
	"ISO8601":  func(t time.Time) string { return t.Format(time.RFC3339) },
}

// InterpolateStr : parse the string as go template
func InterpolateStr(pattern string, data interface{}) string {
	tmpl := template.Must(template.New("got").Funcs(FuncMap).Funcs(sprig.TxtFuncMap()).Parse(pattern))
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, data); err != nil {
		log.Warningf("interpolation pattern failed: %+v", pattern)
		log.Panicf("failed to interpolate: %+v", err)
	}
	return buf.String()
}

// ParseYaml : load the data in the string as yaml
func ParseYaml(pattern string) interface{} {
	var data interface{}
	err := yaml.Unmarshal([]byte(pattern), &data)

	if err != nil {
		panic(err)
	}
	return data
}

// Interpolate : go through the map data structure to find and parse all the templates
func Interpolate(source interface{}, data interface{}) interface{} {
	switch v := source.(type) {
	case string:
		if strings.HasPrefix(v, ":path:") {
			ret, ok := GetMapData(data, v[6:])
			if !ok {
				panic(fmt.Errorf("invalid path %s", v[6:]))
			}
			return ret
		}
		ret := InterpolateStr(v, data)
		if strings.HasPrefix(ret, ":yaml:") {
			defer func() {
				if r := recover(); r != nil {
					log.Warningf("loading yaml string: %s", ret[6:])
					panic(r)
				}
			}()
			return ParseYaml(ret[6:])
		}
		return ret
	case map[string]interface{}:
		ret := map[string]interface{}{}
		for k, val := range v {
			ret[k] = Interpolate(val, data)
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
