package dipper

import (
	"regexp"
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
	strVal, ok := actual.(string)
	if ok {
		return Compare(strVal, criteria)
	}

	listVal, ok := actual.([]interface{})
	if ok {
		strCriteria, ok := criteria.([]interface{})
		if ok && strCriteria[0] == ":all:" {
			// all elements in the list have to match
			for _, subVal := range listVal {
				if !CompareAll(subVal, strCriteria[1]) {
					return false
				}
			}
			return true
		}

		// any one element in the list needs to match
		for _, subVal := range listVal {
			if CompareAll(subVal, criteria) {
				return true
			}
		}
		return false
	}

	mapVal, ok := actual.(map[string]interface{})
	if ok {
		if criteria == nil {
			return true
		}
		if mapCriteria, ok := criteria.(map[string]interface{}); ok {
			for key, subVal := range mapVal {
				if subCriteria, ok := mapCriteria[key]; ok {
					if !CompareAll(subVal, subCriteria) {
						return false
					}
				}
			}

			// check if anything needs to be absent
			if absents, ok := mapCriteria[":absent:"]; ok {
				absentList, ok := absents.([]interface{})
				if !ok {
					absentList = []interface{}{absents}
				}
				keys := []interface{}{}
				for k := range mapVal {
					keys = append(keys, k)
				}
				// if any key matches any in the absent list, return false
				return !CompareAll(keys, absentList)
			}
			return true
		}
	}

	// unable to handle a nil value or unknown criteria
	return false
}
