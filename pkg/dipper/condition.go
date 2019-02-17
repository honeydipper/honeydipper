// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import (
	"reflect"
	"regexp"
	"strings"
)

// Compare : compare an actual value to a criteria
func Compare(actual string, criteria interface{}) bool {
	if criteria == nil {
		return true
	}

	strVal, ok := criteria.(string)
	if ok {
		return (strVal == actual)
	}

	re, ok := criteria.(*regexp.Regexp)
	if ok {
		return re.Match([]byte(actual))
	}

	listVal, ok := criteria.([]interface{})
	if ok {
		for _, subVal := range listVal {
			if Compare(actual, subVal) {
				return true
			}
		}
		return false
	}

	// unable to handle this criteria
	return false
}

// CompareAll : compare all conditions against an event data structure
func CompareAll(actual interface{}, criteria interface{}) bool {
	if criteria == nil {
		return true
	}

	value := reflect.ValueOf(actual)
	switch kind := value.Kind(); kind {
	case reflect.String:
		return Compare(actual.(string), criteria)

	case reflect.Slice:
		strCriteria, ok := criteria.([]interface{})
		if ok && strCriteria[0] == ":all:" {
			// all elements in the list have to match
			for i := 0; i < value.Len(); i++ {
				if !CompareAll(value.Index(i).Interface(), strCriteria[1]) {
					return false
				}
			}
			return true
		}

		// any one element in the list needs to match
		for i := 0; i < value.Len(); i++ {
			if CompareAll(value.Index(i).Interface(), criteria) {
				return true
			}
		}
		return false

	case reflect.Map:
		if mapCriteria, ok := criteria.(map[string]interface{}); ok {
			for key, subCriteria := range mapCriteria {
				if key == ":auth:" {
					// offload to another driver using RPC
					// pass
				} else if key == ":absent:" {
					keys := []interface{}{}
					for _, k := range value.MapKeys() {
						keys = append(keys, k.Interface())
					}
					if CompareAll(keys, subCriteria) {
						// key not absent
						return false
					}
				} else if subVal := value.MapIndex(reflect.ValueOf(key)); subVal.IsValid() {
					if !CompareAll(subVal.Interface(), subCriteria) {
						return false
					}
				} else {
					// value not present for this criteria
					return false
				}
			}
			return true
		}
		// map value with a non-map criteria
		return false
	}

	// unable to handle a nil value or unknown criteria
	return false
}

// RegexParser : used with Recursive to process the data in the conditions so they can be used for matching
func RegexParser(key string, val interface{}) (ret interface{}, replace bool) {
	if str, ok := val.(string); ok {
		if strings.HasPrefix(str, ":regex:") {
			if newval, err := regexp.Compile(str[7:]); err == nil {
				return newval, true
			}
			Logger.Warningf("skipping invalid regex pattern %s", str[7:])
		}
		return nil, false
	}
	return nil, false
}
