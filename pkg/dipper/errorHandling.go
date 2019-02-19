// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import "github.com/go-errors/errors"

// SafeExitOnError : use this function in defer statement to ignore errors
func SafeExitOnError(args ...interface{}) {
	if r := recover(); r != nil {
		Logger.Warningf("Resuming after error: %v", r)
		Logger.Warning(errors.Wrap(r, 1).ErrorStack())
		Logger.Warningf(args[0].(string), args[1:]...)
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

// PanicError accepts multiple variables and will panic if the last variable is not nil.
// It is used to wrap around functions that return error as the last return value.
//   dipper.PanicError(io.ReadFull(&b, lval))
// The io.ReadFull return length read and an error. If error is returned, the function will
// panic.
func PanicError(args ...interface{}) {
	if l := len(args); l > 0 {
		if err := args[l-1]; err != nil {
			panic(err)
		}
	}
}
