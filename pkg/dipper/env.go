// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package dipper is a library used for developing drivers for Honeydipper.
package dipper

import (
	"os"
	"strings"
)

var _envs map[string]interface{}

const (
	envPrefix    string = "HD_"
	envPrefixLen int    = len(envPrefix)
)

// Getenv returns all HD_ prefixed environment variables in a map, with prefix removed.
func Getenv() map[string]interface{} {
	if _envs != nil {
		return _envs
	}

	_envs = map[string]interface{}{}
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, envPrefix) {
			at := strings.IndexRune(v, '=')
			name := v[envPrefixLen:at]
			value := v[at+1:]
			_envs[name] = value
		}
	}

	return _envs
}
