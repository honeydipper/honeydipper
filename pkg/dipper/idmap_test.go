// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

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
