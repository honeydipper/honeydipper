// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package dipper is a library used for developing drivers for Honeydipper.
package dipper

import (
	util "github.com/Masterminds/goutils"
)

const MaxLabelLen = 256

func SanitizedLabels(l map[string]string) map[string]string {
	sl := map[string]string{}

	for k, v := range l {
		sl[k] = Must(util.AbbreviateFull(v, len(v), MaxLabelLen)).(string)
		// more sanitization in the future
	}

	return sl
}
