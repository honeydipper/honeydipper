// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/api/session"
)

// newSID mints a fresh session identifier. The daemon is the sole authority for
// minting sids; it is opaque, random, and stable across token rotation.
func (l *Store) newSID() string {
	return l.newUUID()
}

// RegisterEagerSession eagerly registers a login session for a principal that
// carries a sid. Used at login time (SAML ACS, GitHub OAuth callback) and on
// the driver-fallback path so the session exists in the store before the first
// request.
func (l *Store) RegisterEagerSession(principal Principal) {
	if !l.sessionEnabled() {
		return
	}
	if principal.SID == "" || principal.Subject == "" {
		return
	}
	now := time.Now().Unix()
	_ = l.sessionEngine.manager.Store().Register(sessionCtx, session.Record{
		SID:      principal.SID,
		Subject:  principal.Subject,
		Provider: principal.Provider,
		IssuedAt: now,
		LastSeen: now,
	})
}

// ensureSIDForPrincipal makes sure a principal from the provider-fallback path
// carries a sid whenever its provider tier uses the session store. Providers
// that embed their own sid (GitHub driver) keep it; providers that do not
// (e.g. auth-simple) get a fresh daemon-minted sid so logout/revoke and idle
// tracking work. IAP never touches the session store and is left untouched.
func (l *Store) ensureSIDForPrincipal(principal Principal) Principal {
	if !l.sessionEnabled() {
		return principal
	}
	tier := l.providerTier(principal.Provider)
	if tier == tierIAP {
		return principal
	}
	if principal.SID != "" {
		return principal
	}
	principal.SID = l.newSID()
	l.RegisterEagerSession(principal)

	return principal
}

func (l *Store) sessionEnabled() bool {
	return l.sessionEngine != nil && l.sessionEngine.enabled && l.sessionEngine.manager != nil
}
