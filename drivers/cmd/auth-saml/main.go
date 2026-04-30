// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package auth-saml enables Honeydipper to authenticate web requests using SAML as an SP.
package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/golang-jwt/jwt/v5"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	dsig "github.com/russellhaering/goxmldsig"
)

const (
	defaultTokenExpiration = 24 * time.Hour
	defaultRequestTTL      = 10 * time.Minute
)

var (
	ErrInvalidBearerToken = errors.New("invalid bearer token")
	ErrMissingSAMLConfig  = errors.New("missing SAML driver config")
	ErrMissingSAMLReply   = errors.New("missing SAMLResponse")
	ErrUnknownRelayState  = errors.New("unknown relay state")
	ErrInvalidSessionJWT  = errors.New("invalid session token")
	ErrMissingSubject     = errors.New("missing subject in SAML assertion")
	ErrInvalidPayloadType = errors.New("invalid payload type")
)

type authSAMLDriver struct {
	*dipper.Driver

	sp              *saml.ServiceProvider
	jwtSigningKey   []byte
	tokenExpiration time.Duration
	requestTTL      time.Duration
	allowIDPInit    bool

	requestByRelay sync.Map
}

type samlRequestState struct {
	requestID string
	expiresAt time.Time
}

var driver = &authSAMLDriver{}

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports API service.\n")
		fmt.Printf("  This program provides honeydipper with SAML SP authentication support.\n")
	}
}

func main() {
	initFlags()
	flag.Parse()

	driver.Driver = dipper.NewDriver(os.Args[1], "auth-saml")
	driver.RPCHandlers["auth_web_request"] = driver.authWebRequest
	driver.RPCHandlers["saml_login"] = driver.samlLogin
	driver.RPCHandlers["saml_acs"] = driver.samlACS
	driver.Reload = driver.setupConfig
	driver.Start = driver.setupConfig
	driver.Run()
}

func (d *authSAMLDriver) setupConfig(_ *dipper.Message) {
	if err := d.initConfig(); err != nil {
		d.GetLogger().Panicf("failed to initialize auth-saml driver: %v", err)
	}
}

func (d *authSAMLDriver) initConfig() error {
	acsURLStr, ok := dipper.GetMapDataStr(d.Options, "data.acs_url")
	if !ok || acsURLStr == "" {
		return fmt.Errorf("%w: data.acs_url", ErrMissingSAMLConfig)
	}
	acsURL, err := url.Parse(acsURLStr)
	if err != nil {
		return fmt.Errorf("invalid data.acs_url: %w", err)
	}

	metadataURLStr, ok := dipper.GetMapDataStr(d.Options, "data.idp_metadata_url")
	if !ok || metadataURLStr == "" {
		return fmt.Errorf("%w: data.idp_metadata_url", ErrMissingSAMLConfig)
	}
	metadataURL, err := url.Parse(metadataURLStr)
	if err != nil {
		return fmt.Errorf("invalid data.idp_metadata_url: %w", err)
	}

	jwtSigningKey, ok := dipper.GetMapDataStr(d.Options, "data.jwt_signing_key")
	if !ok || strings.TrimSpace(jwtSigningKey) == "" {
		jwtSigningKey = os.Getenv("AUTH_SAML_JWT_SIGNING_KEY")
	}
	if strings.TrimSpace(jwtSigningKey) == "" {
		return fmt.Errorf("%w: data.jwt_signing_key or AUTH_SAML_JWT_SIGNING_KEY", ErrMissingSAMLConfig)
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	idpMetadata, err := samlsp.FetchMetadata(context.Background(), httpClient, *metadataURL)
	if err != nil {
		return fmt.Errorf("failed to fetch idp metadata: %w", err)
	}

	entityID := acsURL.String()
	if configuredEntityID, ok := dipper.GetMapDataStr(d.Options, "data.entity_id"); ok && configuredEntityID != "" {
		entityID = configuredEntityID
	}

	d.sp = &saml.ServiceProvider{
		EntityID:          entityID,
		AcsURL:            *acsURL,
		MetadataURL:       *acsURL,
		IDPMetadata:       idpMetadata,
		AllowIDPInitiated: false,
		AuthnNameIDFormat: saml.EmailAddressNameIDFormat,
	}
	if spKey, ok := dipper.GetMapDataStr(d.Options, "data.sp_key"); ok && spKey != "" {
		signer, cert, err := parseSPKeyPair(spKey, d.Options)
		if err != nil {
			return err
		}
		d.sp.Key = signer
		d.sp.Certificate = cert
		d.sp.Intermediates = []*x509.Certificate{}
		d.sp.SignatureMethod = dsig.RSASHA256SignatureMethod
	}
	if allow, ok := dipper.GetMapDataBool(d.Options, "data.allow_idp_initiated"); ok {
		d.sp.AllowIDPInitiated = allow
	}
	d.allowIDPInit = d.sp.AllowIDPInitiated

	d.tokenExpiration = defaultTokenExpiration
	if raw, ok := dipper.GetMapData(d.Options, "data.token_expiration"); ok {
		if seconds, ok := raw.(float64); ok && seconds > 0 {
			d.tokenExpiration = time.Duration(int64(seconds)) * time.Second
		}
	}
	d.requestTTL = defaultRequestTTL
	if ttlRaw, ok := dipper.GetMapData(d.Options, "data.request_ttl"); ok {
		if seconds, ok := ttlRaw.(float64); ok && seconds > 0 {
			d.requestTTL = time.Duration(int64(seconds)) * time.Second
		}
	}

	d.jwtSigningKey = []byte(jwtSigningKey)
	if len(d.jwtSigningKey) < 32 {
		padding := make([]byte, 32-len(d.jwtSigningKey))
		d.jwtSigningKey = append(d.jwtSigningKey, padding...)
	}

	d.GetLogger().Debugf("[%s] SAML driver initialized for ACS %s and metadata %s", d.Service, acsURL.String(), metadataURL.String())

	return nil
}

func parseSPKeyPair(spKey string, options interface{}) (crypto.Signer, *x509.Certificate, error) {
	spCert, ok := dipper.GetMapDataStr(options, "data.sp_cert")
	if !ok || spCert == "" {
		return nil, nil, fmt.Errorf("%w: data.sp_cert is required when data.sp_key is set", ErrMissingSAMLConfig)
	}

	block, _ := pem.Decode([]byte(spKey))
	if block == nil {
		return nil, nil, fmt.Errorf("%w: data.sp_key is not valid PEM", ErrMissingSAMLConfig)
	}
	privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse data.sp_key: %w", err)
	}
	signer, ok := privKey.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("%w: data.sp_key does not implement crypto.Signer", ErrMissingSAMLConfig)
	}

	certBlock, _ := pem.Decode([]byte(spCert))
	if certBlock == nil {
		return nil, nil, fmt.Errorf("%w: data.sp_cert is not valid PEM", ErrMissingSAMLConfig)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse data.sp_cert: %w", err)
	}

	return signer, cert, nil
}

func (d *authSAMLDriver) samlLogin(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	payload := mustPayloadMap(m.Payload)
	relayState, _ := dipper.GetMapDataStr(payload, "relay_state")
	if relayState == "" {
		relayState = randomToken(24)
	}

	req, err := d.sp.MakeAuthenticationRequest(
		d.sp.GetSSOBindingLocation(saml.HTTPRedirectBinding),
		saml.HTTPRedirectBinding,
		saml.HTTPPostBinding,
	)
	if err != nil {
		panic(err)
	}
	redirectURL, err := req.Redirect(relayState, d.sp)
	if err != nil {
		panic(err)
	}
	d.requestByRelay.Store(relayState, samlRequestState{requestID: req.ID, expiresAt: time.Now().Add(d.requestTTL)})

	m.Reply <- dipper.Message{Payload: map[string]interface{}{
		"redirect_url": redirectURL.String(),
		"relay_state":  relayState,
	}}
}

func (d *authSAMLDriver) samlACS(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	payload := mustPayloadMap(m.Payload)

	samlResponse, ok := payloadString(payload, "SAMLResponse", "saml_response")
	if !ok || samlResponse == "" {
		panic(ErrMissingSAMLReply)
	}
	relayState, _ := payloadString(payload, "RelayState", "relay_state")

	possibleRequestIDs := []string{}
	if relayState != "" {
		if state, ok := d.lookupRequest(relayState); ok {
			possibleRequestIDs = []string{state.requestID}
		} else if !d.allowIDPInit {
			panic(ErrUnknownRelayState)
		}
	}

	postForm := url.Values{}
	postForm.Set("SAMLResponse", samlResponse)
	if relayState != "" {
		postForm.Set("RelayState", relayState)
	}
	req := &http.Request{
		Method:   http.MethodPost,
		URL:      &d.sp.AcsURL,
		PostForm: postForm,
		Form:     postForm,
	}

	assertion, err := d.sp.ParseResponse(req, possibleRequestIDs)
	if err != nil {
		panic(err)
	}

	claimsData := assertionToClaims(assertion)
	subject := chooseClaim(claimsData, "nameID", "email", "mail", "uid", "upn")
	if subject == "" {
		panic(ErrMissingSubject)
	}
	profileName := chooseClaim(claimsData, "displayName", "name", "givenName", "email", "nameID")
	if profileName == "" {
		profileName = subject
	}

	token, err := d.signSessionToken(subject, profileName, claimsData)
	if err != nil {
		panic(err)
	}

	m.Reply <- dipper.Message{Payload: map[string]interface{}{
		"token":        token,
		"subject":      subject,
		"profile_name": profileName,
		"relay_state":  relayState,
	}}
}

func (d *authSAMLDriver) authWebRequest(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	tokenString, err := bearerTokenFromPayload(mustPayloadMap(m.Payload))
	if err != nil {
		panic(err)
	}

	claims, err := d.verifySessionToken(tokenString)
	if err != nil {
		panic(err)
	}

	subject, _ := claims["sub"].(string)
	profileName, _ := claims["profile_name"].(string)
	if profileName == "" {
		profileName = subject
	}

	data := map[string]interface{}{}
	if raw, ok := claims["data"].(map[string]interface{}); ok {
		data = raw
	}

	m.Reply <- dipper.Message{Payload: map[string]interface{}{
		"Subject":     subject,
		"ProfileName": profileName,
		"Data":        data,
	}}
}

func (d *authSAMLDriver) signSessionToken(subject, profileName string, data map[string]interface{}) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":          subject,
		"profile_name": profileName,
		"data":         data,
		"iat":          now.Unix(),
		"exp":          now.Add(d.tokenExpiration).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(d.jwtSigningKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign session token: %w", err)
	}

	return signed, nil
}

func (d *authSAMLDriver) verifySessionToken(tokenString string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidSessionJWT
		}

		return d.jwtSigningKey, nil
	})
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidSessionJWT
	}

	subject, ok := claims["sub"].(string)
	if !ok || subject == "" {
		return nil, ErrInvalidSessionJWT
	}

	return claims, nil
}

func (d *authSAMLDriver) lookupRequest(relayState string) (samlRequestState, bool) {
	raw, ok := d.requestByRelay.Load(relayState)
	if !ok {
		return samlRequestState{}, false
	}
	state := raw.(samlRequestState)
	if time.Now().After(state.expiresAt) {
		d.requestByRelay.Delete(relayState)

		return samlRequestState{}, false
	}
	d.requestByRelay.Delete(relayState)

	return state, true
}

func bearerTokenFromPayload(payload map[string]interface{}) (string, error) {
	const prefix = "bearer "
	authHeader, ok := dipper.GetMapDataStr(payload, "headers.Authorization.0")
	if !ok {
		authHeader, ok = dipper.GetMapDataStr(payload, "headers.authorization.0")
	}
	if !ok || len(authHeader) <= len(prefix) || !strings.EqualFold(authHeader[:len(prefix)], prefix) {
		return "", ErrInvalidBearerToken
	}

	return authHeader[len(prefix):], nil
}

func mustPayloadMap(payload interface{}) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{}
	}
	mapped, ok := payload.(map[string]interface{})
	if !ok {
		panic(fmt.Errorf("%w: %T", ErrInvalidPayloadType, payload))
	}

	return mapped
}

func payloadString(payload map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			switch typed := value.(type) {
			case string:
				if typed != "" {
					return typed, true
				}
			case []interface{}:
				if len(typed) > 0 {
					if first, ok := typed[0].(string); ok && first != "" {
						return first, true
					}
				}
			}
		}
	}
	if rawBody, ok := payload["body"].(string); ok && rawBody != "" {
		if values, err := url.ParseQuery(rawBody); err == nil {
			for _, key := range keys {
				if value := values.Get(key); value != "" {
					return value, true
				}
			}
		}
	}

	return "", false
}

func assertionToClaims(assertion *saml.Assertion) map[string]interface{} {
	claims := map[string]interface{}{}
	if assertion == nil {
		return claims
	}
	if assertion.Subject != nil && assertion.Subject.NameID != nil && assertion.Subject.NameID.Value != "" {
		claims["nameID"] = assertion.Subject.NameID.Value
	}

	for _, statement := range assertion.AttributeStatements {
		for _, attr := range statement.Attributes {
			values := []string{}
			for _, value := range attr.Values {
				if value.Value != "" {
					values = append(values, value.Value)
				}
			}
			if len(values) == 0 {
				continue
			}
			if len(values) == 1 {
				claims[attr.Name] = values[0]
				if attr.FriendlyName != "" {
					claims[attr.FriendlyName] = values[0]
				}
			} else {
				asInterfaces := make([]interface{}, 0, len(values))
				for _, v := range values {
					asInterfaces = append(asInterfaces, v)
				}
				claims[attr.Name] = asInterfaces
				if attr.FriendlyName != "" {
					claims[attr.FriendlyName] = asInterfaces
				}
			}
		}
	}

	return claims
}

func chooseClaim(claims map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := claims[key]; ok {
			switch typed := value.(type) {
			case string:
				if typed != "" {
					return typed
				}
			case []interface{}:
				if len(typed) > 0 {
					if first, ok := typed[0].(string); ok && first != "" {
						return first
					}
				}
			}
		}
	}

	return ""
}

func randomToken(length int) string {
	if length <= 0 {
		length = 24
	}
	bytesLen := (length * 3 / 4) + 2
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	encoded := base64.RawURLEncoding.EncodeToString(buf)
	if len(encoded) < length {
		return encoded
	}

	return encoded[:length]
}
