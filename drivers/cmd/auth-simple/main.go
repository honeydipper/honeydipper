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
	for _, scheme := range schemes {
		switch scheme.(string) {
		case "basic":
			err = basicAuth(m)
		case "token":
			err = tokenAuth(m)
		default:
			panic(ErrUnsupportedScheme)
		}
		if err == nil {
			m.Reply <- dipper.Message{
				Payload: map[string]interface{}{
					"auth": "true",
				},
			}
			return
		} else if !errors.Is(err, ErrSkipped) {
			break
		}
	}
	panic(err)
}

func tokenAuth(m *dipper.Message) error {
	const prefix = "bearer "
	authHash, ok := dipper.GetMapDataStr(m.Payload, "headers.Authorization.0")
	if !ok {
		authHash, ok = dipper.GetMapDataStr(m.Payload, "headers.authorization.0")
		if !ok || len(authHash) < len(prefix) || !strings.EqualFold(authHash[:len(prefix)], prefix) {
			return ErrSkipped
		}
	}
	if _, ok = driver.GetOption("decrypted"); !ok {
		dipper.DecryptAll(&driver.RPCCaller, driver.Options)
		driver.Options.(map[string]interface{})["decrypted"] = true
	}
	if !dipper.CompareAll(authHash, dipper.MustGetMapData(driver.Options, "data.tokens")) {
		return ErrInvalidBearerToken
	}

	return nil
}

func basicAuth(m *dipper.Message) error {
	const prefix = "basic "
	authHash, ok := dipper.GetMapDataStr(m.Payload, "headers.Authorization.0")
	if !ok {
		authHash, ok = dipper.GetMapDataStr(m.Payload, "headers.authorization.0")
		if !ok || len(authHash) < len(prefix) || !strings.EqualFold(authHash[:len(prefix)], prefix) {
			return ErrSkipped
		}
	}
	req := &http.Request{
		Header: http.Header{
			"Authorization": []string{authHash},
		},
	}
	user, pass, ok := req.BasicAuth()
	if !ok {
		return ErrInvalidBasicAuth
	}
	if _, ok = driver.GetOption("decrypted"); !ok {
		dipper.DecryptAll(&driver.RPCCaller, driver.Options)
		driver.Options.(map[string]interface{})["decrypted"] = true
	}

	if !dipper.CompareAll(map[string]interface{}{
		"name": user,
		"pass": pass,
	}, dipper.MustGetMapData(driver.Options, "data.users")) {
		return ErrInvalidBasicCreds
	}

	return nil
}
