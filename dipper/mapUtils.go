package dipper

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

func init() {
	gob.Register(map[string]interface{}{})
}

// GetMapData : get the data from the deep map following a KV path
func GetMapData(from interface{}, path string) (ret interface{}, ok bool) {
	components := strings.Split(path, ".")
	var current = reflect.ValueOf(from)
	if !current.IsValid() {
		return nil, ok
	}
	for _, component := range components {
		if current.Kind() != reflect.Map {
			return nil, ok
		}
		nextValue := current.MapIndex(reflect.ValueOf(component))
		if !nextValue.IsValid() {
			return nil, ok
		}
		current = reflect.ValueOf(nextValue.Interface())
	}
	return current.Interface(), true
}

// GetMapDataStr : get the data as string from the deep map following a KV path
func GetMapDataStr(from interface{}, path string) (ret string, ok bool) {
	if data, ok := GetMapData(from, path); ok {
		str, ok := data.(string)
		return str, ok
	}
	return "", ok
}

// Recursive : enumerate all the data element deep into the map call the function provided
func Recursive(from interface{}, process func(key string, val interface{}) (newval interface{}, ok bool)) {
	RecursiveWithPrefix(nil, "", "", from, process)
}

// RecursiveWithPrefix : enumerate all the data element deep into the map call the function provided
func RecursiveWithPrefix(
	parent interface{},
	prefixes string,
	key interface{},
	from interface{},
	process func(key string, val interface{}) (newval interface{}, ok bool),
) {
	keyStr := fmt.Sprintf("%v", key)
	newPrefixes := keyStr
	if len(prefixes) > 0 && len(keyStr) > 0 {
		newPrefixes = prefixes + "." + keyStr
	}
	if mp, ok := from.(map[string]interface{}); ok {
		for k, value := range mp {
			RecursiveWithPrefix(mp, newPrefixes, k, value, process)
		}
	} else {
		if parent == nil {
			return
		}
		if newval, ok := process(newPrefixes, from); ok {
			if parentArray, ok := parent.([]interface{}); ok {
				parentArray[key.(int)] = newval
			} else if parentMap, ok := parent.(map[string]interface{}); ok {
				if newval != nil {
					parentMap[key.(string)] = newval
				} else {
					delete(parentMap, key.(string))
				}
			}
		}
	}
}

// LockGetMap : acquire a lock and look up a key in the map then return the result
func LockGetMap(lock *sync.Mutex, resource interface{}, key interface{}) (ret interface{}, ok bool) {
	lock.Lock()
	defer lock.Unlock()
	resVal := reflect.ValueOf(resource)
	if resVal.Kind() != reflect.Map {
		return nil, false
	}
	retVal := resVal.MapIndex(reflect.ValueOf(key))
	if !retVal.IsValid() {
		return nil, false
	}
	return retVal.Interface(), true
}

// LockSetMap : acquire a lock and set the value in the map by index and return the previous value if available
func LockSetMap(lock *sync.Mutex, resource interface{}, key interface{}, val interface{}) (ret interface{}) {
	lock.Lock()
	defer lock.Unlock()
	resVal := reflect.ValueOf(resource)
	keyVal := reflect.ValueOf(key)
	retVal := resVal.MapIndex(keyVal)

	if resVal.IsNil() {
		resVal.Set(reflect.MakeMap(resVal.Type()))
	}
	resVal.SetMapIndex(keyVal, reflect.ValueOf(val))

	if retVal.IsValid() {
		return retVal.Interface()
	}
	return nil
}

// LockCheckDeleteMap : acquire a lock and delete the entry from the map and return the previous value if available
func LockCheckDeleteMap(lock *sync.Mutex, resource interface{}, key interface{}, checkValue interface{}) (ret interface{}) {
	lock.Lock()
	defer lock.Unlock()
	retVal := reflect.ValueOf(ret)
	resVal := reflect.ValueOf(resource)
	keyVal := reflect.ValueOf(key)
	if !resVal.IsNil() {
		retVal = resVal.MapIndex(keyVal)
		if checkValue != nil && retVal.IsValid() {
			if retVal.Interface() == checkValue {
				resVal.SetMapIndex(keyVal, reflect.Value{})
			} else {
				resVal.SetMapIndex(keyVal, reflect.Value{})
			}
		} else if checkValue == nil {
			resVal.SetMapIndex(keyVal, reflect.Value{})
		}
	}

	if retVal.IsValid() {
		return retVal.Interface()
	}
	return nil
}

// DeepCopy : performs a deep copy of the given map m.
func DeepCopy(m map[string]interface{}) (map[string]interface{}, error) {
	var buf bytes.Buffer
	if m == nil {
		return nil, nil
	}
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)
	err := enc.Encode(m)
	if err != nil {
		return nil, err
	}
	var copy map[string]interface{}
	err = dec.Decode(&copy)
	if err != nil {
		return nil, err
	}
	return copy, nil
}
