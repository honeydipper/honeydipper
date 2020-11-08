// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import "github.com/go-errors/errors"

// Error type is a simple error type used by dipper.
type Error string

// Error gives the message of the Error.
func (e Error) Error() string {
	return string(e)
}

// Is : check whether the given error matches this error.
func (e Error) Is(target error) bool {
	if t, ok := target.(Error); ok {
		return t == e
	}

	return false
}

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
