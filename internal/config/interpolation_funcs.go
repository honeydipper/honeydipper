// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package config

import (
	"text/template"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

func getPodIDSigningSecret(cfg *Config) string {
	if cfg == nil || cfg.DataSet == nil || cfg.DataSet.Drivers == nil {
		return ""
	}

	secret, _ := dipper.GetMapDataStr(cfg.DataSet.Drivers, "daemon.workflow.pod_id_signing_secret")

	return secret
}

// GetInterpolationFuncMap returns config-aware interpolation helpers.
func GetInterpolationFuncMap(cfg *Config) template.FuncMap {
	secret := getPodIDSigningSecret(cfg)
	if secret == "" {
		return template.FuncMap{}
	}

	return template.FuncMap{
		"resolve_gh_slug": func(values ...interface{}) string {
			return dipper.FirstNonEmptyString(values...)
		},
		"sign_pod_id": func(podID interface{}, values ...interface{}) string {
			pod := dipper.FirstNonEmptyString(podID)
			ghSlug := dipper.FirstNonEmptyString(values...)
			payload := dipper.PodIDSignaturePayload(pod, ghSlug)

			return dipper.SignPayload(secret, payload)
		},
	}
}
