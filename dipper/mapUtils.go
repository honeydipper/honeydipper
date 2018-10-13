package dipper

import (
	"strings"
)

// GetMapData : get the data from the deep map following a KV path
func GetMapData(from interface{}, path string) (ret interface{}, ok bool) {
	components := strings.Split(path, ".")
	var current = from
	for _, component := range components {
		if current, ok = current.(map[string]interface{})[component]; !ok {
			return nil, ok
		}
	}
	return current, true
}

// GetMapDataStr : get the data as string from the deep map following a KV path
func GetMapDataStr(from interface{}, path string) (ret string, ok bool) {
	if data, ok := GetMapData(from, path); ok {
		str, ok := data.(string)
		return str, ok
	}
	return "", ok
}

// ForEachRecursive : enumerate all the data element deep into the map call the function provided
func ForEachRecursive(prefixes string, from interface{}, process func(key string, val string)) {
	if str, ok := from.(string); ok {
		process(prefixes, str)
	} else if mp, ok := from.(map[interface{}]interface{}); ok {
		for key, value := range mp {
			newkey := key.(string)
			if len(prefixes) > 0 {
				newkey = prefixes + "." + newkey
			}
			ForEachRecursive(newkey, value, process)
		}
	}
}
