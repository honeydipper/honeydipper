// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"encoding/base64"
	"strings"
)

// GetDecryptFunc returns a function used in recursive decryption.
func GetDecryptFunc(rpc RPCCaller) ItemProcessor {
	return func(key string, val interface{}) (interface{}, bool) {
		Logger.Debugf("[%s] decrypting %s", rpc.GetName(), key)
		str, ok := val.(string)
		if !ok {
			return nil, false
		}

		switch {
		case strings.HasPrefix(str, "ENC["):
			//nolint:gomnd
			parts := strings.SplitN(str[4:len(str)-1], ",", 2)
			encDriver := parts[0]
			if encDriver == "deferred" {
				return "ENC[" + parts[1] + "]", true
			}
			decoded, err := base64.StdEncoding.DecodeString(parts[1])
			if err != nil {
				Logger.Panicf("encrypted data should be base64 encoded")
			}
			decrypted, _ := rpc.CallRaw("driver:"+encDriver, "decrypt", decoded)

			return string(decrypted), true
		case strings.HasPrefix(str, "LOOKUP["):
			//nolint:gomnd
			parts := strings.SplitN(str[7:len(str)-1], ",", 2)
			lookupDriver := parts[0]
			if lookupDriver == "deferred" {
				return "LOOKUP[" + parts[1] + "]", true
			}
			lookupValue, _ := rpc.CallRaw("driver:"+lookupDriver, "lookup", []byte(parts[1]))

			return string(lookupValue), true
		}

		return nil, false
	}
}

// DecryptAll find and decrypt all eyaml style encrypted data in the given data structure.
func DecryptAll(rpc RPCCaller, from interface{}) {
	Recursive(from, GetDecryptFunc(rpc))
}
