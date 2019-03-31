// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package honeydipper is an event-driven, rule based orchestration platform tailor towards
// DevOps and system engineering workflows.
package main

import (
	"fmt"
	"strings"

	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/logrusorgru/aurora"
)

func runConfigCheck(cfg *config.Config) bool {
	hasError := false
	for spec, repo := range cfg.Loaded {
		if len(repo.Errors) > 0 {
			hasError = true
			fmt.Printf("Repo [%s] Branch [%s] Path [%s]\n", aurora.Cyan(spec.Repo), aurora.Cyan(spec.Branch), aurora.Cyan(spec.Path))
			fmt.Println("─────────────────────────────────────────────────────────────")
			for _, err := range repo.Errors {
				msg := err.Error.Error()
				// transforming error message
				msg = strings.Replace(msg, "error converting YAML to JSON: yaml: ", "", 1)

				fmt.Printf("%s: %s\n", err.File[1:], aurora.Red(msg))
			}
		}
	}
	return hasError
}
