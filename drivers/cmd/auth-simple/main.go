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
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrUnsupportedScheme means the auth scheme is not supported.
	ErrUnsupportedScheme = errors.New("the auth scheme is not supported")
	// ErrInvalidBearerToken means the bearer token is invalid.
	ErrInvalidBearerToken = errors.New("the bearer token is invalid")
	// ErrInvalidBasicAuth means the basic auth header is invalid.
	ErrInvalidBasicAuth = errors.New("the basic auth header is invalid")
	// ErrInvalidBasicCreds means the basic auth user and password is invalid.
	ErrInvalidBasicCreds = errors.New("the basic auth credential is invalid")
	// ErrSkipped means skipping the current scheme.
	ErrSkipped = errors.New("skipped")

	// EmptySubject is used for an authenticated user without a defined subject.
	_EmptySubject = ""
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
	var subject string
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

func tokenAuth(m *dipper.Message) (string, error) {
	const prefix = "bearer "
	authHash, ok := dipper.GetMapDataStr(m.Payload, "headers.Authorization.0")
	if !ok {
		authHash, ok = dipper.GetMapDataStr(m.Payload, "headers.authorization.0")
	}
	if !ok || len(authHash) < len(prefix) || !strings.EqualFold(authHash[:len(prefix)], prefix) {
		return _EmptySubject, ErrSkipped
	}
	token := []byte(authHash[len(prefix):])

	if _, ok = driver.GetOption("decrypted"); !ok {
		dipper.DecryptAll(driver, driver.Options)
		driver.Options.(map[string]interface{})["decrypted"] = true
	}
	knownTokens, ok := dipper.GetMapData(driver.Options, "data.tokens")
	if ok && knownTokens != nil {
		for _, t := range knownTokens.([]interface{}) {
			kt := t.(map[string]interface{})
			if bcrypt.CompareHashAndPassword([]byte(kt["token"].(string)), token) == nil {
				subject, ok := kt["subject"]
				if !ok || subject == nil {
					return _EmptySubject, nil
				}
				return subject.(string), nil
			}
		}
	}

	return _EmptySubject, ErrInvalidBearerToken
}

func basicAuth(m *dipper.Message) (string, error) {
	const prefix = "basic "
	authHash, ok := dipper.GetMapDataStr(m.Payload, "headers.Authorization.0")
	if !ok {
		authHash, ok = dipper.GetMapDataStr(m.Payload, "headers.authorization.0")
	}
	if !ok || len(authHash) < len(prefix) || !strings.EqualFold(authHash[:len(prefix)], prefix) {
		return _EmptySubject, ErrSkipped
	}
	req := &http.Request{
		Header: http.Header{
			"Authorization": []string{authHash},
		},
	}
	user, pass, ok := req.BasicAuth()
	if !ok {
		return _EmptySubject, ErrInvalidBasicAuth
	}
	passBytes := []byte(pass)

	if _, ok = driver.GetOption("decrypted"); !ok {
		dipper.DecryptAll(driver, driver.Options)
		driver.Options.(map[string]interface{})["decrypted"] = true
	}
	knownUsers, ok := dipper.GetMapData(driver.Options, "data.users")
	if ok && knownUsers != nil {
		for _, k := range knownUsers.([]interface{}) {
			ku := k.(map[string]interface{})
			if ku["name"].(string) == user && bcrypt.CompareHashAndPassword([]byte(ku["pass"].(string)), passBytes) == nil {
				subject, ok := ku["subject"]
				if !ok || subject == nil {
					return _EmptySubject, nil
				}
				return subject.(string), nil
			}
		}
	}

	return _EmptySubject, ErrInvalidBasicCreds
}
