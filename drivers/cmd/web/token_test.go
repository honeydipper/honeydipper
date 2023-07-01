// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package main

import (
	"encoding/base64"
	"io/ioutil"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestGetToken(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/app/installations/123/access_token").
		Reply(201).
		JSON(map[string]string{"token": "foobar"})

	keyb64 := dipper.Must(ioutil.ReadFile("test_fixtures/testkey")).([]byte)
	keybytes := dipper.Must(base64.StdEncoding.DecodeString(string(keyb64))).([]byte)

	githubSource := map[string]interface{}{
		"type":            "github",
		"app_id":          "345",
		"installation_id": "123",
		"key":             string(keybytes),
		"permissions": map[string]interface{}{
			"content": "write",
		},
	}

	driver.Options = map[string]interface{}{
		"data": map[string]interface{}{
			"token_sources": map[string]interface{}{
				"test1": githubSource,
			},
		},
	}

	var token string
	assert.NotPanicsf(t, func() { token = getToken("test1") }, "should not panic when getting token")
	assert.Equalf(t, "foobar", token, "should get the test token")
	assert.Containsf(t, githubSource, "_saved", "should save the token for future use")
	assert.Containsf(t, githubSource, "_expiresAt", "should save the expiresAt")
	assert.Containsf(t, githubSource, "_parsed_key", "should save the expiresAt")

	githubSource["_saved"] = "newtoken"
	assert.NotPanicsf(t, func() { token = getToken("test1") }, "should not panic when getting token from cache")
	assert.Equalf(t, "newtoken", token, "should get the saved token")

	gock.New("https://api.github.com").
		Post("/app/installations/123/access_token").
		Reply(201).
		JSON(map[string]string{"token": "foobar2"})

	githubSource["_expiresAt"] = time.Now().Add(-time.Minute * 15)
	assert.NotPanicsf(t, func() { token = getToken("test1") }, "should not panic when refreshing token")
	assert.Equalf(t, "foobar2", token, "should get a new token if expired")

	gock.New("https://api.github.com").
		Post("/app/installations/123/access_token").
		Reply(201).
		JSON(map[string]string{"token": "foobar3"})

	githubSource["_expiresAt"] = time.Now().Add(-time.Minute * 15)
	githubSource["key"] = ""
	assert.NotPanicsf(t, func() { token = getToken("test1") }, "should not panic when refreshing token with _parsed_key")
	assert.Equalf(t, "foobar3", token, "should get a new token using _parsed_key")
}
