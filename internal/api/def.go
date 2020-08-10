// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package api

import (
	"net/http"
	"time"
)

// Def is a structure defines how an API should be handled in api service.
type Def struct {
	path       string
	name       string
	method     string
	reqType    int
	service    string
	ackTimeout time.Duration
	timeout    time.Duration
}

const (
	// TypeFirst means the API uses first return from whichever node responds.
	TypeFirst = iota
	// TypeAll means returns from all nodes are used.
	TypeAll
	// TypeMatch means the API allows the node who has the matching record to respond.
	TypeMatch

	// InfiniteDuration is used to specify a timeout of infinity duration.
	InfiniteDuration time.Duration = -1
)

// GetDefs return definition for all known API calls.
func GetDefs() map[string]map[string]Def {
	return map[string]map[string]Def{
		"/events/:eventID/wait": {
			http.MethodGet: {name: "eventWait", reqType: TypeMatch, service: "engine", timeout: InfiniteDuration},
		},
		"/events": {
			http.MethodGet: {name: "eventList", reqType: TypeAll, service: "engine"},
		},
	}
}

// GetDefsByName return definition for all known API calls.
func GetDefsByName() map[string]Def {
	ret := map[string]Def{}
	for path, defs := range GetDefs() {
		for method, def := range defs {
			def.path = path
			def.method = method
			ret[def.name] = def
		}
	}
	return ret
}
