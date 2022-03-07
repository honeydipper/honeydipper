// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"net/http"
	"time"
)

// Def is a structure defines how an API should be handled in api service.
type Def struct {
	Path       string
	Object     string
	Name       string
	Method     string
	ReqType    int
	Service    string
	AckTimeout time.Duration
	Timeout    time.Duration
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
		"events/:eventID/wait": {
			http.MethodGet: {Object: "event", Name: "eventWait", ReqType: TypeMatch, Service: "engine", Timeout: InfiniteDuration},
		},
		"events": {
			http.MethodGet:  {Object: "event", Name: "eventList", ReqType: TypeAll, Service: "engine"},
			http.MethodPost: {Object: "event", Name: "eventAdd", ReqType: TypeFirst, Service: "receiver"},
		},
	}
}

// GetDefsByName return definition for all known API calls.
func GetDefsByName() map[string]Def {
	ret := map[string]Def{}
	for path, defs := range GetDefs() {
		for method, def := range defs {
			def.Path = path
			def.Method = method
			ret[def.Name] = def
		}
	}

	return ret
}
