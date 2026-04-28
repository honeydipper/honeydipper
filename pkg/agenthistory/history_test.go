// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package agenthistory

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseHistory_StringContent(t *testing.T) {
	raw := []byte(`[
		{"role":"system","content":"you are helpful"},
		{"role":"user","content":"hello"}
	]`)

	history, err := ParseHistory(raw)
	assert.NoError(t, err)
	assert.Len(t, history, 2)
	assert.Equal(t, RoleSystem, history[0].Role)
	if assert.NotNil(t, history[0].Content) {
		assert.Equal(t, "you are helpful", history[0].Content.Text)
	}
}

func TestParseHistory_PartsContent(t *testing.T) {
	raw := []byte(`[
		{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":"https://example/image.png"}]}
	]`)

	history, err := ParseHistory(raw)
	assert.NoError(t, err)
	assert.Len(t, history, 1)
	if assert.NotNil(t, history[0].Content) {
		assert.Len(t, history[0].Content.Parts, 2)
		assert.Equal(t, ContentPartTypeText, history[0].Content.Parts[0].Type)
		assert.Equal(t, ContentPartTypeImageURL, history[0].Content.Parts[1].Type)
	}
}

func TestMessageContent_MarshalJSON(t *testing.T) {
	textOnly := MessageContent{Text: "hello"}
	b, err := json.Marshal(textOnly)
	assert.NoError(t, err)
	assert.Equal(t, `"hello"`, string(b))

	parts := MessageContent{Parts: []ContentPart{{Type: ContentPartTypeText, Text: "hello"}}}
	b, err = json.Marshal(parts)
	assert.NoError(t, err)
	assert.Contains(t, string(b), `"type":"text"`)
}

func TestParseHistory_InvalidContent(t *testing.T) {
	raw := []byte(`[{"role":"user","content":123}]`)

	_, err := ParseHistory(raw)
	assert.Error(t, err)
}
