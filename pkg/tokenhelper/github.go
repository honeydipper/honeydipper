// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package tokenhelper provides a helper functions to get API tokens.
package tokenhelper

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const _globalGitHubURL = "https://api.github.com"

var (
	ErrRetrieveToken = errors.New("unable to fetch token")
	tokenMutexes     sync.Map // maps source identifier to *sync.Mutex
)

func getGitHubJWT(s map[string]interface{}) (string, time.Time) {
	expiresAt := time.Now().Add(time.Minute * 9).Truncate(time.Second)
	claims := &jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Second * 30).Truncate(time.Second)),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		Issuer:    s["app_id"].(string),
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	var pk *rsa.PrivateKey
	if b, ok := s["_parsed_key"]; ok {
		pk = b.(*rsa.PrivateKey)
	} else {
		b := dipper.MustGetMapDataStr(s, "key")
		pk = dipper.Must(jwt.ParseRSAPrivateKeyFromPEM([]byte(b))).(*rsa.PrivateKey)
		s["_parsed_key"] = pk
	}
	jwtTokenStr := dipper.Must(jwtToken.SignedString(pk)).(string)

	return jwtTokenStr, expiresAt
}

func SetupGitHubSourceDefault(s map[string]interface{}) {
	if _, ok := s["github_url"]; !ok {
		s["github_url"] = _globalGitHubURL
	}

	if _, ok := s["permissions"]; !ok {
		s["permissions"] = map[string]interface{}{
			"contents": "read",
		}
	}

	if _, ok := s["installation_id"]; !ok {
		s["installation_id"] = os.Getenv("GH_APP_INSTALLATION_ID")
		if s["installation_id"] == "" {
			panic(fmt.Errorf("%w: installation_id missing", ErrRetrieveToken))
		}
	}

	if _, ok := s["app_id"]; !ok {
		s["app_id"] = os.Getenv("GH_APP_ID")
		if s["app_id"] == "" {
			panic(fmt.Errorf("%w: app_id missing", ErrRetrieveToken))
		}
	}

	if _, ok := s["key"]; !ok {
		s["key"] = os.Getenv("GH_APP_PRIVATE_KEY")
		if s["key"] == "" {
			panic(fmt.Errorf("%w: key missing", ErrRetrieveToken))
		}
	}
}

func GetGitHubToken(s map[string]interface{}) string {
	// Lock by map identity so concurrent callers for the same source share one mutex.
	sourceID := reflect.ValueOf(s).Pointer()
	muRaw, _ := tokenMutexes.LoadOrStore(sourceID, &sync.Mutex{})
	mu := muRaw.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	SetupGitHubSourceDefault(s)

	if mock, ok := s["mock"]; ok && mock != nil {
		s["_saved"] = mock.(map[string]interface{})["token"]
		s["_expiresAt"] = mock.(map[string]interface{})["expiresAt"]

		return s["_saved"].(string)
	}

	saved, ok := s["_saved"]
	if ok {
		exp := dipper.MustGetMapData(s, "_expiresAt").(time.Time)
		if time.Now().Add(2 * time.Second).Before(exp) {
			return saved.(string)
		}
	}

	jwtTokenStr, expiresAt := getGitHubJWT(s)

	header := http.Header{}
	header.Set("Accept", "application/vnd.github+json")
	header.Set("Authorization", "Bearer "+jwtTokenStr)
	dipper.Logger.Debugf("the gh jwt is %s", jwtTokenStr)

	permissions := dipper.MustGetMapData(s, "permissions").(map[string]interface{})
	contentBytes := dipper.Must(json.Marshal(map[string]interface{}{
		"permissions": permissions,
	})).([]byte)
	buf := bytes.NewBuffer(contentBytes)

	instID := dipper.MustGetMapDataStr(s, "installation_id")

	u, ok := s["github_url"]
	if !ok {
		u = _globalGitHubURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	req := dipper.Must(http.NewRequestWithContext(ctx, "POST", u.(string)+"/app/installations/"+instID+"/access_tokens", buf)).(*http.Request)
	req.Header = header
	client := http.Client{}
	//nolint: bodyClose
	resp := dipper.Must(client.Do(req)).(*http.Response)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		panic(fmt.Errorf("%w with code %d: %s",
			ErrRetrieveToken,
			resp.StatusCode,
			string(dipper.Must(io.ReadAll(resp.Body)).([]byte)),
		))
	}

	bodyObj := map[string]interface{}{}
	dipper.Must(json.Unmarshal(dipper.Must(io.ReadAll(resp.Body)).([]byte), &bodyObj))

	token := dipper.MustGetMapDataStr(bodyObj, "token")
	s["_saved"] = token
	s["_expiresAt"] = expiresAt

	return token
}
