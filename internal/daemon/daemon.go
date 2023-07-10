// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package daemon provides capability to execute the program as a daemon.
package daemon

import (
	"sync"

	"github.com/honeydipper/honeydipper/internal/config"
)

// Emitter is an interface provide data metric emitting feature.
type Emitter interface {
	// Increase a counter metric.
	CounterIncr(metric string, tags []string)
	// Set value for a gauge metric.
	GaugeSet(metric string, value string, tags []string)
}

// ShuttingDown is a flag that is set to true if the daemon is in the shutdown process.
var ShuttingDown bool

// Children keeps track of the child go routines in the daemon.
var Children = &sync.WaitGroup{}

// Emitters contains a group of metrics emitter for sending metrics to external monitoring systems.
var Emitters = map[string]Emitter{}

// OnStart will be called when the daemon starts after config is loaded.
var OnStart func()

// ShutDown the daemon gracefully.
func ShutDown() {
	ShuttingDown = true

	Children.Wait()
}

// Run is the entry point of the daemon.
func Run(cfg *config.Config) {
	cfg.Bootstrap("/tmp")
	OnStart()
	cfg.Watch()
}

// Go launches a go routine with necessary prep and cleanup in the wrapper.
func Go(f func()) {
	Children.Add(1)
	go func() {
		defer Children.Done()
		f()
	}()
}
