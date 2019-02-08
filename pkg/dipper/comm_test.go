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
}
