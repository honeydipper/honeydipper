// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package web enables Honeydipper to make outbound web requests.
package main

import (
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/honeydipper/honeydipper/v4/pkg/tokenhelper"
)

func getToken(source string) string {
	s := dipper.MustGetMapData(driver.Options, "data.token_sources."+source).(map[string]interface{})
	switch s["type"].(string) {
	case "github":

		return tokenhelper.GetGitHubToken(s)
	default:
		log.Panicf("[%s] unknown token source type: %+v", driver.Service, s["type"])
	}

	return ""
}
