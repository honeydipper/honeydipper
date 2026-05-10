// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"net/http"
	"time"
)

// LocalHandlerFunc handles local API requests.
type LocalHandlerFunc func(*Request) (map[string]interface{}, error)

// Def is a structure defines how an API should be handled in api service.
type Def struct {
	Path                string
	Object              string
	Name                string
	Method              string
	AttachPrincipalUser bool
	ReqType             int
	Local               LocalHandlerFunc
	Service             string
	AckTimeout          time.Duration
	Timeout             time.Duration
	AllowAnonymous      bool
	EntitlementProvider string
	EntitlementKey      string
}

const (
	// TypeFirst means the API uses first return from whichever node responds.
	TypeFirst = iota
	// TypeAll means returns from all nodes are used.
	TypeAll
	// TypeMatch means the API allows the node who has the matching record to respond.
	TypeMatch
	// TypeLocal means the API is served by a local handler in api package.
	TypeLocal

	// InfiniteDuration is used to specify a timeout of infinity duration.
	InfiniteDuration time.Duration = -1
)

// GetDefs return definition for all known API calls.
func GetDefs() map[string]map[string]Def {
	return map[string]map[string]Def{
		"auth/saml/login": {
			http.MethodGet: {
				Object: "everything", Name: "samlLogin", ReqType: TypeLocal,
				Local: samlLoginHandler, Service: "api", AllowAnonymous: true,
			},
		},
		"auth/saml/metadata": {
			http.MethodGet: {
				Object: "everything", Name: "samlSPMetadata", ReqType: TypeLocal,
				Local: samlSPMetadataHandler, Service: "api", AllowAnonymous: true,
			},
		},
		"auth/saml/callback": {
			http.MethodPost: {
				Object: "everything", Name: "samlACSCallback", ReqType: TypeLocal,
				Local: samlACSCallbackHandler, Service: "api", AllowAnonymous: true,
			},
			http.MethodGet: {
				Object: "everything", Name: "samlACSCallbackGet", ReqType: TypeLocal,
				Local: samlACSCallbackHandler, Service: "api", AllowAnonymous: true,
			},
		},
		"auth/github/callback": {
			http.MethodGet: {
				Object: "everything", Name: "githubOAuthCallback", ReqType: TypeLocal,
				Local: githubOAuthCallbackHandler, Service: "api", AllowAnonymous: true,
			},
		},
		"user/profile": {
			http.MethodGet: {Object: "user/profile", Name: "userProfile", ReqType: TypeLocal, Local: userProfileHandler, Service: "api"},
		},
		"events/:eventID/wait": {
			http.MethodGet: {Object: "event", Name: "eventWait", ReqType: TypeFirst, Service: "engine", Timeout: InfiniteDuration},
		},
		"events/:sessionID/rerun": {
			http.MethodPost: {Object: "event", Name: "eventRerun", ReqType: TypeFirst, Service: "engine"},
		},
		"events/:sessionID/pause": {
			http.MethodPost: {Object: "event", Name: "eventPause", ReqType: TypeFirst, Service: "engine"},
		},
		"events/:sessionID/resume": {
			http.MethodPost: {Object: "event", Name: "eventResume", ReqType: TypeFirst, Service: "engine"},
		},
		"events/:sessionID/interact": {
			http.MethodPost: {Object: "event", Name: "eventInteract", AttachPrincipalUser: true, ReqType: TypeFirst, Service: "engine"},
		},
		"events/:sessionID/cancel": {
			http.MethodPost: {Object: "event", Name: "eventCancel", ReqType: TypeFirst, Service: "engine"},
		},
		"events": {
			http.MethodGet:  {Object: "event", Name: "eventList", ReqType: TypeFirst, Service: "engine"},
			http.MethodPost: {Object: "event", Name: "eventAdd", ReqType: TypeFirst, Service: "receiver"},
		},
		"gh/events/*gh_slug": {
			http.MethodGet: {
				Object: "gh_event", Name: "ghEventList", ReqType: TypeFirst, Service: "engine",
				EntitlementProvider: "auth-github",
				EntitlementKey:      "gh_slug",
			},
		},
		"pods/:pod_id/log/chunk": {
			http.MethodGet: {
				Object: "pod_log", Name: "podLogChunk", ReqType: TypeFirst, Service: "operator",
			},
		},
		"gh/pods/:pod_id/log/chunk/*gh_slug": {
			http.MethodGet: {
				Object: "gh_event", Name: "ghPodLogChunk", ReqType: TypeFirst, Service: "operator",
				EntitlementProvider: "auth-github",
				EntitlementKey:      "gh_slug",
			},
		},
		"gh/secrets/*gh_slug": {
			http.MethodGet: {
				Object: "gh_secret", Name: "ghSecretList", ReqType: TypeFirst, Service: "operator",
				EntitlementProvider: "auth-github",
				EntitlementKey:      "gh_slug",
			},
			http.MethodPost: {
				Object: "gh_secret", Name: "ghSecretSet", ReqType: TypeFirst, Service: "operator",
				EntitlementProvider: "auth-github",
				EntitlementKey:      "gh_slug",
			},
			http.MethodDelete: {
				Object: "gh_secret", Name: "ghSecretDelete", ReqType: TypeFirst, Service: "operator",
				EntitlementProvider: "auth-github",
				EntitlementKey:      "gh_slug",
			},
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
