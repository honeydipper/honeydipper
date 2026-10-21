// Copyright 2026 PayPal Inc.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"fmt"
	"net/http"
)

// logoutHandler implements POST /auth/logout. It revokes the current sid (and,
// when requested via all=true, every session for the subject). Revocation is
// correct cross-node because it writes to the shared Redis session store.
func logoutHandler(r *Request) (map[string]interface{}, error) {
	p, ok := r.ctx.Get("principal")
	if !ok {
		r.ctx.AbortWithStatusJSON(http.StatusUnauthorized, map[string]interface{}{"error": "session expired", "expired": true})

		return nil, nil
	}
	principal, ok := p.(Principal)
	if !ok || principal.Provider == "none" || principal.Subject == "" {
		r.ctx.AbortWithStatusJSON(http.StatusUnauthorized, map[string]interface{}{"error": "session expired", "expired": true})

		return nil, nil
	}

	if !r.store.sessionEnabled() {
		return map[string]interface{}{"loggedOut": true}, nil
	}

	revokeAll := r.ctx.GetParam("all") == "true" || r.ctx.GetParam("all") == "1"
	manager := r.store.sessionEngine.manager
	if revokeAll {
		if _, err := manager.Store().RevokeAllForSubject(sessionCtx, principal.Subject); err != nil {
			return nil, fmt.Errorf("revoking all sessions: %w", err)
		}
	} else if principal.SID != "" {
		if err := manager.Store().Revoke(sessionCtx, principal.SID); err != nil {
			return nil, fmt.Errorf("revoking session: %w", err)
		}
	}

	r.store.markSessionExpiredRequest(r.ctx)

	return map[string]interface{}{"loggedOut": true}, nil
}
