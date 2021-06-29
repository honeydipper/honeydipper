// Copyright 2021 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package datadog-emitter enables Honeydipper to send metrics to datadog
package main

import "github.com/DataDog/datadog-go/statsd"

type virtualStatsd interface {
	Close() error
	Event(*statsd.Event) error
	Incr(string, []string, float64) error
	Gauge(string, float64, []string, float64) error
}

func newStatsd(datadogOptStr string) (virtualStatsd, error) {
	if mockedstatsd != nil {
		return mockedstatsd, nil
	}

	return statsd.New(datadogOptStr)
}
