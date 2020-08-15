// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

const (
	// DefaultAPIAckTimeout is the number of milliseconds to wait for acks
	DefaultAPIAckTimeout time.Duration = 10

	// APIError is the base for all API related error
	APIError dipper.Error = "API error"

	// DefaultAPIWriteTimeout is the default timeout in seconds for responding to the request
	DefaultAPIWriteTimeout time.Duration = 10

	// ACLAllow reprensts allowing the subject to access the API
	ACLAllow = "allow"
)

// Store stores the live API calls in memory.
type Store struct {
	requests        *sync.Map
	requestsByInput *sync.Map
	caller          *dipper.RPCCaller
	engine          *gin.Engine
	config          interface{}
	apiDef          map[string]map[string]Def

	writeTimeout time.Duration
}

// HandleAPIACK handles the call ACK from the eventbus.
func (l *Store) HandleAPIACK(m *dipper.Message) {
	defer dipper.SafeExitOnError("error handling api ack %+v", m.Labels)

	uuid, ok := m.Labels["uuid"]
	if !ok {
		panic(fmt.Errorf("uuid missing in ack: %w", APIError))
	}
	dipper.Logger.Debugf("handling api ack for %s", uuid)
	a, ok := l.requests.Load(uuid)
	if !ok {
		panic(fmt.Errorf("request not found: %w", APIError))
	}
	api := a.(*Request)
	responder, ok := m.Labels["from"]
	if !ok {
		panic(fmt.Errorf("from label missing in ack: %w", APIError))
	}

	switch api.reqType {
	case TypeAll:
		api.acks = append(api.acks, responder)
	case TypeMatch:
		api.acks = append(api.acks, responder)
		api.firstACK <- 1
	case TypeFirst:
		panic(fmt.Errorf("TypeFirst APIs do not expect acks: %w", APIError))
	}
}

// HandleAPIReturn handles the call return value from the eventbus.
func (l *Store) HandleAPIReturn(m *dipper.Message) {
	defer dipper.SafeExitOnError("error handling api return %+v", m.Labels)

	m = dipper.DeserializePayload(m)
	uuid, ok := m.Labels["uuid"]
	if !ok {
		panic(fmt.Errorf("uuid missing in ack: %w", APIError))
	}
	a, ok := l.requests.Load(uuid)
	if !ok {
		panic(fmt.Errorf("request not found: %w", APIError))
	}
	api := a.(*Request)
	responder, ok := m.Labels["from"]
	if !ok {
		panic(fmt.Errorf("from label missing in ack: %w", APIError))
	}

	if errmsg, ok := m.Labels["error"]; ok {
		api.err = fmt.Errorf("%w: from [%s]: %s", APIError, responder, errmsg)
		api.received <- 1
		return
	}

	if api.reqType == TypeFirst {
		api.results[responder] = m.Payload
		api.received <- 1
		return
	}

	api.results[responder] = m.Payload
	if api.received != nil && len(api.results) == len(api.acks) {
		api.received <- 1
	}
}

// NewStore creates a new Store.
func NewStore(c *dipper.RPCCaller) *Store {
	store := &Store{
		caller:          c,
		requests:        &sync.Map{},
		requestsByInput: &sync.Map{},
	}
	store.apiDef = GetDefs()
	return store
}

// PrepareHTTPServer prepares the provided http server.
func (l *Store) PrepareHTTPServer(s *http.Server, cfg interface{}) {
	gin.DefaultWriter = dipper.LoggingWriter
	l.config = cfg
	l.engine = gin.New()
	l.engine.Use(gin.Logger())
	l.engine.Use(gin.Recovery())
	l.engine.Use(l.AuthMiddleware())

	l.writeTimeout = DefaultAPIWriteTimeout * time.Second
	if writeTimeoutStr, ok := dipper.GetMapDataStr(l.config, "writeTimeout"); ok {
		l.writeTimeout = dipper.Must(time.ParseDuration(writeTimeoutStr)).(time.Duration)
	}

	l.setupRoutes()
	s.Handler = l.engine
}

// AuthMiddleware is a middleware handles auth.
func (l *Store) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		providers, ok := dipper.GetMapData(l.config, "auth-providers")
		if !ok || providers == nil || len(providers.([]interface{})) == 0 {
			c.Next()
			return
		}

		allErrors := map[string]string{}
		for _, p := range providers.([]interface{}) {
			parts := strings.Split(p.(string), ".")
			provider := parts[0]
			fn := "auth_web_request"
			if len(parts) > 1 {
				fn = parts[1]
			}

			subject, err := l.caller.Call("driver:"+provider, fn, dipper.ExtractWebRequestExceptBody(c.Request))
			if err != nil || subject == nil {
				allErrors[p.(string)] = err.Error()
			} else {
				c.Set("subject", dipper.DeserializeContent(subject))
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]interface{}{"errors": allErrors})
	}
}

// Authorize determines if a subject is allowed to call a API.
func (l *Store) Authorize(c RequestContext, def Def) bool {
	subject, ok := c.Get("subject")
	if !ok {
		return false
	}

	rulesObj, ok := dipper.GetMapData(l.config, "acls."+def.name)
	if !ok {
		return false
	}

	rules, ok := rulesObj.([]interface{})
	if !ok {
		return false
	}

	defaultAction := ACLAllow
	for _, ruleObj := range rules {
		rule := ruleObj.(map[string]interface{})
		switch v := rule["subjects"].(type) {
		case string:
			defaultAction = v
		case []interface{}:
			if dipper.CompareAll(subject, v) {
				return rule["type"] == ACLAllow
			}
		default:
			return false
		}
	}

	return defaultAction == ACLAllow
}

// HandleHTTPRequest handles http requests.
func (l *Store) HandleHTTPRequest(c RequestContext, def Def) {
	if !l.Authorize(c, def) {
		c.AbortWithStatusJSON(http.StatusForbidden, map[string]interface{}{"errors": "not allowed"})
		return
	}

	// create or find the original request
	r := l.GetRequest(def, c)
	r.Dispatch()

	// wait for the results
	select {
	case <-r.ready:
		if r.err != nil {
			if errors.Is(r.err, APIErrorNoAck) {
				c.AbortWithStatusJSON(http.StatusNotFound, map[string]interface{}{"error": "object not found"})
			} else {
				c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]interface{}{"error": r.err.Error()})
			}
		} else {
			c.IndentedJSON(http.StatusOK, r.getResults())
		}
	case <-time.After(l.writeTimeout - time.Millisecond):
		if r.method == http.MethodGet && (r.timeout == InfiniteDuration || r.timeout > l.writeTimeout) {
			c.IndentedJSON(http.StatusAccepted, map[string]interface{}{"uuid": r.uuid, "results": r.getResults()})
		} else {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]interface{}{"error": dipper.TimeoutError.Error()})
		}
	}
}

// CreateHTTPHandlerFunc return a handler function for GET method.
func (l *Store) CreateHTTPHandlerFunc(def Def) gin.HandlerFunc {
	// create and return the function
	return func(c *gin.Context) {
		l.HandleHTTPRequest(&GinRequestContext{gin: c}, def)
	}
}

// setupRoutes sets up the routes.
func (l *Store) setupRoutes() {
	for path, defs := range l.apiDef {
		for method, def := range defs {
			def.method = method
			def.path = path
			switch method {
			case http.MethodGet:
				l.engine.GET(path, l.CreateHTTPHandlerFunc(def))
			case http.MethodPost:
				l.engine.POST(path, l.CreateHTTPHandlerFunc(def))
			}
		}
	}
}

// ClearRequest removes the API requests from memory.
func (l *Store) ClearRequest(r *Request) {
	l.requests.Delete(r.uuid)
	if r.method == http.MethodGet && (r.timeout == InfiniteDuration || r.timeout > l.writeTimeout) {
		l.requestsByInput.Delete(r.urlPath)
	}
}

// SaveRequest saves the request into maps for future references.
func (l *Store) SaveRequest(r *Request) {
	l.requests.Store(r.uuid, r)
	if r.method == http.MethodGet && (r.timeout == InfiniteDuration || r.timeout > l.writeTimeout) {
		l.requestsByInput.Store(r.urlPath, r)
	}
}

// GetRequest creates a new Request with the given definition and parameters or return an existing one based on uuid.
func (l *Store) GetRequest(def Def, c RequestContext) *Request {
	if def.method == http.MethodGet {
		if req, ok := l.requestsByInput.Load(c.GetPath()); ok && req != nil {
			return req.(*Request)
		}
	}

	// prepare the parameters
	payload := c.GetPayload(def.method)

	return &Request{
		store:       l,
		uuid:        dipper.Must(uuid.NewRandom()).(uuid.UUID).String(),
		urlPath:     c.GetPath(),
		method:      def.method,
		fn:          def.name,
		params:      payload,
		reqType:     def.reqType,
		service:     def.service,
		ackTimeout:  l.getAckTimeout(def),
		timeout:     l.getTimeout(def),
		contentType: c.ContentType(),
	}
}

// get the ackTimeout with default value.
func (l *Store) getAckTimeout(d Def) time.Duration {
	if d.ackTimeout != 0 {
		return d.ackTimeout
	}

	timeoutStr, ok := dipper.GetMapDataStr(l.config, "ack_timeout")
	if ok {
		return dipper.Must(time.ParseDuration(timeoutStr)).(time.Duration)
	}

	return DefaultAPIAckTimeout * time.Millisecond
}

// get the timeout with default value.
func (l *Store) getTimeout(d Def) time.Duration {
	if d.timeout != 0 {
		return d.timeout
	}

	timeoutStr, ok := dipper.GetMapDataStr(l.config, "timeout")
	if ok {
		return dipper.Must(time.ParseDuration(timeoutStr)).(time.Duration)
	}

	return DefaultAPIWriteTimeout * time.Second
}
