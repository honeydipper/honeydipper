// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"encoding/base64"
	"strings"
)

// DecryptString uses the appropriate driver to decrypt the given string.
func DecryptString(rpc RPCCaller, key, str string) (string, bool) {
	optional := false

	switch {
	case strings.HasPrefix(str, "ENC["):
		Logger.Debugf("[%s] decrypting %s", rpc.GetName(), key)
		parts := strings.SplitN(str[4:len(str)-1], ",", 2)
		encDriver := parts[0]
		if encDriver == "deferred" {
			p := parts[1]
			if parts[1][0] == '?' {
				p = parts[1][1:]
			}

			return "ENC[" + p + "]", true
		}
		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			Logger.Panicf("encrypted data should be base64 encoded")
		}
		decrypted, e := rpc.CallRaw("driver:"+encDriver, "decrypt", decoded)
		if !optional {
			Must(e)
		}

		return string(decrypted), true
	case strings.HasPrefix(str, "LOOKUP["):
		Logger.Debugf("[%s] looking up %s", rpc.GetName(), key)
		parts := strings.SplitN(str[7:len(str)-1], ",", 2)
		lookupDriver := parts[0]
		if lookupDriver == "deferred" {
			return "LOOKUP[" + parts[1] + "]", true
		}
		p := parts[1]
		if parts[1][0] == '?' {
			optional = true
			p = parts[1][1:]
		}
		lookupValue, e := rpc.CallRaw("driver:"+lookupDriver, "lookup", []byte(p))
		if !optional {
			Must(e)
		}

		return string(lookupValue), true
	}

	return "", false
}

// GetDecryptFunc returns a function used in recursive decryption.
func GetDecryptFunc(rpc RPCCaller) ItemProcessor {
	return func(key string, val interface{}) (interface{}, bool) {
		str, ok := val.(string)
		if !ok {
			return nil, false
		}

		decrypted, updated := DecryptString(rpc, key, str)
		if updated {
			return decrypted, true
		}

		return nil, false
	}
}

// DecryptAll find and decrypt all eyaml style encrypted data in the given data structure.
func DecryptAll(rpc RPCCaller, from interface{}) {
	Recursive(from, GetDecryptFunc(rpc))
}
