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

// agentDefs returns API route definitions served by the agent service.
func agentDefs() map[string]map[string]Def {
	return map[string]map[string]Def{
		"convos": {
			http.MethodGet:  {Object: "convo", Name: "convoList", ReqType: TypeFirst, Service: "agent"},
			http.MethodPost: {Object: "convo", Name: "convoNew", AttachPrincipalUser: true, ReqType: TypeFirst, Service: "agent"},
		},
		"convos/:convoID": {
			http.MethodGet: {Object: "convo", Name: "convoState", ReqType: TypeFirst, Service: "agent"},
		},
		"agents": {
			http.MethodGet: {Object: "agent", Name: "agentList", ReqType: TypeFirst, Service: "agent"},
		},
		"engines": {
			http.MethodGet: {Object: "engine", Name: "agentEngines", ReqType: TypeFirst, Service: "agent"},
		},
		"convos/:convoID/history": {
			http.MethodGet: {Object: "convo", Name: "convoHistory", ReqType: TypeFirst, Service: "agent"},
		},
		"convos/:convoID/cancel": {
			http.MethodPost: {Object: "convo", Name: "convoCancel", ReqType: TypeFirst, Service: "agent"},
		},
		"convos/:convoID/turn": {
			http.MethodPost: {Object: "convo", Name: "convoTurn", AttachPrincipalUser: true, ReqType: TypeFirst, Service: "agent"},
		},
	}
}

// ghEventDefs returns API route definitions for GitHub-scoped event operations.
func ghEventDefs() map[string]map[string]Def {
	return map[string]map[string]Def{
		"gh/events/*gh_slug": {
			http.MethodGet: {
				Object: "gh_event", Name: "ghEventList", ReqType: TypeFirst, Service: "engine",
				EntitlementProvider: "auth-github", EntitlementKey: "gh_slug",
			},
		},
		"gh/events/:sessionID/rerun/*gh_slug": {
			http.MethodPost: {
				Object: "gh_event", Name: "ghEventRerun", ReqType: TypeFirst, Service: "engine",
				EntitlementProvider: "auth-github", EntitlementKey: "gh_slug",
			},
		},
		"gh/events/:sessionID/pause/*gh_slug": {
			http.MethodPost: {
				Object: "gh_event", Name: "ghEventPause", ReqType: TypeFirst, Service: "engine",
				EntitlementProvider: "auth-github", EntitlementKey: "gh_slug",
			},
		},
		"gh/events/:sessionID/resume/*gh_slug": {
			http.MethodPost: {
				Object: "gh_event", Name: "ghEventResume", ReqType: TypeFirst, Service: "engine",
				EntitlementProvider: "auth-github", EntitlementKey: "gh_slug",
			},
		},
		"gh/events/:sessionID/interact/*gh_slug": {
			http.MethodPost: {
				Object: "gh_event", Name: "ghEventInteract", AttachPrincipalUser: true, ReqType: TypeFirst, Service: "engine",
				EntitlementProvider: "auth-github", EntitlementKey: "gh_slug",
			},
		},
	}
}

// GetDefs return definition for all known API calls.
func GetDefs() map[string]map[string]Def {
	defs := map[string]map[string]Def{
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
		"auth/logout": {
			http.MethodPost: {Object: "auth/logout", Name: "authLogout", ReqType: TypeLocal, Local: logoutHandler, Service: "api"},
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
	for path, methods := range ghEventDefs() {
		defs[path] = methods
	}
	for path, methods := range agentDefs() {
		defs[path] = methods
	}

	return defs
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
