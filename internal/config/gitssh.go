// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package config

import (
	"os"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	crypto_ssh "golang.org/x/crypto/ssh"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

var currentSSHAuth map[string]transport.AuthMethod = map[string]transport.AuthMethod{}

// GetGitSSHAuth creates an AuthMethod to be used for various git operations.
func GetGitSSHAuth(keyfile, keypassEnv string) transport.AuthMethod {
	keypass := os.Getenv("DIPPER_SSH_PASS")
	if keyfile == "" {
		keyfile = os.Getenv("DIPPER_SSH_KEYFILE")
	} else {
		if keypassEnv == "" {
			keypass = ""
		} else {
			keypass = os.Getenv(keypassEnv)
		}
	}

	if loaded, ok := currentSSHAuth[keyfile]; ok {
		return loaded
	}

	keysock := os.Getenv("SSH_AUTH_SOCK")
	if keyfile == "" && keysock == "" {
		dipper.Logger.Panicf("Unable load ssh key: no key file specified")
	}

	keybytes := os.Getenv("DIPPER_SSH_KEY")

	switch {
	case keybytes != "":
		if auth, e := ssh.NewPublicKeys("git", []byte(keybytes), keypass); e == nil {
			// #nosec
			auth.HostKeyCallback = crypto_ssh.InsecureIgnoreHostKey()
			currentSSHAuth[keyfile] = auth
		} else {
			dipper.Logger.Panicf("Unable load ssh key: %v", e)
		}
	case keysock != "":
		// using SSH_AUTH_SOCK to do ssh authentication
	default:
		if auth, e := ssh.NewPublicKeysFromFile("git", keyfile, keypass); e == nil {
			// #nosec
			auth.HostKeyCallback = crypto_ssh.InsecureIgnoreHostKey()
			currentSSHAuth[keyfile] = auth
		} else {
			dipper.Logger.Panicf("Unable load ssh key file: %v", e)
		}
	}

	return currentSSHAuth[keyfile]
}
