package dipper

import (
	"reflect"
	"strconv"
	"sync"
)

// MaxID : the maximum rpcID
const MaxID = 13684

// IDMap : a map that store values with automatically generated keys
type IDMap interface{}

// IDMapMeta : meta info structure for a IDMap object
type IDMapMeta struct {
	Counter int
	Lock    sync.Mutex
}

// IDMapMetadata : actual metadata for all IDMap objects
var IDMapMetadata = map[IDMap]*IDMapMeta{}

// InitIDMap : create a new IDMap Object
func InitIDMap(m IDMap) {
	IDMapMetadata[m] = &IDMapMeta{}
}

// IDMapPut : putting an value in map return a unique ID
func IDMapPut(m IDMap, val interface{}) string {
	meta := IDMapMetadata[m]

	(*meta).Lock.Lock()
	defer (*meta).Lock.Unlock()

	mapValue := reflect.ValueOf(m).Elem()
	for mapValue.MapIndex(reflect.ValueOf(strconv.Itoa((*meta).Counter))).IsValid() {
		(*meta).Counter++
		if (*meta).Counter == MaxID {
			(*meta).Counter = 0
		}
	}
	ID := strconv.Itoa((*meta).Counter)
	mapValue.SetMapIndex(reflect.ValueOf(ID), reflect.ValueOf(val))

	(*meta).Counter++
	if (*meta).Counter == MaxID {
		(*meta).Counter = 0
	}

	return ID
}

// IDMapDel : deleting a value from ID map
func IDMapDel(m IDMap, key string) {
	meta := IDMapMetadata[m]
	(*meta).Lock.Lock()
	defer (*meta).Lock.Unlock()

	mapValue := reflect.ValueOf(m).Elem()
	mapValue.SetMapIndex(reflect.ValueOf(key), reflect.Zero(mapValue.Type().Elem()))
}
