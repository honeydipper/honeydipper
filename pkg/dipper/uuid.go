// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import (
	"github.com/google/uuid"
)

// UUIDSource is a function that returns uuids.
type UUIDSource func() string

// NewUUID returns a new UUID.
func NewUUID() string {
	return Must(uuid.NewRandom()).(uuid.UUID).String()
}
