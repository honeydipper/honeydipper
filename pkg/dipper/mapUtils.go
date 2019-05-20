// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

//nolint:gochecknoinits
func init() {
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
}

// GetMapData : get the data from the deep map following a KV path
func GetMapData(from interface{}, path string) (ret interface{}, ok bool) {
	var current = reflect.ValueOf(from)
	if !current.IsValid() {
		return nil, ok
	}
	components := strings.Split(path, ".")
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
	if !current.IsValid() {
		return nil, ok
	}
	return current.Interface(), true
}

// MustGetMapData : get the data from the deep map following a KV path, may raise errors
func MustGetMapData(from interface{}, path string) interface{} {
	ret, ok := GetMapData(from, path)
	if !ok {
		panic(fmt.Errorf("path not valid in data %s", path))
	}
	return ret
}

// GetMapDataStr : get the data as string from the deep map following a KV path
func GetMapDataStr(from interface{}, path string) (ret string, ok bool) {
	if data, ok := GetMapData(from, path); ok {
		str, ok := data.(string)
		return str, ok
	}
	return "", ok
}

// MustGetMapDataStr : get the data as string from the deep map following a KV path, may raise errors
func MustGetMapDataStr(from interface{}, path string) string {
	ret := MustGetMapData(from, path)
	return ret.(string)
}

// GetMapDataBool : get the data as bool from the deep map following a KV path
func GetMapDataBool(from interface{}, path string) (ret bool, ok bool) {
	if data, ok := GetMapData(from, path); ok {
		switch v := data.(type) {
		case bool:
			return v, true
		case int:
			return (v != 0), true
		case float64:
			return (v != 0), true
		case string:
			flag, err := strconv.ParseBool(v)
			return flag, (err == nil)
		}
	}
	return false, false
}

// MustGetMapDataBool : get the data as bool from the deep map following a KV path or panic
func MustGetMapDataBool(from interface{}, path string) bool {
	data, ok := GetMapData(from, path)
	if ok {
		switch v := data.(type) {
		case bool:
			return v
		case int:
			return (v != 0)
		case float64:
			return (v != 0)
		case string:
			flag, err := strconv.ParseBool(v)
			if err != nil {
				panic(err)
			}
			return flag
		}
	}
	panic(fmt.Errorf("not a valid bool %+v", data))
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
	vfrom := reflect.ValueOf(from)
	switch vfrom.Kind() {
	case reflect.Map:
		for _, vk := range vfrom.MapKeys() {
			RecursiveWithPrefix(from, newPrefixes, vk.Interface(), vfrom.MapIndex(vk).Interface(), process)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < vfrom.Len(); i++ {
			RecursiveWithPrefix(from, newPrefixes, i, vfrom.Index(i).Interface(), process)
		}
	case reflect.Ptr:
		vfrom = vfrom.Elem()
		if vfrom.Kind() == reflect.Struct {
			for i := 0; i < vfrom.NumField(); i++ {
				field := vfrom.Field(i)
				if field.IsValid() && field.CanSet() {
					RecursiveWithPrefix(from, newPrefixes, i, field.Interface(), process)
				}
			}
		}
	default:
		if parent == nil {
			return
		}
		if newval, ok := process(newPrefixes, from); ok {
			vparent := reflect.ValueOf(parent)
			vval := reflect.ValueOf(newval)
			switch vparent.Kind() {
			case reflect.Map:
				vparent.SetMapIndex(reflect.ValueOf(key), vval)
			case reflect.Slice, reflect.Array:
				vparent.Index(key.(int)).Set(vval)
			case reflect.Ptr:
				vparent = vparent.Elem()
				if vparent.Kind() == reflect.Struct {
					vparent.Field(key.(int)).Set(vval)
				}
			default:
				panic(fmt.Errorf("unable to change value in parent"))
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
				// should not delete if not the same
				// } else {
				//	resVal.SetMapIndex(keyVal, reflect.Value{})
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
