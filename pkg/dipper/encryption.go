// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrConfigProcessing indicates config decryption or lookup processing failed.
var ErrConfigProcessing = errors.New("config processing failed")

func panicConfigProcessingError(err error) {
	Logger.Error(err)
	panic(err)
}

func splitConfigRef(rpc RPCCaller, key string, refType string, ref string, prefixLen int) []string {
	if len(ref) <= prefixLen || !strings.HasSuffix(ref, "]") {
		panicConfigProcessingError(
			fmt.Errorf(
				"%w: [%s] invalid %s config value for key %s",
				ErrConfigProcessing,
				rpc.GetName(), refType, key,
			),
		)
	}

	parts := strings.SplitN(ref[prefixLen:len(ref)-1], ",", 2)
	if len(parts) != 2 {
		panicConfigProcessingError(
			fmt.Errorf(
				"%w: [%s] invalid %s config value for key %s",
				ErrConfigProcessing,
				rpc.GetName(), refType, key,
			),
		)
	}

	return parts
}

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
			parts := splitConfigRef(rpc, key, "ENC", str, 4)
			encDriver := parts[0]
			if encDriver == "deferred" {
				return "ENC[" + parts[1] + "]", true
			}
			decoded, err := base64.StdEncoding.DecodeString(parts[1])
			if err != nil {
				panicConfigProcessingError(
					fmt.Errorf(
						"%w: [%s] encrypted config value for key %s should be base64 encoded: %w",
						ErrConfigProcessing,
						rpc.GetName(), key, err,
					),
				)
			}
			decrypted, err := rpc.CallRaw("driver:"+encDriver, "decrypt", decoded)
			if err != nil {
				panicConfigProcessingError(
					fmt.Errorf(
						"%w: [%s] failed to decrypt config key %s using driver %s: %w",
						ErrConfigProcessing,
						rpc.GetName(), key, encDriver, err,
					),
				)
			}

			return string(decrypted), true
		case strings.HasPrefix(str, "LOOKUP["):
			//nolint:gomnd
			parts := splitConfigRef(rpc, key, "LOOKUP", str, 7)
			lookupDriver := parts[0]
			if lookupDriver == "deferred" {
				return "LOOKUP[" + parts[1] + "]", true
			}
			lookupValue, err := rpc.CallRaw("driver:"+lookupDriver, "lookup", []byte(parts[1]))
			if err != nil {
				panicConfigProcessingError(
					fmt.Errorf(
						"%w: [%s] failed to resolve LOOKUP using driver %s for config key %s: %w",
						ErrConfigProcessing,
						rpc.GetName(), lookupDriver, key, err,
					),
				)
			}

			return string(lookupValue), true
		}

		return nil, false
	}
}

// DecryptAll find and decrypt all eyaml style encrypted data in the given data structure.
func DecryptAll(rpc RPCCaller, from interface{}) {
	Recursive(from, GetDecryptFunc(rpc))
}
