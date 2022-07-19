// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
)

func TestGetGitSSHAuth(t *testing.T) {
	currentSSHAuth = map[string]transport.AuthMethod{}
	os.Unsetenv("SSH_AUTH_SOCK")
	os.Setenv("DIPPER_SSH_KEYFILE", "test_fixtures/not_secured")
	defer os.Unsetenv("DIPPER_SSH_KEYFILE")
	assert.NotPanics(t, func() {
		a := GetGitSSHAuth("", "")
		assert.NotNil(t, a, "GitSSH should load the key specified by DIPPER_SSH_KEYFILE environment variable")
	}, "GitSSH should not panic loading key specified by DIPPER_SSH_KEYFILE environment variable")
}

func TestGetGitSSHAuthParameter(t *testing.T) {
	currentSSHAuth = map[string]transport.AuthMethod{}
	os.Unsetenv("SSH_AUTH_SOCK")
	os.Setenv("DIPPER_SSH_KEYFILE", "test_fixtures/not_secured")
	os.Setenv("SECOND_SSH_PASS", "x1234")
	defer os.Unsetenv("DIPPER_SSH_KEYFILE")
	defer os.Unsetenv("SECOND_SSH_PASS")
	assert.NotPanics(t, func() {
		a := GetGitSSHAuth("test_fixtures/not_secured2", "SECOND_SSH_PASS")
		assert.NotNil(t, a, "GitSSH should load the key specified by parameter")
	}, "GitSSH should not panic loading key specified by parameter")
	GetGitSSHAuth("", "")
	assert.Equal(t, 2, len(currentSSHAuth), "should be able to load multiple keys")
}

func TestGetGitSSHAuthPanic(t *testing.T) {
	currentSSHAuth = map[string]transport.AuthMethod{}
	os.Unsetenv("SSH_AUTH_SOCK")
	assert.Panics(t, func() {
		GetGitSSHAuth("", "x1234")
	}, "GitSSH should panic if no key is specified")
}
