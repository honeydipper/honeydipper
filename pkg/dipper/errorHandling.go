// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import "github.com/go-errors/errors"

// SafeExitOnError : use this function in defer statement to ignore errors.
func SafeExitOnError(args ...interface{}) {
	if r := recover(); r != nil {
		l := len(args)
		if handler, ok := args[l-1].(func(interface{})); ok {
			l--
			handler(r)
		}
		Logger.Warningf("Resuming after error: %v", r)
		Logger.Warning(errors.Wrap(r, 1).ErrorStack())
		if l >= 1 {
			Logger.Warningf(args[0].(string), args[1:l]...)
		}
	}
}

// IgnoreError : use this function in defer statement to ignore a particular error.
func IgnoreError(expectedError interface{}) {
	if x := recover(); x != nil {
		e, CheckingError := expectedError.(error)
		xe, FoundError := x.(error)
		if CheckingError && FoundError && errors.Is(xe, e) || x == expectedError {
			return
		} else {
			panic(x)
		}
	}
}

// CatchError : use this in defer to catch a certain error.
func CatchError(err interface{}, handler func()) {
	if x := recover(); x != nil {
		e, CheckingError := err.(error)
		xe, FoundError := x.(error)
		if CheckingError && FoundError && errors.Is(xe, e) || x == err {
			if handler != nil {
				handler()
			}
		} else {
			panic(x)
		}
	}
}

// Must is used to catch function return with error, used for wrapping a call that can return a error.
func Must(args ...interface{}) interface{} {
	l := len(args)
	if l == 0 {
		return nil
	}
	if err := args[l-1]; err != nil {
		panic(err)
	}
	switch l {
	case 1:
		return nil
	case 2:
		return args[0]
	}

	return args[0 : l-1]
}
