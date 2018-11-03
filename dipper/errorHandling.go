package dipper

import (
	"github.com/op/go-logging"
)

// SafeExitOnError : use this function in defer statement to ignore errors
func SafeExitOnError(l *logging.Logger, args ...interface{}) {
	if r := recover(); r != nil {
		l.Warningf("Resuming after error: %v\n", r)
		l.Warningf(args[0].(string), args[1:]...)
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
