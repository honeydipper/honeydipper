// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/gin-gonic/gin"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	scas "github.com/qiangmzsx/string-adapter/v2"
)

const (
	// DefaultAPIAckTimeout is the number of milliseconds to wait for acks.
	DefaultAPIAckTimeout time.Duration = 10

	// DefaultAPIWriteTimeout is the default timeout in seconds for responding to the request.
	DefaultAPIWriteTimeout time.Duration = 10

	// ACLAllow reprensts allowing the subject to access the API.
	ACLAllow = "allow"

	// ACLDeny reprensts denying the subject to access the API.
	ACLDeny = "deny"

	// RefreshedJWTHeader carries an optionally rotated auth token from provider middleware.
	RefreshedJWTHeader = "X-Honeydipper-Refreshed-JWT"

	// DefaultEntitlementCacheTTL is the duration entitlement results stay in cache.
	DefaultEntitlementCacheTTL = 30 * time.Minute
)

var (
	// ErrAPIError is the base for all API related error.
	ErrAPIError = errors.New("API error")

	// ErrAPINoACK means not able to receive ACK for the API call.
	ErrAPINoACK = fmt.Errorf("%w: no ACK", ErrAPIError)

	// ErrLocalHandlerNotFound means a local API definition is missing its handler.
	ErrLocalHandlerNotFound = fmt.Errorf("%w: local handler not found", ErrAPIError)

	// ErrMissingAuthorizationCode means the GitHub OAuth callback is missing the code parameter.
	ErrMissingAuthorizationCode = fmt.Errorf("%w: authorization code not provided", ErrAPIError)

	// ErrEmptyDriverResponse means the driver returned no content for a local API call.
	ErrEmptyDriverResponse = fmt.Errorf("%w: empty driver response", ErrAPIError)

	// ErrMissingSAMLResponse means the SAML callback did not provide a SAMLResponse.
	ErrMissingSAMLResponse = fmt.Errorf("%w: SAMLResponse not provided", ErrAPIError)
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
	apiPrefix       string
	uiURL           string

	writeTimeout time.Duration
}

// Principal represents the subject making the API call.
type Principal struct {
	Subject     string
	ProfileName string
	Provider    string
	Data        map[string]interface{}
}

// HandleAPIACK handles the call ACK from the eventbus.
func (l *Store) HandleAPIACK(m *dipper.Message) {
	defer dipper.SafeExitOnError("error handling api ack %+v", m.Labels)

	uuid, ok := m.Labels["uuid"]
	if !ok {
		panic(fmt.Errorf("%w: uuid missing ACK", ErrAPIError))
	}
	dipper.Logger.Debugf("handling api ACK for %s", uuid)
	a, ok := l.requests.Load(uuid)
	if !ok {
		panic(fmt.Errorf("%w: request not found", ErrAPIError))
	}
	api := a.(*Request)
	responder, ok := m.Labels["from"]
	if !ok {
		panic(fmt.Errorf("%w: missing from label in ACK", ErrAPIError))
	}

	switch api.reqType {
	case TypeAll:
		api.acks = append(api.acks, responder)
	case TypeMatch:
		api.acks = append(api.acks, responder)
		api.firstACK <- 1
	case TypeFirst:
		panic(fmt.Errorf("%w: TypeFirst APIs do not expect ACKs", ErrAPIError))
	}
}

// HandleAPIReturn handles the call return value from the eventbus.
func (l *Store) HandleAPIReturn(m *dipper.Message) {
	defer dipper.SafeExitOnError("error handling api return %+v", m.Labels)

	m = dipper.DeserializePayload(m)
	uuid, ok := m.Labels["uuid"]
	if !ok {
		panic(fmt.Errorf("%w: uuid missing return", ErrAPIError))
	}
	a, ok := l.requests.Load(uuid)
	if !ok {
		panic(fmt.Errorf("%w: request not found", ErrAPIError))
	}
	api := a.(*Request)
	responder, ok := m.Labels["from"]
	if !ok {
		panic(fmt.Errorf("%w: missing from label in return", ErrAPIError))
	}

	if errmsg, ok := m.Labels["error"]; ok {
		api.err = fmt.Errorf("%w: from [%s]: %s", ErrAPIError, responder, errmsg)
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

func userProfileHandler(r *Request) (map[string]interface{}, error) {
	profileName := ""
	if p, ok := r.ctx.Get("principal"); ok {
		principal := p.(Principal)
		profileName = principal.ProfileName
		if profileName == "" {
			profileName = principal.Subject
		}
	}

	return map[string]interface{}{
		"profile_name": profileName,
	}, nil
}

func githubOAuthCallbackHandler(r *Request) (map[string]interface{}, error) {
	code := r.ctx.GetParam("code")
	if code == "" {
		return nil, ErrMissingAuthorizationCode
	}

	answer, err := r.store.caller.Call("driver:auth-github", "github_oauth_callback", map[string]interface{}{
		"code": code,
	})
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	if answer == nil {
		return nil, ErrEmptyDriverResponse
	}

	result := map[string]interface{}{}
	if err := json.Unmarshal(answer, &result); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return result, nil
}

func samlLoginHandler(r *Request) (map[string]interface{}, error) {
	payload := map[string]interface{}{}
	if relayState := r.ctx.GetParam("relay_state"); relayState != "" {
		payload["relay_state"] = relayState
	}

	answer, err := r.store.caller.Call("driver:auth-saml", "saml_login", payload)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	if answer == nil {
		return nil, ErrEmptyDriverResponse
	}

	result := map[string]interface{}{}
	if err := json.Unmarshal(answer, &result); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	redirectURL, ok := result["redirect_url"].(string)
	if !ok || redirectURL == "" {
		return nil, ErrEmptyDriverResponse
	}

	return map[string]interface{}{"_redirect": redirectURL}, nil
}

func samlSPMetadataHandler(r *Request) (map[string]interface{}, error) {
	answer, err := r.store.caller.Call("driver:auth-saml", "saml_sp_metadata", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	if answer == nil {
		return nil, ErrEmptyDriverResponse
	}

	result := map[string]interface{}{}
	if err := json.Unmarshal(answer, &result); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	metadata, ok := result["metadata"].(string)
	if !ok || strings.TrimSpace(metadata) == "" {
		return nil, ErrEmptyDriverResponse
	}

	return map[string]interface{}{
		"_raw_body":     metadata,
		"_content_type": "application/samlmetadata+xml; charset=utf-8",
	}, nil
}

func samlACSCallbackHandler(r *Request) (map[string]interface{}, error) {
	payload := r.ctx.GetPayload(http.MethodPost)
	if payload["SAMLResponse"] == nil {
		if response := r.ctx.GetParam("SAMLResponse"); response != "" {
			payload["SAMLResponse"] = response
		}
	}
	if payload["RelayState"] == nil {
		if relayState := r.ctx.GetParam("RelayState"); relayState != "" {
			payload["RelayState"] = relayState
		}
	}
	if payload["SAMLResponse"] == nil {
		return nil, ErrMissingSAMLResponse
	}

	answer, err := r.store.caller.Call("driver:auth-saml", "saml_acs", payload)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	if answer == nil {
		return nil, ErrEmptyDriverResponse
	}

	result := map[string]interface{}{}
	if err := json.Unmarshal(answer, &result); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	subject, _ := result["subject"].(string)
	profileName, _ := result["profile_name"].(string)
	data, _ := result["data"].(map[string]interface{})

	query := url.Values{}

	// Prefer a Honeydipper JWT so subsequent requests bypass driver RPCs entirely.
	// Fall back to the driver's own session token for backwards compatibility.
	tokenSet := false
	if subject != "" {
		if jwtCfg, err := getJWTConfig(); err == nil {
			principal := Principal{Subject: subject, ProfileName: profileName, Provider: "auth-saml", Data: data}
			if hdToken, err := SignPrincipalJWT(principal, jwtCfg); err == nil {
				query.Set("token", hdToken)
				tokenSet = true
			}
		}
	}
	if !tokenSet {
		if token, ok := result["token"].(string); ok {
			query.Set("token", token)
		}
	}

	if subject != "" {
		query.Set("subject", subject)
	}
	if profileName != "" {
		query.Set("profile_name", profileName)
	}

	uiBase := strings.TrimRight(r.store.uiURL, "/")

	return map[string]interface{}{"_redirect": uiBase + "/auth/saml/callback?" + query.Encode()}, nil
}

// GetAPIHandler prepares and returns the gin Engine for API.
func (l *Store) GetAPIHandler(prefix string, cfg interface{}) http.Handler {
	gin.DefaultWriter = dipper.LoggingWriter
	l.config = cfg
	l.apiPrefix = prefix
	l.engine = gin.New()
	l.engine.Use(gin.Logger())
	l.engine.Use(gin.Recovery())
	l.engine.Use(l.AuthMiddleware())

	l.writeTimeout = DefaultAPIWriteTimeout * time.Second
	if writeTimeoutStr, ok := dipper.GetMapDataStr(l.config, "writeTimeout"); ok {
		l.writeTimeout = dipper.Must(time.ParseDuration(writeTimeoutStr)).(time.Duration)
	}

	l.uiURL = os.Getenv("HD_UI_URL")
	if uiURLStr, ok := dipper.GetMapDataStr(l.config, "ui_url"); ok && uiURLStr != "" {
		l.uiURL = uiURLStr
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
	ef, e := l.enforcer.Enforce(args...)
	if e != nil {
		return ef, fmt.Errorf("auth middleware error: %w", e)
	}

	return ef, nil
}

// AuthMiddleware is a middleware handles auth.
func (l *Store) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if l.isAnonymousRoute(c) {
			c.Set("principal", Principal{Subject: "guest", ProfileName: "Guest", Provider: "none"})
			c.Next()

			return
		}

		// 1. Try JWT from Authorization header
		jwtCfg, jwtErr := getJWTConfig()
		// var jwtPrincipal *Principal
		var jwtToken string
		authHeader := c.GetHeader("Authorization")
		if jwtErr == nil {
			jwtToken = ExtractJWTFromRequest(authHeader)
			if jwtToken != "" {
				if p, err := ParsePrincipalJWT(jwtToken, jwtCfg); err == nil && p != nil {
					c.Set("principal", *p)
					c.Next()

					return
				}
			}
		}

		// 2. Fallback to drivers
		providers, ok := dipper.GetMapData(l.config, "auth-providers")
		principal := Principal{
			Subject:     "guest",
			ProfileName: "Guest",
			Provider:    "none",
		}
		if !ok || providers == nil || len(providers.([]interface{})) == 0 {
			c.Set("principal", principal)
			c.Next()

			return
		}

		allErrors := map[string]string{}
		for _, providerEntry := range providers.([]interface{}) {
			providerName := providerEntry.(string)
			parts := strings.Split(providerName, ".")
			provider := parts[0]
			fn := "auth_web_request"
			if len(parts) > 1 {
				fn = parts[1]
			}

			answer, err := l.caller.Call("driver:"+provider, fn, dipper.ExtractWebRequestExceptBody(c.Request))
			if err != nil {
				allErrors[providerName] = err.Error()

				continue
			}
			if answer == nil {
				allErrors[providerName] = "empty auth response"

				continue
			}

			principal = Principal{}
			if err := json.Unmarshal(answer, &principal); err != nil {
				allErrors[providerName] = err.Error()

				continue
			} else {
				principal.Provider = provider
				l.applyRotatedJWTHeader(c, principal)
				c.Set("principal", principal)
				// Issue a new JWT for the principal if possible
				if jwtErr == nil {
					if token, err := SignPrincipalJWT(principal, jwtCfg); err == nil {
						c.Header("X-Honeydipper-JWT", token)
						// Optionally, set as cookie here if desired
					}
				}
				c.Next()

				return
			}
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]interface{}{"errors": allErrors})
	}
}

func (l *Store) applyRotatedJWTHeader(c *gin.Context, principal Principal) {
	if principal.Data == nil {
		return
	}

	rotatedJWT, ok := principal.Data["rotatedJwt"].(string)
	if !ok || rotatedJWT == "" {
		return
	}

	c.Header(RefreshedJWTHeader, rotatedJWT)
	exposed := c.Writer.Header().Get("Access-Control-Expose-Headers")
	if exposed == "" {
		c.Header("Access-Control-Expose-Headers", RefreshedJWTHeader)

		return
	}

	for _, part := range strings.Split(exposed, ",") {
		if strings.EqualFold(strings.TrimSpace(part), RefreshedJWTHeader) {
			return
		}
	}

	c.Header("Access-Control-Expose-Headers", exposed+", "+RefreshedJWTHeader)
}

func (l *Store) isAnonymousRoute(c *gin.Context) bool {
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}

	if l.apiPrefix != "" && strings.HasPrefix(path, l.apiPrefix) {
		path = strings.TrimPrefix(path, l.apiPrefix)
	}

	defs, ok := l.apiDef[path]
	if !ok {
		return false
	}

	def, ok := defs[c.Request.Method]
	if !ok {
		return false
	}

	return def.AllowAnonymous
}

// CheckEntitlement resolves derived subjects from an external entitlement provider.
func (l *Store) CheckEntitlement(c RequestContext, def Def) bool {
	p, ok := c.Get("principal")
	if !ok {
		return false
	}
	principal := p.(Principal)

	provider := def.EntitlementProvider
	entitlementTarget := c.GetParam(def.EntitlementKey)
	cacheKey := l.entitlementCacheKey(provider, principal.Subject, entitlementTarget)

	if cacheAnswer, err := l.caller.Call("cache", "load", map[string]any{"key": cacheKey}); err == nil {
		if derivedSubjects, ok := parseDerivedSubjects(cacheAnswer, provider); ok {
			c.Set("derivedSubjects", derivedSubjects)

			return true
		}
	} else {
		dipper.Logger.Warningf("[api] failed to load entitlement cache for %s: %v", cacheKey, err)
	}

	answer, err := l.caller.Call("driver:"+provider, "check_entitlements", map[string]interface{}{
		"principal":         principal,
		"entitlementTarget": entitlementTarget,
	})
	if err != nil {
		return false
	}

	derivedSubjects, ok := parseDerivedSubjects(answer, provider)
	if !ok {
		return false
	}

	_, err = l.caller.Call("cache", "save", map[string]any{
		"key":   cacheKey,
		"value": string(bytes.TrimSpace(answer)),
		"ttl":   l.entitlementCacheTTL().String(),
	})
	if err != nil {
		dipper.Logger.Warningf("[api] failed to save entitlement cache for %s: %v", cacheKey, err)
	}
	c.Set("derivedSubjects", derivedSubjects)

	return true
}

func (l *Store) entitlementCacheTTL() time.Duration {
	ttl := DefaultEntitlementCacheTTL

	if ttlStr, ok := dipper.GetMapDataStr(l.config, "auth.entitlementCacheTTL"); ok && ttlStr != "" {
		if parsed, err := time.ParseDuration(ttlStr); err == nil && parsed > 0 {
			ttl = parsed
		}
	}

	return ttl
}

func (l *Store) entitlementCacheKey(provider, subject, entitlementTarget string) string {
	return fmt.Sprintf("api-entitlements/%s/%s/%s", provider, url.QueryEscape(subject), url.QueryEscape(entitlementTarget))
}

func parseDerivedSubjects(answer []byte, provider string) ([]string, bool) {
	if answer == nil {
		return nil, false
	}

	answer = bytes.TrimSpace(answer)
	if len(answer) == 0 {
		return nil, false
	}

	var derivedSubjects []string
	if err := json.Unmarshal(answer, &derivedSubjects); err != nil {
		var single string
		if err := json.Unmarshal(answer, &single); err != nil {
			dipper.Logger.Warningf("[api] invalid check_entitlements response from %s: %v", provider, err)

			return nil, false
		}

		single = strings.TrimSpace(single)
		if single == "" {
			return nil, false
		}
		derivedSubjects = []string{single}
	}

	if len(derivedSubjects) == 0 {
		return nil, false
	}

	return derivedSubjects, true
}

// UsesPrincipalInRequest returns true when casbin request_definition includes a principal object field.
func (l *Store) UsesPrincipalInRequest() bool {
	requestTokens, ok := l.enforcer.GetModel()["r"]["r"]

	return ok && len(requestTokens.Tokens) >= 5
}

func (l *Store) buildEnforceArgs(subject interface{}, principal Principal, object, method, provider string) []interface{} {
	if l.UsesPrincipalInRequest() {
		return []interface{}{subject, principal, object, method, provider}
	}

	return []interface{}{subject, object, method, provider}
}

func (l *Store) enforceWithPrincipal(subject interface{}, principal Principal, object, method, provider string) (bool, error) {
	return l.Enforce(l.buildEnforceArgs(subject, principal, object, method, provider)...)
}

// Authorize determines if a subject is allowed to call a API.
func (l *Store) Authorize(c RequestContext, def Def) bool {
	p, ok := c.Get("principal")
	if !ok {
		return false
	}
	principal := p.(Principal)

	subject := principal.Subject
	provider := principal.Provider
	if res, err := l.enforceWithPrincipal(subject, principal, def.Object, def.Method, provider); res && err == nil {
		return true
	} else if err != nil {
		dipper.Logger.Warningf("[api] denied access with enforcer error: %+v", err)
	}

	if def.EntitlementProvider != "" {
		if !l.CheckEntitlement(c, def) {
			return false
		}

		derivedSubjects, ok := c.Get("derivedSubjects")
		if !ok {
			return false
		}

		for _, subject := range derivedSubjects.([]string) {
			entitledPrincipal := Principal{Subject: subject, Provider: def.EntitlementProvider}
			if res, err := l.enforceWithPrincipal(subject, entitledPrincipal, def.Object, def.Method, def.EntitlementProvider); res && err == nil {
				return true
			}
		}

		return false
	}

	return false
}

// HandleHTTPRequest handles http requests.
func (l *Store) HandleHTTPRequest(c RequestContext, def Def) {
	if !def.AllowAnonymous && !l.Authorize(c, def) {
		c.AbortWithStatusJSON(http.StatusForbidden, map[string]interface{}{"errors": "not allowed"})

		return
	}

	// create or find the original request
	r := l.GetRequest(def, c)
	r.Dispatch()

	writeTimer := time.NewTimer(l.writeTimeout - time.Millisecond)
	defer writeTimer.Stop()

	// wait for the results
	select {
	case <-r.ready:
		//nolint:nestif // so what if it is nested, it's more readable this way
		if r.err != nil {
			if errors.Is(r.err, ErrAPINoACK) {
				c.AbortWithStatusJSON(http.StatusNotFound, map[string]interface{}{"error": "object not found"})
			} else {
				c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]interface{}{"error": r.err.Error()})
			}
		} else if redirectURL, ok := r.getResults()["_redirect"]; ok {
			c.Redirect(http.StatusFound, redirectURL.(string))
		} else if rawBody, ok := r.getResults()["_raw_body"]; ok {
			contentType, _ := r.getResults()["_content_type"].(string)
			if contentType == "" {
				contentType = "text/plain; charset=utf-8"
			}
			switch typed := rawBody.(type) {
			case string:
				c.Data(http.StatusOK, contentType, []byte(typed))
			case []byte:
				c.Data(http.StatusOK, contentType, typed)
			default:
				c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]interface{}{"error": ErrAPIError.Error()})
			}
		} else {
			c.IndentedJSON(http.StatusOK, r.getResults())
		}
	case <-writeTimer.C:
		if r.method == http.MethodGet && (r.timeout == InfiniteDuration || r.timeout > l.writeTimeout) {
			c.IndentedJSON(http.StatusAccepted, map[string]interface{}{"uuid": r.uuid, "results": r.getResults()})
		} else {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]interface{}{"error": dipper.ErrTimeout.Error()})
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
	group := &l.engine.RouterGroup
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
			case http.MethodDelete:
				group.DELETE(path, l.CreateHTTPHandlerFunc(def))
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
		store:               l,
		ctx:                 c,
		uuid:                l.newUUID(),
		urlPath:             path,
		method:              def.Method,
		local:               def.Local,
		fn:                  def.Name,
		params:              payload,
		attachPrincipalUser: def.AttachPrincipalUser,
		reqType:             def.ReqType,
		service:             def.Service,
		ackTimeout:          l.getAckTimeout(def),
		timeout:             l.getTimeout(def),
		contentType:         c.ContentType(),
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

	return l.writeTimeout
}
