// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// session provider tiers.
const (
	tierIAP    = "iap"
	tierGitHub = "github"
	tierSAML   = "saml"
	tierSimple = "simple"
	tierOther  = "other"
)

// providerTier maps a provider name to its session-enforcement tier.
//
// IAP takes creds directly from the IAP JWT and never touches the session
// store. GitHub/SAML check the session in Redis. auth-simple is also stored so
// logout/revoke works and idle applies; it has no provider re-vouch so expiry
// is a login.
func (l *Store) providerTier(provider string) string {
	switch {
	case provider == "auth-gcp-iap" || strings.Contains(provider, "iap"):
		return tierIAP
	case provider == "auth-github":
		return tierGitHub
	case provider == "auth-saml":
		return tierSAML
	case provider == "auth-simple":
		return tierSimple
	default:
		return tierOther
	}
}

// enforceSession applies the daemon session policy to an authenticated
// principal. alreadyVouched indicates the principal was just returned by the
// provider driver (driver-fallback path), meaning the provider confirmed the
// session is alive this request.
//
// It returns (proceed, effectivePrincipal). When proceed is false the caller
// must abort with the session-expired response.
func (l *Store) enforceSession(c *gin.Context, principal Principal, engine *sessionEngine, alreadyVouched bool) (bool, Principal) {
	if engine == nil || !engine.enabled || engine.manager == nil {
		return true, principal
	}

	if l.providerTier(principal.Provider) == tierIAP {
		// IAP: no session store involvement (driver validates the IAP JWT).
		return true, principal
	}

	sid := principal.SID
	if sid == "" {
		// A freshly vouched session from the driver-fallback path (provider
		// alive this request) carries no sid yet; the caller mints one
		// afterwards, so it must proceed.
		if alreadyVouched {
			return true, principal
		}
		// Pre-upgrade (sid-less) token. Accepted best-effort during the
		// configured grace period; cleanly rejected afterwards.
		if engine.manager.GraceActive() {
			return true, principal
		}
		l.markSessionExpired(c)

		return false, principal
	}

	res := engine.manager.Check(sessionCtx, sid, principal.Subject, principal.Provider, 0)
	if res.StoreDown || res.Revoked {
		l.markSessionExpired(c)

		return false, principal
	}
	if res.MaxLifetimeExceeded {
		// Hard re-auth: never silently re-vouch past the absolute cap.
		l.markSessionExpired(c)

		return false, principal
	}
	if res.IdleExceeded {
		return l.handleIdleExceeded(c, principal, alreadyVouched)
	}

	// Accepted: any authenticated request reaching an API route is activity
	// (idle definition).
	_ = engine.manager.Refresh(sessionCtx, sid)

	return true, principal
}

func (l *Store) handleIdleExceeded(c *gin.Context, principal Principal, alreadyVouched bool) (bool, Principal) {
	engine := l.sessionEngine
	if engine == nil || engine.manager == nil {
		return true, principal
	}

	if alreadyVouched {
		// The provider driver just re-vouched this very request (provider
		// alive): a silent renewal with the same sid. This is the idle-exceeded
		// -> re-vouch path for any stored provider tier; only GitHub reaches it
		// in practice because others do not map to a re-vouchable provider.
		_ = engine.manager.Refresh(sessionCtx, principal.SID)

		return true, principal
	}

	// A daemon JWT cannot re-confirm the provider session on its own. Idle thus
	// forces a fresh login with a new sid for every stored tier:
	//   - GitHub: no re-vouch without going back through the provider
	//   - SAML: no IdP session introspection exists -> re-auth (new sid)
	//   - auth-simple: no refresh possible -> expiry = login
	l.markSessionExpired(c)
	l.mustReauthWithNewSID(c)

	return false, principal
}

// markSessionExpired signals that the presented session cannot continue and
// records an Access-Control-Expose-Headers entry so cross-origin UIs can read
// the indicator.
func (l *Store) markSessionExpired(c *gin.Context) {
	c.Header(SessionExpiredHeader, "true")
	l.ensureExposeHeader(c, SessionExpiredHeader)
}

// mustReauthWithNewSID marks that a fresh login (with a new sid) is required.
// It is informational for the UI layer (Phase B) and reinforces the
// session-expired signal already set.
func (l *Store) mustReauthWithNewSID(c *gin.Context) {
	c.Header("X-Honeydipper-Reauth", "new-session")
	l.ensureExposeHeader(c, "X-Honeydipper-Reauth")
}

// markSessionExpiredRequest signals session-expired through a RequestContext
// used by local handlers (which are not gin.Context directly).
func (l *Store) markSessionExpiredRequest(rc RequestContext) {
	if g, ok := rc.(*GinRequestContext); ok {
		l.markSessionExpired(g.gin)
	}
}

// ensureExposeHeader adds header to Access-Control-Expose-Headers without
// duplicating an existing entry.
func (l *Store) ensureExposeHeader(c *gin.Context, header string) {
	exposed := c.Writer.Header().Get("Access-Control-Expose-Headers")
	if exposed == "" {
		c.Header("Access-Control-Expose-Headers", header)

		return
	}
	for _, part := range strings.Split(exposed, ",") {
		if strings.EqualFold(strings.TrimSpace(part), header) {
			return
		}
	}
	c.Header("Access-Control-Expose-Headers", exposed+", "+header)
}

// abortSessionExpired writes a 401 with the session-expired indicator set. All
// expiry modes funnel here so the UI sees one consistent re-authenticate path.
func abortSessionExpired(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]interface{}{
		"error":   "session expired",
		"reAuth":  true,
		"expired": true,
	})
}
