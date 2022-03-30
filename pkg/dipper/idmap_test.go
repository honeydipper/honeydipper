// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package dipper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIDMapGet(t *testing.T) {
	sessions := &map[string]string{}
	InitIDMap(sessions)
	id := IDMapPut(sessions, "foo")
	val := IDMapGet(sessions, id)
	assert.Equal(t, "foo", val, "idmap can store kv and operate with locks")
}
