// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package auth-simple enables Honeydipper to authenticate/authorize incoming web requests.
package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

var (
	// ErrUnsupportedScheme means the auth scheme is not supported
	ErrUnsupportedScheme = errors.New("the auth scheme is not supported")
	// ErrInvalidBearerToken means the bearer token is invalid
	ErrInvalidBearerToken = errors.New("the bearer token is invalid")
	// ErrInvalidBasicAuth means the basic auth header is invalid
	ErrInvalidBasicAuth = errors.New("the basic auth header is invalid")
	// ErrInvalidBasicCreds means the basic auth user and password is invalid
	ErrInvalidBasicCreds = errors.New("the basic auth credential is invalid")
	// ErrInvalidUserEntry means user entry in the config is invalid
	ErrInvalidUserEntry = errors.New("the user entry is invalid")
	// ErrSkipped means skipping the current scheme
	ErrSkipped = errors.New("skipped")
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports receiver and API service.")
		fmt.Printf("  This program provides honeydipper with some simple capability of authenticating and authorizing the incoming web requests.")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "auth-simple")
	driver.RPCHandlers["auth_web_request"] = authWebRequest
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func authWebRequest(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	var schemes []interface{}
	schemesOpt, ok := driver.GetOption("data.schemes")
	if ok {
		schemes = schemesOpt.([]interface{})
	} else {
		schemes = []interface{}{"token"}
	}

	var err error
	var subject map[string]interface{}
	for _, scheme := range schemes {
		switch scheme.(string) {
		case "basic":
			subject, err = basicAuth(m)
		case "token":
			subject, err = tokenAuth(m)
		default:
			panic(ErrUnsupportedScheme)
		}
		if err == nil {
			m.Reply <- dipper.Message{
				Payload: subject,
			}
			return
		} else if !errors.Is(err, ErrSkipped) {
			break
		}
	}
	panic(err)
}

func tokenAuth(m *dipper.Message) (map[string]interface{}, error) {
	const prefix = "bearer "
	authHash, ok := dipper.GetMapDataStr(m.Payload, "headers.Authorization.0")
	if !ok {
		authHash, ok = dipper.GetMapDataStr(m.Payload, "headers.authorization.0")
		if !ok || len(authHash) < len(prefix) || !strings.EqualFold(authHash[:len(prefix)], prefix) {
			return nil, ErrSkipped
		}
	}
	if _, ok = driver.GetOption("decrypted"); !ok {
		dipper.DecryptAll(&driver.RPCCaller, driver.Options)
		driver.Options.(map[string]interface{})["decrypted"] = true
	}
	found, ok := dipper.GetMapData(driver.Options, "data.tokens."+authHash)
	if !ok {
		return nil, ErrInvalidBearerToken
	}
	subject, ok := found.(map[string]interface{})["subject"]
	if !ok || subject == nil || len(subject.(map[string]interface{})) == 0 {
		return nil, ErrInvalidUserEntry
	}

	return subject.(map[string]interface{}), nil
}

func basicAuth(m *dipper.Message) (map[string]interface{}, error) {
	const prefix = "basic "
	authHash, ok := dipper.GetMapDataStr(m.Payload, "headers.Authorization.0")
	if !ok {
		authHash, ok = dipper.GetMapDataStr(m.Payload, "headers.authorization.0")
		if !ok || len(authHash) < len(prefix) || !strings.EqualFold(authHash[:len(prefix)], prefix) {
			return nil, ErrSkipped
		}
	}
	req := &http.Request{
		Header: http.Header{
			"Authorization": []string{authHash},
		},
	}
	user, pass, ok := req.BasicAuth()
	if !ok {
		return nil, ErrInvalidBasicAuth
	}
	if _, ok = driver.GetOption("decrypted"); !ok {
		dipper.DecryptAll(&driver.RPCCaller, driver.Options)
		driver.Options.(map[string]interface{})["decrypted"] = true
	}

	knownUsers, ok := dipper.GetMapData(driver.Options, "data.users")
	if !ok || knownUsers == nil {
		return nil, ErrInvalidBasicCreds
	}
	for _, k := range knownUsers.([]interface{}) {
		ku := k.(map[string]interface{})
		if ku["name"].(string) == user && ku["pass"].(string) == pass {
			subject, ok := ku["subject"]
			if !ok || subject == nil || len(subject.(map[string]interface{})) == 0 {
				return nil, ErrInvalidUserEntry
			}
			return subject.(map[string]interface{}), nil
		}
	}

	return nil, ErrInvalidBasicCreds
}
