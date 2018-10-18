package dipper

import (
	"log"
	"reflect"
	"strings"
	"sync"
)

// GetMapData : get the data from the deep map following a KV path
func GetMapData(from interface{}, path string) (ret interface{}, ok bool) {
	components := strings.Split(path, ".")
	var current = reflect.ValueOf(from)
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
func Recursive(from interface{}, process func(key string, val string) (newval interface{}, ok bool)) {
	RecursiveWithPrefix(nil, "", "", from, process)
}

// RecursiveWithPrefix : enumerate all the data element deep into the map call the function provided
func RecursiveWithPrefix(
	parent map[string]interface{},
	prefixes string,
	key string,
	from interface{},
	process func(key string, val string) (newval interface{}, ok bool),
) {
	newPrefixes := key
	if len(prefixes) > 0 && len(key) > 0 {
		newPrefixes = prefixes + "." + key
	}
	if str, ok := from.(string); ok {
		if newval, ok := process(newPrefixes, str); ok {
			if parent != nil {
				if newval != nil {
					parent[key] = newval
				} else {
					delete(parent, key)
				}
			}
		}
	} else if mp, ok := from.(map[string]interface{}); ok {
		for k, value := range mp {
			RecursiveWithPrefix(mp, newPrefixes, k, value, process)
		}
	} else {
		log.Panicf("*********** Passed a map but not map[string]interface{} *********************")
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
		if checkValue != nil && retVal.IsValid() && retVal.Interface() == checkValue {
			resVal.SetMapIndex(keyVal, reflect.Value{})
		} else if checkValue == nil {
			resVal.SetMapIndex(keyVal, reflect.Value{})
		}
	}

	if retVal.IsValid() {
		return retVal.Interface()
	}
	return nil
}
