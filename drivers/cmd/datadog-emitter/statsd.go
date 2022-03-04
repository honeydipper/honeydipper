// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package datadog-emitter enables Honeydipper to send metrics to datadog
package main

import (
	"fmt"

	"github.com/DataDog/datadog-go/statsd"
)

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
	d, e := statsd.New(datadogOptStr)
	if e != nil {
		return d, fmt.Errorf("wrapping statsd library error: %w", e)
	}

	return d, nil
}
