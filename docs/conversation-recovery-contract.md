# Conversation Recovery Contract (Phase 1 — research, pin, regression)

This document is the Phase 1 deliverable for recovering an agent conversation
whose `convo_state:<id>` Redis key has been reclaimed (TTL/eviction). It records
every call site of `StartTurn`, `StartInference`, and `StartNewConvo` across the
daemon (`honeydipper/honeydipper`, branch `v4`) and the UI (`Charles546/hd-ui`,
branch `main`), pins the recovery contract, and points at the regression test
that encodes today's broken behavior.

> Phase 1 only researches, pins the contract, and adds a regression test. No
> recovery logic is implemented here. Phases 2, 3, and 5 implement the fix.

## Background

When a conversation sits idle long enough, its `convo_state:<id>` key is
reclaimed. Reopening it hits `StartTurn` (`internal/agent/agent_store.go`), which
resolves the agent name from `ConvoState.LastSession`/`FirstSession`. When both
are empty it logs `cannot determine agent name for convo ...` and returns
silently. The API handler `handleConvoTurnAPI` (`internal/service/agent_apis.go`)
still returns `{"ok":true}`, so the UI sees a false success: no turn starts, no
message is appended to `convo_history`, and `ConvoState.ActiveSession` is never
set.

## Call-site map — daemon (`honeydipper/honeydipper`, `v4`)

### `StartTurn(convoID, text, user, engine, driver string)`
- **Definition / interface**: `internal/agent/agent_store.go:28` (interface `AgentStore`), impl `internal/agent/agent_store.go:237`.
- **Primary revive path (UI turn API)**: `internal/service/agent_apis.go:141` — `handleConvoTurnAPI` calls `agentStore.StartTurn(convoID, body.Text, user, body.Engine, body.Driver)`.
- **Route**: `POST /convos/:convoID/turn` registered in `internal/api/def.go:70` as `Name: "convoTurn"` (`AttachPrincipalUser: true`, `Service: "agent"`).

### `StartInference(msg *dipper.Message)`
- **Definition / interface**: `internal/agent/agent_store.go:18` (interface), impl `internal/agent/agent_store.go:102`.
- **Call site**: `internal/service/agent_svc.go:132` — `handleAgentStart` (responder for `eventbus:agent_start`) calls `go agentStore.StartInference(msg)`. The agent name rides in `msg.Labels["agent_name"]` (set by `initNewSession`).
- **Route / trigger**: registered responder `eventbus:agent_start` in `internal/service/agent_svc.go` (`StartAgent`). This is the workflow/orchestrated inference path, not the UI turn path.

### `StartNewConvo(agentName, text, user, engine, driver string) string`
- **Definition / interface**: `internal/agent/agent_store.go:32` (interface), impl `internal/agent/agent_store.go:272`.
- **Call site**: `internal/service/agent_apis.go:164` — `handleConvoNewAPI` calls `agentStore.StartNewConvo(body.Agent, body.Text, user, body.Engine, body.Driver)` and returns the generated `convo_id` synchronously.
- **Route**: `POST /convos` registered in `internal/api/def.go:53` as `Name: "convoNew"` (`AttachPrincipalUser: true`, `Service: "agent"`). Already agent-driven; Phase 2 reuses its `ConvoState` construction for recovery.

## Call-site map — UI (`Charles546/hd-ui`, `main`)

### Client functions in `src/api.js`
- `startTurn(creds, convoID, text, engine, driver)` → `POST /convos/:convoID/turn` (daemon `convoTurn` API). `src/api.js:313`.
- `startNewConvo(creds, agentName, text, engine, driver)` → `POST /convos` (daemon `convoNew` API). `src/api.js:322`.

### Callers of those client functions
- `src/components/ConvoHistoryPage.jsx:394` — `handleSendTurn` calls `startTurn(creds, convoId, text, finalEngine, finalDriver)` (turn on an open conversation).
- `src/components/ConversationsPage.jsx:657` — calls `startTurn(creds, selectedID, text, finalEngine, finalDriver)` (turn on an existing conversation).
- `src/components/ConversationsPage.jsx:671` — calls `startNewConvo(creds, agent, text, engine, driver)` (brand-new conversation).
- `src/components/NewConvoInput.jsx` — `handleSubmit` invokes `onSend(selectedAgent, trimmed, engine, driver)`; the parent (`ConversationsPage`) wires `onSend` to a handler that calls `startNewConvo`.
- Tests: `src/components/ConvoHistoryPage.test.jsx`, `src/api.test.js` mock `startTurn`/`startNewConvo`.

## Recovery contract

### Request — `POST /convos/:convoID/turn`
Body gains two optional fields (scaffolded in `handleConvoTurnAPI`, not yet
consulted):

| Field | Type | Default | Meaning |
|---|---|---|---|
| `agent` | string | `""` | Agent to use when recreating a reclaimed conversation. |
| `agent_override` | bool | `false` | When `true`, allow `StartTurn` to recreate/overwrite the `ConvoState` even when one is present. When `false`, `StartTurn` must stick to the existing `ConvoState` and never overwrite it. |

### Response
- **Success** → `{"ok":true}` (unchanged).
- **Unrecoverable** (cs missing and no agent supplied) → HTTP `409` with body
  `{"ok":false,"error":"conversation_expired","message":"select an agent to continue"}`
  (the `ConversationExpiredResponse` const in `internal/service/agent_apis.go`).

### Truth table (recorded for Phase 2; NOT implemented in Phase 1)

| cs present? | agent supplied? | agent_override | behavior |
|---|---|---|---|
| no | no | n/a | `409 conversation_expired` (UI prompts for agent) |
| no | yes | false | recreate cs with supplied agent (nothing to stick to) |
| no | yes | true | recreate cs with supplied agent |
| yes | no | n/a | use existing cs (normal turn) |
| yes | yes | false (stick) | use existing cs; do not overwrite |
| yes | yes | true (override) | recreate/overwrite cs with supplied agent |

## Regression test (Phase 1)

- `internal/agent/agent_store_test.go::TestStartTurn_ConvoStateEvicted_RegressesToSilentNoOp`
  seeds a conversation, evicts `convo_state:<id>` via `cache:del`, drives
  `StartTurn`, and asserts all four symptoms of the broken behavior:
  1. the `cannot determine agent name for convo <id>` log line is emitted;
  2. `convo_state:<id>` is NOT recreated (no `cache:save`);
  3. `convo_history:<id>` is NOT appended;
  4. no message is emitted to the AI driver / `agent_response`.
- `internal/service/agent_apis_test.go::TestHandleConvoTurnAPI_EvictedConvoReturnsOkTrue`
  drives the broken path through `handleConvoTurnAPI` and asserts the
  false-success `{"ok":true}` response, and that the convo state is not
  recreated afterward.

Both tests encode today's defect and are expected to be **inverted** by Phase 2's
recovery fix.
