// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

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
