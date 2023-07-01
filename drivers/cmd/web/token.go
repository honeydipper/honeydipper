// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package web enables Honeydipper to make outbound web requests.
package main

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

const globalGitHubURL = "https://api.github.com"

func getToken(source string) string {
	s := dipper.MustGetMapData(driver.Options, "data.token_sources."+source).(map[string]interface{})
	switch s["type"].(string) {
	case "github":

		return getGitHubToken(s)
	default:
		log.Panicf("[%s] unknown token source type: %+v", driver.Service, s["type"])
	}

	return ""
}

func getGitHubJWT(s map[string]interface{}) (string, time.Time) {
	//nolint: gomnd
	expiresAt := time.Now().Add(time.Minute * 10).Truncate(time.Second)
	claims := &jwt.RegisteredClaims{
		//nolint: gomnd
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

func getGitHubToken(s map[string]interface{}) string {
	saved, ok := s["_saved"]
	if ok {
		exp := dipper.MustGetMapData(s, "_expiresAt").(time.Time)
		//nolint: gomnd
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
		u = globalGitHubURL
	}
	req := dipper.Must(http.NewRequest("POST", u.(string)+"/app/installations/"+instID+"/access_tokens", buf)).(*http.Request)
	req.Header = header
	client := http.Client{}
	//nolint: bodyClose
	resp := dipper.Must(client.Do(req)).(*http.Response)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		log.Panicf("[%s] failed to fetch github access token with status code %+v, %+v",
			driver.Service,
			resp.StatusCode,
			string(dipper.Must(io.ReadAll(resp.Body)).([]byte)),
		)
	}

	bodyObj := map[string]interface{}{}
	dipper.Must(json.Unmarshal(dipper.Must(io.ReadAll(resp.Body)).([]byte), &bodyObj))

	token := dipper.MustGetMapDataStr(bodyObj, "token")
	s["_saved"] = token
	s["_expiresAt"] = expiresAt

	return token
}
