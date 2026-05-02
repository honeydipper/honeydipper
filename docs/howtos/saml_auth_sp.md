# SAML SP Authentication Setup

This guide explains how to configure Honeydipper with the built-in `auth-saml` driver so Honeydipper acts as a SAML Service Provider (SP).

<!-- toc -->

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [API Endpoints Added By This Setup](#api-endpoints-added-by-this-setup)
- [Daemon And Driver Configuration](#daemon-and-driver-configuration)
- [Casbin Policy Examples](#casbin-policy-examples)
- [Expected Login Flow](#expected-login-flow)
- [Notes And Security Recommendations](#notes-and-security-recommendations)

<!-- tocstop -->

## Overview

The `auth-saml` driver provides four RPC handlers:

1. `saml_login`: creates a redirect URL for SP-initiated login.
2. `saml_acs`: validates `SAMLResponse` and returns a Honeydipper session token.
3. `auth_web_request`: validates session token from `Authorization: Bearer ...` and returns principal data to API middleware.
4. `saml_sp_metadata`: returns SP metadata XML for IdP configuration.

When SAML login is complete, Honeydipper API authorization still uses Casbin, so SSO and authorization policy remain separate.

## Prerequisites

1. IdP metadata URL is reachable from the daemon runtime.
2. You know the ACS callback URL exposed by your Honeydipper API.
3. You set a strong JWT signing key for the session token (`drivers.auth-saml.jwt_signing_key` or `AUTH_SAML_JWT_SIGNING_KEY`).

## API Endpoints Added By This Setup

When using this repository version, API service includes these local endpoints:

1. `GET /api/auth/saml/login`
2. `GET /api/auth/saml/metadata`
3. `POST /api/auth/saml/callback`
4. `GET /api/auth/saml/callback`

Typical usage:

1. Frontend calls `/api/auth/saml/login` and redirects browser to `redirect_url`.
2. IdP admin imports metadata from `/api/auth/saml/metadata` into the IdP.
3. IdP posts assertion to `/api/auth/saml/callback`.
4. Callback response contains a Honeydipper JWT token.
5. Frontend stores token and sends it in the `Authorization` header for API calls.

## Daemon And Driver Configuration

Example daemon config snippet:

```yaml
drivers:
  daemon:
    featureMap:
      global:
        eventbus: redisqueue
    drivers:
      auth-saml:
        name: auth-saml
        type: builtin
        handlerData:
          shortName: auth-saml

    services:
      api:
        auth-providers:
          - auth-saml
        auth:
          casbin:
            models:
              - |
                [request_definition]
                r = sub, principal, obj, act, provider

                [policy_definition]
                p = sub, obj, act, provider

                [role_definition]
                g = _, _

                [policy_effect]
                e = some(where (p.eft == allow))

                [matchers]
                m = (r.sub == p.sub && r.obj == p.obj && r.act == p.act && r.provider == p.provider) \
                  || (g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act && r.provider == p.provider)

            policies:
              - |
                p, everyone, user/profile, read, auth-saml
                g, alice@example.com, everyone
```

Driver data snippet:

```yaml
drivers:
  auth-saml:
    acs_url: https://hd.example.com/api/auth/saml/callback
    idp_metadata_url: https://idp.example.com/metadata

    # Optional: defaults to acs_url
    entity_id: https://hd.example.com/saml/sp

    # Optional: defaults to false
    allow_idp_initiated: false

    # Optional: seconds, default 86400
    token_expiration: 86400

    # Optional: seconds, default 600
    request_ttl: 600

    # Recommended via env var in production
    jwt_signing_key: change-me-to-a-strong-random-value
```

Environment variable alternative:

```bash
export AUTH_SAML_JWT_SIGNING_KEY='replace-with-a-strong-random-secret'
```

## Casbin Policy Examples

Because API auth can now pass a principal object (`r = sub, principal, obj, act, provider`), you can write claim-based rules.

Example using SAML assertion attributes in `r.principal.Data`:

```ini
[matchers]
m = (
  r.obj == p.obj &&
  r.act == p.act &&
  r.provider == p.provider &&
  r.principal.Data["groups"] == "platform-admins"
)
```

Important note for multi-value attributes:

1. Single-value attributes are stored as string.
2. Multi-value attributes are stored as list.

If your IdP sends group values as a list, match with list-aware logic in model expressions.

## Expected Login Flow

1. Browser requests `GET /api/auth/saml/login`.
2. API calls `driver:auth-saml/saml_login` and returns `redirect_url`.
3. Browser redirects to IdP.
4. IdP sends assertion to `/api/auth/saml/callback`.
5. API calls `driver:auth-saml/saml_acs`.
6. Driver validates assertion and returns Honeydipper JWT.
7. Browser stores JWT and uses `Authorization: Bearer <token>`.
8. API auth middleware calls `driver:auth-saml/auth_web_request` for each request.

## Notes And Security Recommendations

1. Use HTTPS for all callback and API URLs.
2. Keep `jwt_signing_key` out of git; prefer environment variable or secret manager.
3. Keep `allow_idp_initiated` disabled unless your flow explicitly requires IdP-initiated login.
4. Keep `request_ttl` short (default 10 minutes) to reduce replay window for stale relay states.
5. Restrict authorization through Casbin policies; successful SAML authentication alone should not grant broad access.