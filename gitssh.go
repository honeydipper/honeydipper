package main

import (
	crypto_ssh "golang.org/x/crypto/ssh"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
	"os"
	"path"
)

var loadedSSHAuth bool
var currentSSHAuth transport.AuthMethod

// GetGitSSHAuth : create an AuthMethod to be used for various git operations
func GetGitSSHAuth() transport.AuthMethod {
	if !loadedSSHAuth {
		loadedSSHAuth = true

		keypass := os.Getenv("HONEY_SSH_PASS")
		keybytes := os.Getenv("HONEY_SSH_KEY")
		keyfile := os.Getenv("HONEY_SSH_KEYFILE")
		keysock := os.Getenv("SSH_AUTH_SOCK")

		if len(keybytes) > 0 || len(keyfile) > 0 || len(keysock) == 0 {
			if len(keybytes) > 0 {
				if auth, e := ssh.NewPublicKeys("git", []byte(keybytes), keypass); e == nil {
					auth.HostKeyCallback = crypto_ssh.InsecureIgnoreHostKey()
					currentSSHAuth = auth
				} else {
					log.Panicf("Unable load ssh key: %v", e)
				}
			} else {
				if len(keyfile) == 0 {
					keyfile = path.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
				}
				if auth, e := ssh.NewPublicKeysFromFile("git", keyfile, keypass); e == nil {
					auth.HostKeyCallback = crypto_ssh.InsecureIgnoreHostKey()
					currentSSHAuth = auth
				} else {
					log.Panicf("Unable load ssh key file: %v", e)
				}
			}
		}
	}

	return currentSSHAuth
}
