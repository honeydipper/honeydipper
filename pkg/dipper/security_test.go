// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package dipper

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizedLabels(t *testing.T) {
	src := map[string]string{
		"str1": "foo",
		"str2": strings.Repeat("x", MaxLabelLen),
		"str3": strings.Repeat("x", 999) + "123",
	}

	dst := SanitizedLabels(src)

	assert.Equal(t, src["str1"], dst["str1"], "short string should not change")
	assert.Equal(t, src["str2"], dst["str2"], "max length string should not change")
	assert.Equal(t, "..."+strings.Repeat("x", MaxLabelLen-6)+"123", dst["str3"], "long string should be abbreviated")
}
