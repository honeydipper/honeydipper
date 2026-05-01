// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package auth-saml enables Honeydipper to authenticate web requests using SAML as an SP.
package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	saml2 "github.com/russellhaering/gosaml2"
	saml2types "github.com/russellhaering/gosaml2/types"
	dsig "github.com/russellhaering/goxmldsig"
)

const (
	defaultTokenExpiration = 24 * time.Hour
	defaultRequestTTL      = 10 * time.Minute
)

var (
	ErrInvalidBearerToken     = errors.New("invalid bearer token")
	ErrMissingSAMLConfig      = errors.New("missing SAML driver config")
	ErrMissingSAMLReply       = errors.New("missing SAMLResponse")
	ErrUnknownRelayState      = errors.New("unknown relay state")
	ErrInvalidInResponse      = errors.New("unexpected InResponseTo value")
	ErrMetadataHTTPStatus     = errors.New("metadata endpoint returned non-success status")
	ErrUnsupportedPrivKey     = errors.New("unsupported private key format")
	ErrInvalidSessionJWT      = errors.New("invalid session token")
	ErrMissingSubject         = errors.New("missing subject in SAML assertion")
	ErrInvalidPayloadType     = errors.New("invalid payload type")
	ErrUnsuccessfulSAMLStatus = errors.New("idp returned non-success SAML status")
)

type authSAMLDriver struct {
	*dipper.Driver

	sp              *saml2.SAMLServiceProvider
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

type samlStatusCodeNode struct {
	Value      string              `xml:"Value,attr"`
	StatusCode *samlStatusCodeNode `xml:"StatusCode"`
}

type samlStatusEnvelope struct {
	Status struct {
		StatusCode    *samlStatusCodeNode `xml:"StatusCode"`
		StatusMessage string              `xml:"StatusMessage"`
	} `xml:"Status"`
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
	driver.RPCHandlers["saml_sp_metadata"] = driver.samlSPMetadata
	driver.Reload = driver.setupConfig
	driver.Start = driver.setupConfig
	driver.Run()
}

func (d *authSAMLDriver) setupConfig(_ *dipper.Message) {
	if err := d.initConfig(); err != nil {
		d.GetLogger().Panicf("failed to initialize auth-saml driver: %v", err)
	}
}

//nolint:gocyclo,funlen
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

	idpMetadata, err := fetchIDPMetadata(metadataURL.String())
	if err != nil {
		return fmt.Errorf("failed to fetch idp metadata: %w", err)
	}
	idpCerts, err := extractIDPSigningCerts(idpMetadata)
	if err != nil {
		return fmt.Errorf("failed to parse idp signing certs: %w", err)
	}
	if len(idpCerts) == 0 {
		return fmt.Errorf("%w: no idp signing certs in metadata", ErrMissingSAMLConfig)
	}
	idpSSOURL := pickIDPSSOURL(idpMetadata, saml2.BindingHttpRedirect)
	if idpSSOURL == "" {
		return fmt.Errorf("%w: no idp sso url in metadata", ErrMissingSAMLConfig)
	}

	entityID := acsURL.String()
	if configuredEntityID, ok := dipper.GetMapDataStr(d.Options, "data.entity_id"); ok && configuredEntityID != "" {
		entityID = configuredEntityID
	}

	idpCertStore := &dsig.MemoryX509CertificateStore{Roots: idpCerts}
	nameIDFormat := saml2.NameIdFormatEmailAddress
	if configured, ok := dipper.GetMapDataStr(d.Options, "data.name_id_format"); ok && strings.TrimSpace(configured) != "" {
		nameIDFormat = configured
	}

	d.sp = &saml2.SAMLServiceProvider{
		IdentityProviderSSOURL:      idpSSOURL,
		IdentityProviderSSOBinding:  saml2.BindingHttpRedirect,
		IdentityProviderIssuer:      idpMetadata.EntityID,
		AssertionConsumerServiceURL: acsURL.String(),
		ServiceProviderIssuer:       entityID,
		AudienceURI:                 entityID,
		NameIdFormat:                nameIDFormat,
		IDPCertificateStore:         idpCertStore,
		SkipSignatureValidation:     false,
		SignAuthnRequestsAlgorithm:  dsig.RSASHA256SignatureMethod,
		Clock:                       dsig.NewRealClock(),
	}

	hasSPEncryptionKey := false
	if spKey, ok := dipper.GetMapDataStr(d.Options, "data.sp_key"); ok && spKey != "" {
		signer, cert, err := parseSPKeyPair(spKey, d.Options)
		if err != nil {
			return err
		}
		if err := d.sp.SetSPKeyStore(&saml2.KeyStore{Signer: signer, Cert: cert.Raw}); err != nil {
			return fmt.Errorf("failed to set sp encryption key: %w", err)
		}
		// gosaml2 response decryption currently relies on the legacy SPKeyStore field.
		d.sp.SPKeyStore = dsig.TLSCertKeyStore(tls.Certificate{Certificate: [][]byte{cert.Raw}, PrivateKey: signer})
		hasSPEncryptionKey = true
	}
	hasSPSigningKey := false
	//nolint:nestif // this is simple enough
	if spSigningKey, ok := dipper.GetMapDataStr(d.Options, "data.sp_signing_key"); ok && spSigningKey != "" {
		signer, cert, err := parseNamedSPKeyPair(spSigningKey, d.Options, "data.sp_signing_cert")
		if err != nil {
			return err
		}
		if err := d.sp.SetSPSigningKeyStore(&saml2.KeyStore{Signer: signer, Cert: cert.Raw}); err != nil {
			return fmt.Errorf("failed to set sp signing key: %w", err)
		}
		hasSPSigningKey = true
		if !hasSPEncryptionKey {
			if err := d.sp.SetSPKeyStore(&saml2.KeyStore{Signer: signer, Cert: cert.Raw}); err != nil {
				return fmt.Errorf("failed to set sp encryption fallback key: %w", err)
			}
			d.sp.SPKeyStore = dsig.TLSCertKeyStore(tls.Certificate{Certificate: [][]byte{cert.Raw}, PrivateKey: signer})
			hasSPEncryptionKey = true
			d.GetLogger().Warning("[auth-saml] data.sp_key/data.sp_cert not configured; using signing keypair as encryption fallback")
		}
	}
	signAuthnRequests := hasSPSigningKey || hasSPEncryptionKey
	if configured, ok := dipper.GetMapDataBool(d.Options, "data.sign_authn_requests"); ok {
		signAuthnRequests = configured
	}
	if signAuthnRequests && !hasSPSigningKey && !hasSPEncryptionKey {
		return fmt.Errorf("%w: enable signing requires data.sp_signing_key/sp_signing_cert or data.sp_key/sp_cert", ErrMissingSAMLConfig)
	}
	d.sp.SignAuthnRequests = signAuthnRequests
	if allow, ok := dipper.GetMapDataBool(d.Options, "data.allow_idp_initiated"); ok {
		d.allowIDPInit = allow
	} else {
		d.allowIDPInit = false
	}

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
	return parseNamedSPKeyPair(spKey, options, "data.sp_cert")
}

func parseNamedSPKeyPair(spKey string, options interface{}, certField string) (crypto.Signer, *x509.Certificate, error) {
	spCert, ok := dipper.GetMapDataStr(options, certField)
	if !ok || spCert == "" {
		return nil, nil, fmt.Errorf("%w: %s is required for configured key", ErrMissingSAMLConfig, certField)
	}

	block, _ := pem.Decode([]byte(spKey))
	if block == nil {
		return nil, nil, fmt.Errorf("%w: data.sp_key is not valid PEM", ErrMissingSAMLConfig)
	}
	privKey, err := parsePrivateKey(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse key: %w", err)
	}
	signer, ok := privKey.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("%w: configured key does not implement crypto.Signer", ErrMissingSAMLConfig)
	}

	certBlock, _ := pem.Decode([]byte(spCert))
	if certBlock == nil {
		return nil, nil, fmt.Errorf("%w: %s is not valid PEM", ErrMissingSAMLConfig, certField)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse %s: %w", certField, err)
	}

	return signer, cert, nil
}

func parsePrivateKey(block *pem.Block) (interface{}, error) {
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		switch key.(type) {
		case *rsa.PrivateKey, *ecdsa.PrivateKey:
			return key, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	return nil, ErrUnsupportedPrivKey
}

func fetchIDPMetadata(metadataURL string) (*saml2types.EntityDescriptor, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, metadataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build metadata request: %w", err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: %d", ErrMetadataHTTPStatus, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata response: %w", err)
	}
	entity := &saml2types.EntityDescriptor{}
	if err := xml.Unmarshal(body, entity); err != nil {
		return nil, fmt.Errorf("failed to parse metadata xml: %w", err)
	}

	return entity, nil
}

func extractIDPSigningCerts(entity *saml2types.EntityDescriptor) ([]*x509.Certificate, error) {
	if entity == nil || entity.IDPSSODescriptor == nil {
		return nil, nil
	}
	certs := make([]*x509.Certificate, 0)
	for _, kd := range entity.IDPSSODescriptor.KeyDescriptors {
		if kd.Use != "" && kd.Use != "signing" {
			continue
		}
		for _, certData := range kd.KeyInfo.X509Data.X509Certificates {
			if strings.TrimSpace(certData.Data) == "" {
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(certData.Data))
			if err != nil {
				return nil, fmt.Errorf("failed to decode idp certificate: %w", err)
			}
			cert, err := x509.ParseCertificate(raw)
			if err != nil {
				return nil, fmt.Errorf("failed to parse idp certificate: %w", err)
			}
			certs = append(certs, cert)
		}
	}

	return certs, nil
}

func pickIDPSSOURL(entity *saml2types.EntityDescriptor, binding string) string {
	if entity == nil || entity.IDPSSODescriptor == nil {
		return ""
	}
	for _, service := range entity.IDPSSODescriptor.SingleSignOnServices {
		if service.Binding == binding && service.Location != "" {
			return service.Location
		}
	}
	for _, service := range entity.IDPSSODescriptor.SingleSignOnServices {
		if service.Location != "" {
			return service.Location
		}
	}

	return ""
}

func (d *authSAMLDriver) samlLogin(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	payload := mustPayloadMap(m.Payload)
	relayState, _ := dipper.GetMapDataStr(payload, "relay_state")
	if relayState == "" {
		relayState = randomToken(24)
	}

	doc, err := d.sp.BuildAuthRequestDocument()
	if err != nil {
		panic(err)
	}
	requestID := doc.Root().SelectAttrValue("ID", "")
	if requestID == "" {
		panic(ErrMissingSAMLConfig)
	}
	redirectURL, err := d.sp.BuildAuthURLRedirect(relayState, doc)
	if err != nil {
		panic(err)
	}
	d.requestByRelay.Store(relayState, samlRequestState{requestID: requestID, expiresAt: time.Now().Add(d.requestTTL)})

	m.Reply <- dipper.Message{Payload: map[string]interface{}{
		"redirect_url": redirectURL,
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

	if statusCodePath, statusMessage, ok := extractSAMLStatus(samlResponse); ok &&
		statusCodePath != "" && !strings.HasSuffix(statusCodePath, ":Success") {
		detail := statusCodePath
		if statusMessage != "" {
			detail = fmt.Sprintf("%s (%s)", statusCodePath, statusMessage)
		}
		panic(fmt.Errorf("%w: %s", ErrUnsuccessfulSAMLStatus, detail))
	}

	response, err := d.sp.ValidateEncodedResponse(samlResponse)
	if err != nil {
		panic(err)
	}
	if len(possibleRequestIDs) > 0 {
		if !matchesRequestID(response.InResponseTo, possibleRequestIDs) {
			panic(ErrInvalidInResponse)
		}
	}
	if len(response.Assertions) == 0 {
		panic(ErrMissingSubject)
	}
	assertion := &response.Assertions[0]

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
		"data":         claimsData,
		"relay_state":  relayState,
	}}
}

func (d *authSAMLDriver) samlSPMetadata(m *dipper.Message) {
	metadata, err := d.sp.Metadata()
	if err != nil {
		panic(err)
	}
	if metadata.SPSSODescriptor != nil && d.sp.NameIdFormat != "" {
		metadata.SPSSODescriptor.NameIDFormats = []string{d.sp.NameIdFormat}
	}
	serialized, err := xml.MarshalIndent(metadata, "", "  ")
	if err != nil {
		panic(err)
	}

	m.Reply <- dipper.Message{Payload: map[string]interface{}{
		"metadata": xml.Header + string(serialized),
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

func extractSAMLStatus(encodedResponse string) (statusCodePath, statusMessage string, ok bool) {
	decodeAttempts := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
	}

	var decoded []byte
	for _, decode := range decodeAttempts {
		body, err := decode(strings.TrimSpace(encodedResponse))
		if err == nil {
			decoded = body

			break
		}
	}
	if len(decoded) == 0 {
		return "", "", false
	}

	envelope := &samlStatusEnvelope{}
	if err := xml.Unmarshal(decoded, envelope); err != nil {
		return "", "", false
	}
	if envelope.Status.StatusCode == nil {
		return "", strings.TrimSpace(envelope.Status.StatusMessage), false
	}

	parts := []string{}
	for code := envelope.Status.StatusCode; code != nil; code = code.StatusCode {
		if code.Value != "" {
			parts = append(parts, code.Value)
		}
	}

	return strings.Join(parts, " -> "), strings.TrimSpace(envelope.Status.StatusMessage), len(parts) > 0
}

func assertionToClaims(assertion *saml2types.Assertion) map[string]interface{} {
	claims := map[string]interface{}{}
	if assertion == nil {
		return claims
	}
	if assertion.Subject != nil && assertion.Subject.NameID != nil && assertion.Subject.NameID.Value != "" {
		claims["nameID"] = assertion.Subject.NameID.Value
	}

	if assertion.AttributeStatement == nil {
		return claims
	}
	for _, attr := range assertion.AttributeStatement.Attributes {
		addAttributeClaim(claims, attr)
	}

	return claims
}

func addAttributeClaim(claims map[string]interface{}, attr saml2types.Attribute) {
	values := []string{}
	for _, value := range attr.Values {
		if value.Value != "" {
			values = append(values, value.Value)
		}
	}
	if len(values) == 0 {
		return
	}
	if len(values) == 1 {
		claims[attr.Name] = values[0]
		if attr.FriendlyName != "" {
			claims[attr.FriendlyName] = values[0]
		}

		return
	}
	asInterfaces := make([]interface{}, 0, len(values))
	for _, v := range values {
		asInterfaces = append(asInterfaces, v)
	}
	claims[attr.Name] = asInterfaces
	if attr.FriendlyName != "" {
		claims[attr.FriendlyName] = asInterfaces
	}
}

func matchesRequestID(actual string, expected []string) bool {
	if actual == "" {
		return false
	}
	for _, exp := range expected {
		if actual == exp {
			return true
		}
	}

	return false
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
