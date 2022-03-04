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

func TestMessageCopy(t *testing.T) {
	src := &Message{
		Channel: "c1",
		Subject: "s1",
		Labels: map[string]string{
			"label1": "value1",
		},
	}

	dst, err := MessageCopy(src)
	assert.Nil(t, err, "copy message should not raise err")
	assert.Equal(t, src.Channel, dst.Channel, "channel copied")
	assert.Equal(t, src.Subject, dst.Subject, "subject copied")
	assert.Equal(t, len(src.Labels), len(dst.Labels), "same number of labels")
	assert.Equal(t, src.Labels["label1"], dst.Labels["label1"], "the same label value")

	dst2, err := MessageCopy(nil)
	assert.Nil(t, err, "copy message should not raise err")
	assert.Nil(t, dst2, "Error: Copy of nil should be nil")
}
