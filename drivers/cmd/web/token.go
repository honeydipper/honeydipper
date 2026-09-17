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

func applyTokenSourceParams(source map[string]interface{}, params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return source
	}

	// Clone top-level source map so per-request overrides do not mutate shared driver config.
	overridden := map[string]interface{}{}
	for k, v := range source {
		overridden[k] = v
	}

	if installationID, ok := dipper.GetMapDataStr(params, "installation_id"); ok && installationID != "" {
		overridden["installation_id"] = installationID
	}

	return overridden
}

func getToken(source string, params map[string]interface{}) string {
	s := dipper.MustGetMapData(driver.Options, "data.token_sources."+source).(map[string]interface{})
	t := applyTokenSourceParams(s, params)
	switch t["type"].(string) {
	case "github":

		return tokenhelper.GetGitHubToken(t)
	default:
		driver.GetLogger().Panicf("[%s] unknown token source type: %+v", driver.Service, t["type"])
	}

	return ""
}
