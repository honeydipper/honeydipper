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

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/gin-gonic/gin"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	scas "github.com/qiangmzsx/string-adapter/v2"
)

const (
	// DefaultAPIAckTimeout is the number of milliseconds to wait for acks.
	DefaultAPIAckTimeout time.Duration = 10

	// APIError is the base for all API related error.
	APIError dipper.Error = "API error"

	// DefaultAPIWriteTimeout is the default timeout in seconds for responding to the request.
	DefaultAPIWriteTimeout time.Duration = 10

	// ACLAllow reprensts allowing the subject to access the API.
	ACLAllow = "allow"

	// ACLDeny reprensts denying the subject to access the API.
	ACLDeny = "deny"
)

// Store stores the live API calls in memory.
type Store struct {
	requests        *sync.Map
	requestsByInput *sync.Map
	caller          dipper.RPCCaller
	engine          *gin.Engine
	config          interface{}
	apiDef          map[string]map[string]Def
	newUUID         dipper.UUIDSource
	enforcer        *casbin.Enforcer

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
func NewStore(c dipper.RPCCaller) *Store {
	store := &Store{
		caller:          c,
		requests:        &sync.Map{},
		requestsByInput: &sync.Map{},
	}
	store.apiDef = GetDefs()
	store.newUUID = dipper.NewUUID

	return store
}

// GetAPIHandler prepares and returns the gin Engine for API.
func (l *Store) GetAPIHandler(prefix string, cfg interface{}) http.Handler {
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

	l.setupRoutes(prefix)
	l.setupAuthorization()

	return l.engine
}

// setupAuthorization sets up authorization enforcer.
func (l *Store) setupAuthorization() {
	modelList := dipper.MustGetMapData(l.config, "auth.casbin.models").([]interface{})
	modelText := make([]string, len(modelList))
	for i, line := range modelList {
		modelText[i] = line.(string)
	}
	policyList := dipper.MustGetMapData(l.config, "auth.casbin.policies").([]interface{})
	policyText := make([]string, len(policyList))
	for i, line := range dipper.MustGetMapData(l.config, "auth.casbin.policies").([]interface{}) {
		policyText[i] = line.(string)
	}
	models := model.NewModel()
	dipper.Must(models.LoadModelFromText(strings.Join(modelText, "\n")))
	policies := scas.NewAdapter(strings.Join(policyText, "\n"))
	l.enforcer = dipper.Must(casbin.NewEnforcer(models, policies)).(*casbin.Enforcer)
}

// Enforce checks if the action is allowed based on rules.
func (l *Store) Enforce(args ...interface{}) (bool, error) {
	return l.enforcer.Enforce(args...)
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

	dipper.Logger.Warningf("'%s' , '%s', '%s'", subject, def.Object, def.Method)
	if res, err := l.enforcer.Enforce(subject.(string), def.Object, def.Method); res && err == nil {
		return true
	} else if err != nil {
		dipper.Logger.Warningf("[api] denied access with enforcer error: %+v", err)
	}

	return false
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
func (l *Store) setupRoutes(prefix string) {
	var group *gin.RouterGroup = &l.engine.RouterGroup
	if prefix != "" {
		group = group.Group(prefix)
	}
	for path, defs := range l.apiDef {
		for method, def := range defs {
			def.Method = method
			def.Path = path
			switch method {
			case http.MethodGet:
				group.GET(path, l.CreateHTTPHandlerFunc(def))
			case http.MethodPost:
				group.POST(path, l.CreateHTTPHandlerFunc(def))
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
	path := c.GetPath()
	if def.Method == http.MethodGet {
		if req, ok := l.requestsByInput.Load(path); ok && req != nil {
			return req.(*Request)
		}
	}

	// prepare the parameters
	payload := c.GetPayload(def.Method)

	return &Request{
		store:       l,
		uuid:        l.newUUID(),
		urlPath:     path,
		method:      def.Method,
		fn:          def.Name,
		params:      payload,
		reqType:     def.ReqType,
		service:     def.Service,
		ackTimeout:  l.getAckTimeout(def),
		timeout:     l.getTimeout(def),
		contentType: c.ContentType(),
	}
}

// get the ackTimeout with default value.
func (l *Store) getAckTimeout(d Def) time.Duration {
	if d.AckTimeout != 0 {
		return d.AckTimeout
	}

	timeoutStr, ok := dipper.GetMapDataStr(l.config, "ack_timeout")
	if ok {
		return dipper.Must(time.ParseDuration(timeoutStr)).(time.Duration)
	}

	return DefaultAPIAckTimeout * time.Millisecond
}

// get the timeout with default value.
func (l *Store) getTimeout(d Def) time.Duration {
	if d.Timeout != 0 {
		return d.Timeout
	}

	timeoutStr, ok := dipper.GetMapDataStr(l.config, "timeout")
	if ok {
		return dipper.Must(time.ParseDuration(timeoutStr)).(time.Duration)
	}

	return DefaultAPIWriteTimeout * time.Second
}
