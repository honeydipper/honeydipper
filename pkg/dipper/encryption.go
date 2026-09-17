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

var ErrConfigProcessing = errors.New("config processing failed")

func panicConfigProcessingError(err error) {
	Logger.Error(err)
	panic(err)
}

func splitConfigRef(rpc RPCCaller, key, refType, ref string, prefixLen int) ([]string, string) {
	closingIdx := strings.Index(ref, "]")
	if closingIdx < prefixLen {
		panicConfigProcessingError(
			fmt.Errorf(
				"%w: [%s] invalid %s config value for key %s",
				ErrConfigProcessing,
				rpc.GetName(),
				refType,
				key,
			),
		)
	}

	pattern := "%s"
	if closingIdx < len(ref)-1 && ref[closingIdx+1] == ':' {
		pattern = ref[closingIdx+2:]
	}

	parts := strings.SplitN(ref[prefixLen:closingIdx], ",", 2)
	if len(parts) != 2 {
		panicConfigProcessingError(
			fmt.Errorf(
				"%w: [%s] invalid %s config value for key %s",
				ErrConfigProcessing,
				rpc.GetName(),
				refType,
				key,
			),
		)
	}

	return parts, pattern
}

// DecryptString uses the appropriate driver to decrypt the given string.
func DecryptString(rpc RPCCaller, key, str string) (string, bool) {
	optional := false

	switch {
	case strings.HasPrefix(str, "ENC["):
		Logger.Debugf("[%s] decrypting %s", rpc.GetName(), key)
		parts, pattern := splitConfigRef(rpc, key, "ENC", str, 4)
		encDriver := parts[0]
		if encDriver == "deferred" {
			p := parts[1]
			if parts[1][0] == '?' {
				p = parts[1][1:]
			}
			result := "ENC[" + p + "]"
			if pattern != "%s" {
				result += ":" + pattern
			}

			return result, true
		}
		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			panicConfigProcessingError(
				fmt.Errorf(
					"%w: [%s] encrypted config value for key %s should be base64 encoded: %w",
					ErrConfigProcessing,
					rpc.GetName(),
					key,
					err,
				),
			)
		}
		decrypted, err := rpc.CallRaw("driver:"+encDriver, "decrypt", decoded)
		if err != nil && !optional {
			panicConfigProcessingError(
				fmt.Errorf(
					"%w: [%s] failed to decrypt config key %s using driver %s: %w",
					ErrConfigProcessing,
					rpc.GetName(),
					key,
					encDriver,
					err,
				),
			)
		}

		return fmt.Sprintf(pattern, string(decrypted)), true
	case strings.HasPrefix(str, "LOOKUP["):
		Logger.Debugf("[%s] looking up %s", rpc.GetName(), key)
		parts, pattern := splitConfigRef(rpc, key, "LOOKUP", str, 7)
		lookupDriver := parts[0]
		if lookupDriver == "deferred" {
			result := "LOOKUP[" + parts[1] + "]"
			if pattern != "%s" {
				result += ":" + pattern
			}

			return result, true
		}
		p := parts[1]
		if parts[1][0] == '?' {
			optional = true
			p = parts[1][1:]
		}
		lookupValue, err := rpc.CallRaw("driver:"+lookupDriver, "lookup", []byte(p))
		if err != nil && !optional {
			panicConfigProcessingError(
				fmt.Errorf(
					"%w: [%s] failed to resolve LOOKUP using driver %s for config key %s: %w",
					ErrConfigProcessing,
					rpc.GetName(),
					lookupDriver,
					key,
					err,
				),
			)
		}

		return fmt.Sprintf(pattern, string(lookupValue)), true
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
