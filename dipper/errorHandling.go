package dipper

import "github.com/go-errors/errors"

// SafeExitOnError : use this function in defer statement to ignore errors
func SafeExitOnError(args ...interface{}) {
	if r := recover(); r != nil {
		log.Warningf("Resuming after error: %v", r)
		log.Warning(errors.Wrap(r, 1).ErrorStack())
		log.Warningf(args[0].(string), args[1:]...)
	}
}

// IgnoreError : use this function in defer statement to ignore a particular error
func IgnoreError(expectedError interface{}) {
	if x := recover(); x != nil && x != expectedError {
		panic(x)
	}
}

// CatchError : use this in defer to catch a certain error
func CatchError(err interface{}, handler func()) {
	if x := recover(); x != nil {
		if x == err {
			handler()
		} else {
			panic(x)
		}
	}
}
