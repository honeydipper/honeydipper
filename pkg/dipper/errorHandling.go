// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import "github.com/go-errors/errors"

// SafeExitOnError : use this function in defer statement to ignore errors.
func SafeExitOnError(args ...interface{}) {
	if r := recover(); r != nil {
		Logger.Warningf("Resuming after error: %v", r)
		Logger.Warning(errors.Wrap(r, 1).ErrorStack())
		Logger.Warningf(args[0].(string), args[1:]...)
	}
}

// IgnoreError : use this function in defer statement to ignore a particular error.
func IgnoreError(expectedError interface{}) {
	if x := recover(); x != nil && x != expectedError {
		panic(x)
	}
}

// CatchError : use this in defer to catch a certain error.
func CatchError(err interface{}, handler func()) {
	if x := recover(); x != nil {
		if x == err {
			handler()
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
	//nolint:gomnd
	case 2:
		return args[0]
	}

	return args[0 : l-1]
}
