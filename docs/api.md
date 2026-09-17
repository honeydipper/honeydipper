# HTTP API Reference

> Honeydipper exposes an HTTP API server that accepts webhook events, forwards workflow actions, and provides a conversation management interface for the UI. The API is served on a configurable address (default: `:9000`).

## Table of Contents

- [Overview](#overview)
- [Authentication](#authentication)
  - [JWT Authentication](#jwt-authentication)
  - [SAML Authentication](#saml-authentication)
  - [GitHub OAuth](#github-oauth)
  - [Casbin Authorization](#casbin-authorization)
- [API Endpoints](#api-endpoints)
  - [Event Operations](#event-operations)
  - [Agent/Conversation Operations](#agentconversation-operations)
  - [GitHub-Scoped Operations](#github-scoped-operations)
  - [Pod Log Operations](#pod-log-operations)
  - [Secret Operations](#secret-operations)
  - [Utility Endpoints](#utility-endpoints)
- [Request/Response Formats](#requestresponse-formats)
- [Error Handling](#error-handling)
- [Examples](#examples)

---

## Overview

The HTTP API server is started by the `api` service (`internal/service/api.go`). It listens on the configured address and routes requests based on definitions in `internal/api/def.go`.

Configuration (`daemon.services.api`):

```yaml
daemon:
  services:
    api:
      listener:
        addr: ":9000"     # Default API listen address
      api_prefix: "/api/"  # Default API path prefix
      healthcheck_prefix: "/healthz"
      writeTimeout: "10s"  # Response write timeout
      timeout: "10s"       # Default request timeout
      ack_timeout: "10ms"  # ACK wait timeout
      auth:
        casbin:
          models:
            - "[request_definition]"
            - "r = sub, obj, act, ..."
          policies:
            - "p, admin, event, *, allow"
        entitlementCacheTTL: "30m"
      auth-providers:
        - auth-saml.saml_login
        - auth-github.github_web_request
      ui_url: "https://honeydipper.example.com"
```

### Health Check

The health check endpoint (default `/healthz`) returns:
- `200 OK` when the API service is healthy
- `500 Internal Server Error` otherwise

No authentication is required for the health check.

---

## Authentication

The API supports multiple authentication methods, tried in order:

### 1. JWT Authentication (Preferred)

If `HD_JWT_SIGNING_KEY` is configured, the API accepts JWTs via the `Authorization` header:

```
Authorization: Bearer <jwt-token>
```

JWTs are issued after SAML or GitHub OAuth authentication. They contain:

```go
type Principal struct {
    Subject     string                 // Username or subject ID
    ProfileName string                 // Human-readable profile name
    Provider    string                 // Auth provider (e.g., "auth-saml", "auth-github")
    Data        map[string]interface{} // Additional provider-specific data
}
```

On subsequent requests (e.g., SAML rotation), a `X-Honeydipper-Refreshed-JWT` header may be returned with a new token.

### 2. Driver-Based Authentication

If JWT extraction fails, the API iterates through configured `auth-providers`:

```yaml
auth-providers:
  - auth-saml.saml_login
  - auth-github.github_web_request
```

Each provider is called via RPC with the web request data (excluding body). The provider returns a `Principal`.

#### SAML Authentication

The SAML flow uses these endpoints:

| Endpoint | Method | Description |
|---|---|---|
| `/api/auth/saml/login` | `GET` | Initiates SAML login, returns redirect URL |
| `/api/auth/saml/metadata` | `GET` | Returns SAML service provider metadata (XML) |
| `/api/auth/saml/callback` | `GET`/`POST` | SAML assertion consumer service callback |

The `/api/auth/saml/login` and `/api/auth/saml/callback` endpoints are anonymous (no prior auth required). After successful SAML authentication, the callback redirects to the UI with a JWT token as a query parameter.

#### GitHub OAuth

| Endpoint | Method | Description |
|---|---|---|
| `/api/auth/github/callback` | `GET` | GitHub OAuth callback handler |

### 3. Anonymous Access

Certain endpoints are configured as anonymous (no authentication required). These include SAML login endpoints and the health check.

### Casbin Authorization

After authentication, the API uses [Casbin](https://casbin.org/) for authorization. Each API object has a `casbin_model` and `casbin_policies` that define who can access which endpoints.

```yaml
# Example Casbin configuration
auth:
  casbin:
    models:
      - "[request_definition]"
        "r = sub, obj, act, dom"
      - "[policy_definition]"
        "p = sub, obj, act, dom"
      - "[policy_effect]"
        "e = some(where (p.eft == allow))"
      - "[matchers]"
        "m = r.sub == p.sub && r.obj == p.obj && r.act == p.act && r.dom == p.dom"
    policies:
      - "p, alice, event, POST, allow"
      - "p, bob, convo, *, allow"
```

If an endpoint has an `EntitlementProvider` configured, the API first checks derived subjects from the entitlement provider before falling back to Casbin rules. Entitlement results are cached (default TTL: 30 minutes).

---

## API Endpoints

### Utility Endpoints

#### Get User Profile

```
GET /api/user/profile
```

Returns the authenticated user's profile.

**Response (200):**
```json
{
  "profile_name": "Alice Smith",
  "subject": "alice@example.com",
  "provider": "auth-saml"
}
```

---

### Event Operations

These endpoints manage Honeydipper event sessions. They require `object: event` permission (or equivalent Casbin policy).

#### Wait for Event

```
GET /api/events/:eventID/wait
```

Long-polling endpoint that blocks until the specified event session completes.

| Param | Description |
|---|---|
| `:eventID` | Event session ID to wait for |

**Response (200):** Event results when the session completes.

**Response (202):** `{"uuid": "<uuid>", "results": {...}}` — Accepted, still processing (for long-running GET requests).

#### Pause Event Session

```
POST /api/events/:sessionID/pause
```

Pauses a running event session.

#### Resume Event Session

```
POST /api/events/:sessionID/resume
```

Resumes a paused event session.

#### Rerun Event Session

```
POST /api/events/:sessionID/rerun
```

Reruns a completed or failed event session.

#### Interact with Event Session

```
POST /api/events/:sessionID/interact
```

Sends interactive input to an event session (e.g., responding to a prompt).

**Request Body:**
```json
{
  "value": "user input text"
}
```

> This endpoint attaches the authenticated principal's user identity, allowing the interaction to be attributed.

#### Cancel Event Session

```
POST /api/events/:sessionID/cancel
```

Cancels a running event session.

#### List Events

```
GET /api/events
```

Lists current events in the system.

---

### Agent/Conversation Operations

These endpoints are served by the agent service and require `object: convo` or `object: agent` permission. The `user_provider` and `user` labels from authentication are forwarded to the agent.

#### List Conversations

```
GET /api/convos
```

Returns a list of recent conversations.

| Query Param | Default | Description |
|---|---|---|
| `look_back` | `12` | Number of 2-hour blocks to look back |
| `as_of` | (latest) | Cursor for pagination |

**Response (200):** Array of conversation state snapshots (stream_hvals format).

#### Create New Conversation

```
POST /api/convos
```

Starts a brand-new conversation for the named agent. Returns the generated `convo_id` synchronously.

**Request Body:**
```json
{
  "agent": "my_assistant",
  "text": "Help me deploy the frontend service."
}
```

**Response (200):**
```json
{
  "convo_id": "a1b2c3d4-..."
}
```

#### List Agents

```
GET /api/agents
```

Returns a sorted list of configured agent names.

**Response (200):**
```json
["code_reviewer", "github_assistant", "my_assistant"]
```

#### Get Conversation History

```
GET /api/convos/:convoID/history
```

Returns the full message history for a conversation.

#### Add Conversation Turn

```
POST /api/convos/:convoID/turn
```

Adds a new message (turn) to an existing conversation. The caller's user identity (`user@provider`) is attached.

**Request Body:**
```json
{
  "text": "What about the database migration?"
}
```

**Response (200):** `{"ok": true}`

#### Cancel Conversation

```
POST /api/convos/:convoID/cancel
```

Marks a conversation as cancelled. Active agent sessions will detect this on their next poll cycle and abort.

**Response (200):** `{"ok": true}`

---

### GitHub-Scoped Operations

These endpoints require GitHub entitlement checking. The `:gh_slug` path parameter (e.g., `owner/repo`) is checked against the `auth-github` entitlement provider.

#### List GitHub Events

```
GET /api/gh/events/*gh_slug
```

Lists events scoped to a GitHub repository.

#### Rerun GitHub Event

```
POST /api/gh/events/:sessionID/rerun/*gh_slug
```

Reruns a GitHub-triggered event session.

#### Pause GitHub Event

```
POST /api/gh/events/:sessionID/pause/*gh_slug
```

Pauses a GitHub-triggered event session.

#### Resume GitHub Event

```
POST /api/gh/events/:sessionID/resume/*gh_slug
```

Resumes a GitHub-triggered event session.

#### Interact with GitHub Event

```
POST /api/gh/events/:sessionID/interact/*gh_slug
```

Sends interactive input to a GitHub event session.

---

### Pod Log Operations

#### Get Pod Log Chunk

```
GET /api/pods/:pod_id/log/chunk
```

Retrieves a chunk of pod log data.

#### Get GitHub-Scoped Pod Log Chunk

```
GET /api/gh/pods/:pod_id/log/chunk/*gh_slug
```

Retrieves pod log data, scoped to a GitHub repository (entitlement-checked).

---

### Secret Operations

These endpoints manage secrets and require GitHub entitlement checking.

#### List Secrets

```
GET /api/gh/secrets/*gh_slug
```

Lists secrets for the specified GitHub repository scope.

#### Set Secret

```
POST /api/gh/secrets/*gh_slug
```

Sets a secret value in the configured secret driver.

#### Delete Secret

```
DELETE /api/gh/secrets/*gh_slug
```

Deletes a secret.

---

## Request/Response Formats

### Request Routing

API requests are routed based on the definitions in `internal/api/def.go`. Each endpoint has:

| Property | Description |
|---|---|
| `Object` | Casbin object for authorization |
| `Name` | Handler function name |
| `ReqType` | Request dispatch type (`TypeFirst`, `TypeAll`, `TypeLocal`) |
| `Service` | Target service (`engine`, `operator`, `receiver`, `agent`) |
| `AllowAnonymous` | If true, no authentication required |

### Request Types

| Type | Behavior |
|---|---|
| `TypeFirst` | First responder to return wins (most common) |
| `TypeAll` | Collects and aggregates results from all matching nodes |
| `TypeMatch` | Only nodes with matching data respond |
| `TypeLocal` | Handled locally within the API process (no eventbus dispatch) |

### Response Formats

**Success (200) — JSON data:**
```json
{
  "results": { ... },
  "status": "success"
}
```

**Success (200) — Raw body:**
Used for SAML metadata endpoints. Raw bytes with appropriate `Content-Type`.

**Accepted (202):**
Returned for long-running GET requests that haven't completed yet:
```json
{
  "uuid": "request-uuid",
  "results": { ... }
}
```

**Redirect (302):**
Used for SAML/OAuth callbacks. The `Location` header contains the redirect URL.

### Timeout Behavior

- **Default write timeout:** 10 seconds
- **Default ACK timeout:** 10 milliseconds
- **Configurable per-endpoint** via `timeout` and `ack_timeout` in the API definition

For GET requests with `InfiniteDuration` timeout (e.g., event wait), if the write timeout is reached before a result, the API returns `202 Accepted` with partial results and a UUID. The client can re-request with the same parameters to get cached results.

---

## Error Handling

| Status Code | Meaning | Description |
|---|---|---|
| `401` | Unauthorized | Authentication failed (all providers returned errors) |
| `403` | Forbidden | Authorization denied by Casbin or entitlement check |
| `404` | Not Found | The requested object/session was not found (no ACK received) |
| `500` | Internal Server Error | Unexpected error during processing |

**Error response format:**
```json
{
  "error": "descriptive error message"
}
```

**Auth error format (401):**
```json
{
  "errors": {
    "auth-saml": "SAMLResponse not provided",
    "auth-github": "token expired"
  }
}
```

---

## Examples

### cURL: Create a New Conversation

```bash
curl -X POST "https://honeydipper.example.com/api/convos" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "agent": "my_assistant",
    "text": "Check the status of the production deployment."
  }'
```

Response:
```json
{
  "convo_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### cURL: Get Conversation History

```bash
curl -H "Authorization: Bearer $JWT_TOKEN" \
  "https://honeydipper.example.com/api/convos/550e8400-e29b-41d4-a716-446655440000/history"
```

### cURL: List Agents

```bash
curl -H "Authorization: Bearer $JWT_TOKEN" \
  "https://honeydipper.example.com/api/agents"
```

### cURL: Add a Message to a Conversation

```bash
curl -X POST "https://honeydipper.example.com/api/convos/550e8400-e29b-41d4-a716-446655440000/turn" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "text": "What about the database migration?"
  }'
```

### cURL: Rerun an Event

```bash
curl -X POST "https://honeydipper.example.com/api/events/abc123/rerun" \
  -H "Authorization: Bearer $JWT_TOKEN"
```

### cURL: Wait for Event Completion

```bash
curl "https://honeydipper.example.com/api/events/abc123/wait" \
  -H "Authorization: Bearer $JWT_TOKEN"
```

### cURL: Wait for Event (Polling with UUID)

If the event hasn't completed, you'll get a `202` with a UUID:

```json
{
  "uuid": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "results": {}
}
```

Then re-request with the UUID (same endpoint + cached results).

### cURL: Check Health

```bash
curl -I "https://honeydipper.example.com/healthz"
```

### Workflow API Call Example

From within a Honeydipper workflow, you can call the API indirectly through the engine:

```yaml
workflows:
  check_external:
    steps:
      - call_api:
          call: "events/some_event_id/wait"
```

The API is also used by the Honeydipper UI for:
- Agent conversation management (list, create, turn, cancel)
- Event monitoring and interaction
- Pod log streaming
- Secret management
