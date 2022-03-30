// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package config

import (
	"os"
	"path"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	crypto_ssh "golang.org/x/crypto/ssh"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

var (
	loadedSSHAuth  bool
	currentSSHAuth transport.AuthMethod
)

// GetGitSSHAuth creates an AuthMethod to be used for various git operations.
func GetGitSSHAuth() transport.AuthMethod {
	if loadedSSHAuth {
		return currentSSHAuth
	}

	keypass := os.Getenv("DIPPER_SSH_PASS")
	keybytes := os.Getenv("DIPPER_SSH_KEY")
	keyfile := os.Getenv("DIPPER_SSH_KEYFILE")
	keysock := os.Getenv("SSH_AUTH_SOCK")

	switch {
	case keybytes != "":
		if auth, e := ssh.NewPublicKeys("git", []byte(keybytes), keypass); e == nil {
			// #nosec
			auth.HostKeyCallback = crypto_ssh.InsecureIgnoreHostKey()
			currentSSHAuth = auth
		} else {
			dipper.Logger.Panicf("Unable load ssh key: %v", e)
		}
	case keysock != "":
		// using SSH_AUTH_SOCK to do ssh authentication
	default:
		if len(keyfile) == 0 {
			keyfile = path.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
		}
		if auth, e := ssh.NewPublicKeysFromFile("git", keyfile, keypass); e == nil {
			// #nosec
			auth.HostKeyCallback = crypto_ssh.InsecureIgnoreHostKey()
			currentSSHAuth = auth
		} else {
			dipper.Logger.Panicf("Unable load ssh key file: %v", e)
		}
	}

	loadedSSHAuth = true

	return currentSSHAuth
}
