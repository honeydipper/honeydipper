// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import (
	"encoding/base64"
	"strings"
)

// DecryptAll find and decrypt all eyaml style encrypted data in the given data structure.
func DecryptAll(rpc RPCCaller, from interface{}) {
	Recursive(from, func(key string, val interface{}) (interface{}, bool) {
		Logger.Debugf("[%s] decrypting %s", rpc.GetName(), key)
		str, ok := val.(string)
		if !ok || !strings.HasPrefix(str, "ENC[") {
			return nil, false
		}

		parts := strings.SplitN(str[4:len(str)-1], ",", 2)
		encDriver := parts[0]
		if encDriver == "deferred" {
			return "ENC[" + parts[1] + "]", true
		}

		data := []byte(parts[1])
		decoded, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			Logger.Panicf("encrypted data shoud be base64 encoded")
		}

		decrypted, _ := rpc.CallRaw("driver:"+encDriver, "decrypt", decoded)

		return string(decrypted), true
	})
}
