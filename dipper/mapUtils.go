package dipper

import (
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
