// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"os"
	"path"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	crypto_ssh "golang.org/x/crypto/ssh"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

var loadedSSHAuth bool
var currentSSHAuth transport.AuthMethod

// GetGitSSHAuth creates an AuthMethod to be used for various git operations
func GetGitSSHAuth() transport.AuthMethod {
	if !loadedSSHAuth {
		loadedSSHAuth = true

		keypass := os.Getenv("DIPPER_SSH_PASS")
		keybytes := os.Getenv("DIPPER_SSH_KEY")
		keyfile := os.Getenv("DIPPER_SSH_KEYFILE")
		keysock := os.Getenv("SSH_AUTH_SOCK")

		if len(keybytes) > 0 || len(keyfile) > 0 || len(keysock) == 0 {
			if len(keybytes) > 0 {
				if auth, e := ssh.NewPublicKeys("git", []byte(keybytes), keypass); e == nil {
					auth.HostKeyCallback = crypto_ssh.InsecureIgnoreHostKey()
					currentSSHAuth = auth
				} else {
					dipper.Logger.Panicf("Unable load ssh key: %v", e)
				}
			} else {
				if len(keyfile) == 0 {
					keyfile = path.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
				}
				if auth, e := ssh.NewPublicKeysFromFile("git", keyfile, keypass); e == nil {
					auth.HostKeyCallback = crypto_ssh.InsecureIgnoreHostKey()
					currentSSHAuth = auth
				} else {
					dipper.Logger.Panicf("Unable load ssh key file: %v", e)
				}
			}
		}
	}

	return currentSSHAuth
}
