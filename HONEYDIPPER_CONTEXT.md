# Honeydipper v4 — Comprehensive AI Knowledge Base

## 1. Overview

Honeydipper is an event-driven orchestration platform written in Go. It listens for events from various sources (webhooks, pub/sub, schedulers, etc.), matches them against configurable rules, and executes workflows — sequences of function calls, AI agent interactions, sub-workflows, and data transformations.

**Module path**: `github.com/honeydipper/honeydipper/v4`
**Go version**: 1.25.0

---

## 2. Architecture — 4 Services + 1 Daemon

The daemon (`cmd/honeydipper/main.go`) runs a single process that may host **up to 5 services** concurrently:

| Service | Purpose | Entry File |
|---|---|---|
| **Engine** | Event → Workflow router. Matches events against rules, creates/manages workflow sessions. | `internal/service/engine.go` |
| **Receiver** | Discovers event sources from rules, wires driver features, routes events to engine. | `internal/service/receiver.go` |
| **Operator** | Resolves and executes function calls (system.function → driver.action), handles retries/backoff, runs commands. | `internal/service/operator.go` |
| **API** | HTTP REST API (Gin-based): events management, agent conversations, secrets, auth (SAML/GitHub), user profiles, long-poll endpoints. | `internal/api/` |
| **Agent** | AI agent conversations: inference, tool calls, sub-agents, MCP servers, streaming, conversation history. | `internal/agent/` |

**Auxiliary modes**: `configcheck` (validate YAML), `docgen` (generate docs), `job` (run `reserved/main` workflow once then exit).

---

## 3. Core Data Flow

```
External Event (webhook, pubsub, scheduler)
        │
        ▼
  [Receiver Service]  ── Discovers drivers needed from rules
        │                  Routes `eventbus:message` to engine
        ▼
  [Engine Service]    ── Matches event against ruleMap
        │                  Creates Session (workflow instance)
        ▼
  [Workflow Engine]   ── State machine: conditions → cache → loops → steps → actions → exports → done
   (pkg/workflow/)         Executes via eventbus commands
        │
        ▼
  [Operator Service]  ── Resolves system.function → driver.action
        │                  Interpolates parameters, adds decrypt/retry/timeout
        ▼
  [Driver Runtime]    ── Builtin (child process), Remote (downloaded), or Null
   (internal/driver/)      Sends `command:action` to driver binary
        │
        ▼
  [External System]   ── Kubernetes, GCP, Redis, Vault, AI models, etc.
```

**Return path**: Driver emits `eventbus:return` → Operator forwards → Engine `continueSession` → Workflow state machine progresses.

---

## 4. Configuration System

### 4.1 Config Data Model (`internal/config/schema.go`)

```go
type DataSet struct {
    Systems   map[string]System       // named groupings of triggers, functions, data
    Rules     []Rule                  // event → workflow bindings
    Agents    map[string]Agent        // AI agent definitions
    Drivers   map[string]interface{}  // driver metadata/config
    Includes  []string                // glob patterns to more YAML files
    Repos     []RepoInfo              // child git repos
    Workflows map[string]Workflow     // named workflows (e.g. "reserved/main")
    Contexts  map[string]interface{}  // runtime context (enriched during assembly)
}
```

### 4.2 Key Structs

- **Rule**: `{When: Trigger, Do: Workflow}` — binds an event trigger to a workflow
- **Trigger**: `{Driver, RawEvent, Match (if_match), Parameters, Source (Event{System,Trigger}), Export, Description}`
- **Function**: `{Driver, RawAction, Parameters, Target (Action{System,Function}), Export, ExportOnSuccess/Failure}`
- **System**: `{Data, Triggers map, Functions map (can be nested: "subsys.func"), Extends []string}`
- **Workflow**: Rich step definition with conditions (`If/IfAny/Unless/UnlessAll/Match/UnlessMatch`), iteration (`Iterate/IterateParallel`), branching (`Switch/Cases/Default`), error handling (`OnError/OnFailure`), caching (`CacheKey/CacheTTL`), exports, sub-workflows, agent calls
- **Agent**: `{Driver, Engine, SystemPrompt, InferencePrompt, ShouldEmitThoughts, ShouldStream, MaxHistoryLen, CompactionPolicy, Tools, ModelData}`
- **RepoInfo**: `{Repo (URL/path), Branch, Path, InitFile, KeyFile (SSH), TokenSource (github), Username/PassEnv}`

### 4.3 Config Lifecycle (5 Stages)

```
StageLoading (0) → StageBooting (1) → StageDiscovering (2) → StageServing (3) → StageDrained (4)
```

- **StageLoading**: Initial repo bootstrap
- **StageBooting**: Load required features (essential drivers like `eventbus`)
- **StageDiscovering**: Load dynamic drivers (discovered from rules/systems), apply decryption, extend systems
- **StageServing**: Apply RegexParser, set DataSet active, call ServiceReload
- **StageDrained**: After graceful shutdown

Stage advancement is synchronized across all services via `Config.StageWG` (per-stage WaitGroups).

### 4.4 Config Assembly (`internal/config/repo.go`)

1. **Clone/load repos** — git clone (go-git) or use local path. Handles SSH keys, GitHub App tokens, HTTP basic auth
2. **Load YAML files** — Go template interpolation (`{% %}` delimiters) → YAML unmarshal → `DataSet`
3. **Recursive repos** — Child repos loaded via `Repos` field
4. **Includes** — Glob patterns resolved relative to repo root
5. **Merge** — `mergeDataSet()` across all repos depth-first. Systems merge specially (extend via `Extends` field)
6. **Built-in contexts** — `_loaded.*.repo_matcher`, `_loaded.*.loaded_repos` injected automatically
7. **Refresh** — Every 60s, `git pull` on loaded repos (hard reset if unstaged), re-assemble, call `OnChange` (triggers `reload()`)

### 4.5 Git Operations (`internal/config/githelper.go`)

- Pure Go via `go-git` library (no git CLI needed)
- Auth methods: SSH key (`DIPPER_SSH_KEY` / `SSH_AUTH_SOCK` / file), GitHub App token (`GH_APP_*` env vars), HTTP Basic (env var passwords)
- Error recovery: `ErrUnstagedChanges` → hard reset → re-pull
- Non-fatal per-file errors caught and collected (config continues loading other files)

---

## 5. Workflow Engine (`pkg/workflow/`)

### 5.1 State Machine — ~30 States

```
Init → CheckCondition → [Else|CheckCache|CheckLoop|CheckIteration]
  → FirstRound/NextRound → CheckIteration → [FirstItem/NextItem|CheckSteps]
  → FirstStep/NextStep → CheckAction → FirstAction/Action
  → Update → CheckEndSteps → EndStep → CheckEndIterations
  → EndItem → CheckEndRounds → EndRound → Export
  → [Failure|Error|SaveCache|Success] → Done
```

### 5.2 Conditions (`condition.go`)

- **If** — ALL conditions truthy (AND)
- **IfAny** — ANY condition truthy (OR)
- **Unless** — ALL conditions falsy (NOT AND)
- **UnlessAll** — At least one falsy (NOT OR)
- **Match / UnlessMatch** — Deep comparison via `dipper.CompareAll`
- **While / Until / WhileMatch / UntilMatch** — Loop continuation conditions

### 5.3 Actions (`execute.go`)

| Action | Description |
|---|---|
| `IterateParallel` | Concurrent iterations (pool size default 100) |
| `Workflow` | Call a named workflow (sub-workflow) |
| `Function` | Inline function definition |
| `CallDriver` | Raw `driver.action` call |
| `CallFunction` | Shorthand `system.function` string |
| `CallAgent` | Start AI agent inference |
| `WaitAgent` | Wait for agent response with resume key |
| `SendEvent` | Emit a new event |
| `Steps` | Sequential sub-steps |
| `Threads` | Parallel threads (async children) |
| `Wait` | Wait for threads to complete |
| `Switch/Cases/Default` | Branching |
| `Resume` | Resume a pending session |
| `Detach` | Fire-and-forget |

### 5.4 Data Export (`export.go`)

- `Export` (always), `ExportOnSuccess`, `ExportOnFailure`, `ExportOnError`
- `NoExport` — block specific keys from export (`"*"` = block all)
- Special suffix `*` in export keys marks cache data

### 5.5 Hooks (`hooks.go`)

Lifecycle callbacks at specific state transitions:

| Hook | Fires At |
|---|---|
| `on_session` | `CheckCondition` (entry) |
| `on_first_round` / `on_round` | Loop start/iteration |
| `on_first_item` / `on_item` | Per-iteration item |
| `on_first_action` | Before first action |
| `on_update` | After update (exit) |
| `on_failure` / `on_error` / `on_success` | Termination states (exit) |

### 5.6 Caching (`cache.go`)

- In-memory TTL cache (max 1h) + `cache` driver (Redis) fallback
- Key: `"workflow-cache/<cache_key>"`
- `force-cache-refresh` context flag bypasses cache
- Default TTL: 24h

### 5.7 Session Store (`store.go`)

`Store` interface:
- `StartSession(wf, msg, ctx)` — Start a predefined workflow
- `StartDynamicSession(spec, ctx)` — Start from payload
- `CreateChildSession(parent, wf, msg)` — Synchronous child
- `CreateAsyncChildSession(parent, wf, msg)` — Detached async child
- `ContinueSession(id, msg, child)` — Resume with new data
- `ResumeSession(key, msg)` — Resume a waiting session
- `ActivateSession(w)` — Progress a session
- `EmitResult(w)` — Persist result

Session ID counter wraps at 10B, batch = 100.

---

## 6. Agent System (`internal/agent/`)

### 6.1 Architecture

```
User/Workflow ──► Agent Service ──► AgentSession ──► AI Driver (OpenAI/Gemini/Ollama)
                        │                                       │
                        │                                       ▼
                        │                               Tool Calls:
                        │                               sys_<sys>__<func>
                        │                               wf__<workflow>
                        │                               ag__<subagent>
                        │                               mcp__<server>__<tool>
                        │                                       │
                        ▼                                       ▼
                  ConvoState                              Results back to AI
                  (persisted to cache)
```

### 6.2 Session Types

- **`inference`**: Single-turn (no conversation history)
- **`chat_turn`**: Multi-turn with history

### 6.3 Key Components

- **AgentSession** (`agent_session.go`): Full session lifecycle — setup, run, stream, tool dispatch, poll, parent notification
- **PersistentAgentStore** (`agent_store.go`): Eventbus-driven interface, `StartInference`, `PollInference`, `ContinueInference`, `StartAgentCall`, `StartMCPCall`, `CancelConvo`, `StartTurn`, `StartNewConvo`
- **ConvoState** (`convo_state.go`): Lock-protected conversation state (redis), tracks sessions, token usage, cancellation, streams for UI
- **Tool Building**: 4 sources — `system` (sys_), `workflow` (wf__), `agent` (ag__), `mcp` (mcp__)
- **Polling**: Busy-wait with 1s sleep, rate-limited, emits `agent_response` with `AgentState` metadata
- **Default TTL**: 72h, timeout: 1h, poll timeout: 9s

### 6.4 Role Constants

`RoleSystem`, `RoleUser`, `RoleAgent`, `RoleTool`, `RoleToolResult`

### 6.5 Agent Types (`pkg/agent/types.go`)

- **Message**: `{Role, User, IsThinking, ToolCalls, ToolResult, Content, IsComplete, InputTokens, OutputTokens}`
- **Tool**: `{Name, Description, Params (JSON Schema)}`
- **ToolCall**: `{FuncName, Params}`
- **CompactionPolicy**: `{Strategy, Threshold, ThresholdType, PreserveRecent, SummarizationAgent, SummarizationPrompt}` — controls when and how conversation history is compacted (summarized) to stay within token/history limits

- **CompactionStrategy**: currently only `"summarize"` — uses an LLM agent to condense older messages while preserving recent ones verbatim
- **State**: `{HistoryLen, TotalTokens, ConvoID}` — reported at end of turn

---

## 7. Driver System

### 7.1 Driver Runtime (`internal/driver/driver.go`)

```go
type Runtime struct {
    Data, DynamicData interface{}   // static + dynamic config
    Feature           string        // e.g. "eventbus", "emitter"
    Handler           Handler       // builtin, remote, or null
    Stream            <-chan *dipper.Message
    Service, State    string
}
```

### 7.2 Handler Types

| Type | Description | Code |
|---|---|---|
| **Builtin** | Spawns driver binary as child process (stdin/stdout pipes) | `internal/driver/builtin.go` |
| **Remote** | Downloads driver binary from URL or registry, verifies SHA256 + Ed25519 sig, spawns as child | `internal/driver/remote.go` |
| **Null** | No-op sink driver | `internal/driver/nulldriver.go` |

### 7.3 Driver Lifecycle State Machine

```
loaded → alive (after Start)
       → draining → drained
       → stopping → completed
```

### 7.4 Remote Driver Source Resolution

1. **Direct URL**: `url` + `sha256` in handlerData → verify SHA → optional Ed25519 signature
2. **Registry**: Fetch `{registryURL}/{driverName}.json` manifest → resolve version/channel → find OS/arch artifact → download + verify
3. **Cache**: `{remotePath}/sha256/{sha}/{fileName}`, directory-based mutex (`30s` timeout)
4. **Package installation**: auto-detect apk/apt/dnf/brew, install `requiredPackages`

### 7.5 Wire Protocol (`pkg/dipper/comm.go`)

Text header + binary labels/payload over stdin/stdout:
```
{channel} {subject} {numLabels} {payloadSize}\n
  label1 {len1}\n{value1}
  ...
  {payload bytes}
```

**Key channels**: `eventbus`, `rpc`, `state`, `command`

---

## 8. RPC System (`pkg/dipper/rpc.go`)

### 8.1 Architecture

```
Caller (RPCCallerBase)                 Provider (RPCProvider)
    │--- "rpc:call" msg ---─► Labels: rpcID, feature, method, caller
    │                                   │-- Lookup handler
    │                                   │-- Execute handler (with timeout)
    │                                   │-- Return via "rpc:return"
    │◄─ "rpc:return" msg ---─ Labels: rpcID, caller, (error: reason)
```

### 8.2 Call Variants

| Method | Serialization | Returns | Use Case |
|---|---|---|---|
| `Call(feature, method, params)` | JSON | Yes | Synchronous RPC |
| `CallNoWait(...)` | JSON | No | Fire-and-forget |
| `CallRaw(...)` | Raw bytes | Yes | Synchronous with raw data |
| `CallRawNoWait(...)` | Raw bytes | No | Fine-grained raw |
| `CallWithMessage(msg)` | Pre-built | Yes | Full control |

### 8.3 Interruptible Handlers

- Support for long-running operations: handler can be marked `interruptible`
- On context cancellation: sends `return/interrupted` message instead of `return`
- Operator re-queues interrupted commands on restart

---

## 9. Interpolation System (`pkg/dipper/interpolation.go`)

### 9.1 Syntax

| Mode | Delimiters | Used In |
|---|---|---|
| `loading` | `{% %}` | YAML file template processing |
| `embedded` / `dollar` | `{{ }}` | Runtime interpolation |
| Dollar syntax | `$path.to.key` | Direct data lookup |

### 9.2 Dollar Syntax

```
$path.to.key        → lookup value, panic if missing
$?path.to.key       → lookup value, return nil if missing
$key1,key2,"default" → try keys, fallback to quoted default
```

### 9.3 Post-Processing

```
:yaml:<string>      → parse as YAML, recursively interpolate
:yaml_safe:<string> → parse as YAML, return as-is
\<prefix>           → strip backslash (escape mechanism)
```

### 9.4 Custom Template Functions

- `return v` — Captures any value (bypasses string rendering)
- `duration s` — Parse duration → `time.Duration`
- `render t, d` — Recursive nested interpolation
- `fromPath key obj` — Path-based map access
- `toYaml v` — Marshal to YAML string
- `cue_validate_error schema, name, content` — CUE validation
- `decrypt` — Injected by operator for secret decryption
- Full Sprig library (`sprig.TxtFuncMap()`)

---

## 10. Encryption/Secrets (`pkg/dipper/encryption.go`)

### 10.1 Syntax

```
ENC[<driver>,<base64_data>][:<printf_pattern>]
LOOKUP[<driver>,<path>][:<printf_pattern>]
```

- `ENC` — Decrypt base64 ciphertext via driver's `decrypt` RPC
- `LOOKUP` — Lookup secret from driver's `lookup` RPC
- Optional `:<pattern>` — Format result (e.g., `:https://%s/api`)
- `deferred` driver name → leave as-is for later resolution
- `?` prefix on path → optional lookup (swallow errors)

### 10.2 Supported Drivers

- Vault, AWS KMS, GCP Secret Manager, etc. (any driver implementing `decrypt`/`lookup` RPC)

---

## 11. HTTP API (`internal/api/`)

### 11.1 Framework

- Gin HTTP engine, Casbin authorization, JWT authentication (HS256)
- Request types: `TypeFirst` (first responder), `TypeAll` (all respond), `TypeMatch` (matching node), `TypeLocal` (in-process)

### 11.2 Auth Flow

1. Anonymous routes → guest principal
2. `Authorization: Bearer <JWT>` → parse `PrincipalClaims`
3. Fallback to auth provider driver (`auth_web_request` RPC) → on success, issue `X-Honeydipper-JWT` header
4. All fail → 401

### 11.2.1 Server-Side Session Engine

Login sessions are enforced server-side via a Redis-backed session store keyed
by a daemon-minted opaque UUID `sid` (stable across token rotation). See
`internal/api/session/`.

- `auth.session.*` daemon config controls the policy: `enabled`, `idleTimeout`,
  `maxLifetime` (optional absolute cap, default off), `tokenGracePeriod`
  (roll-out grace for pre-upgrade sid-less tokens), `cacheTTL`, and `redis`
  connection settings. Env-var overrides follow the `HD_SESSION_*` convention.
- Provider tiers: IAP takes creds directly from the IAP JWT (no session store);
  GitHub/SAML check the session in Redis; auth-simple is also stored so
  logout/revoke and idle tracking work.
- Idle timeout is a provider re-vouch trigger, not a hard logout. An idle
  GitHub session is silently renewed (same sid) when the provider is alive,
  otherwise the user re-authenticates (new sid). An idle SAML/auth-simple
  session requires re-authentication. `maxLifetime`-exceeded is always a hard
  re-auth (never silently re-vouched).
- Sessions are registered eagerly at login: the GitHub OAuth callback reads the
  driver-returned login as `subject` (with a `username` fallback) and registers
  the daemon-minted `sid` immediately; SAML ACS does the same. Unknown-but-valid
  sids are lazily registered as a rollout safety net, anchored to the token's
  real issuance time (`iat`) so the absolute `maxLifetime` cap is measured from
  actual issuance, not first-seen.
- `last_seen` writes are throttled: the short-TTL cache refreshes `last_seen` in
  memory on every request but writes through to Redis at most once per
  `cacheTTL`, bounding write amplification on busy multi-node deployments. The
  per-subject Redis index (`hd-auth-sessions:subject:*`) is pruned of stale sids
  (whose records were GC'd) on revoke-all and via `PruneSubject`, so it does not
  grow without bound.
- Daemon JWTs minted for an authenticated principal have their `exp` bound to
  the session's absolute `maxLifetime` cap (via `SignPrincipalJWTSession`) when
  one is configured, so the JWT exp never disagrees with the sid-based cap.
- `POST /auth/logout` revokes the current sid (or all sessions for the subject
  via `?all=true`), correct cross-node through the shared Redis store.
- All expiry modes set the `X-Honeydipper-Session-Expired` response header
  (exposed via `Access-Control-Expose-Headers`) and return a consistent
  "re-authenticate" 401.

### 11.3 Registered Routes

| Path | Service | Notes |
|---|---|---|
| `/auth/saml/*` | Local | SAML SP login/metadata/ACS callback |
| `/auth/github/callback` | Local | GitHub OAuth |
| `/auth/logout` | Local | Revoke current session (or all for subject) |
| `/user/profile` | Local | Current user profile |
| `/events/*` | Engine | CRUD + long-poll wait |
| `/gh/events/*` | Engine | GitHub-scoped (with entitlement) |
| `/pods/*/log/chunk` | Operator | Pod log streaming |
| `/gh/secrets/*` | Operator | GitHub-scoped secrets |
| `/convos` / `/agents` | Agent | Conversation management |
| `/convos/:convoID/*` | Agent | Cancel, history, add turn |

---

## 12. Service Framework (`internal/service/service.go`)

### 12.1 Service Struct

```go
type Service struct {
    name, daemonID string
    config *config.Config
    driverRuntimes map[string]*driver.Runtime  // feature → runtime
    responders     map[string][]MessageResponder  // "channel:subject" → handlers
    expects        map[string][]ExpectHandler     // one-shot with timeout
    sequences      map[string]chan dipper.Message  // FIFO ordered per key
    Route          func(msg) []RoutedMessage
    DiscoverFeatures func(*config.DataSet) map[string]interface{}
    ServiceReload  func(*config.Config)
    EmitMetrics    func()
    APIs           map[string]func(*api.Response)
    Drain          func()
    healthy        bool
    // ... lifecycle tracking
}
```

### 12.2 Message Processing Pipeline

```
Inbound Message
    │
    ├─ Expect handlers (one-shot, timeout) ─── matched by "channel:subject:driverName"
    │
    ├─ Responders ─── registered for "channel:subject"
    │
    ├─ Transformers ── sequential, can abort (return nil)
    │
    └─ Route() ────► []RoutedMessage ──► sent to target driver runtimes
```

**Two modes**:
- **Async** (default): All stages run in separate goroutines
- **Sequenced** (when `sequence` label present): FIFO per-key via dedicated goroutine (important for agent conversations)

### 12.3 Built-in Responders

| Pattern | Handler |
|---|---|
| `state:cold` | Cold reload driver runtime (replace) |
| `state:completed` | Track stop completion |
| `state:drained` | Track drain completion |
| `rpc:call` | Route RPC to target feature driver |
| `rpc:return` | Return RPC result to caller |
| `broadcast:reload` | Trigger config refresh or force-quit |
| `api:call` | Dispatch to registered API handlers |

### 12.4 Driver Loading

- **Hot reload** (same meta, driver alive): re-send options only
- **Cold reload** (different meta or dead): start new driver, gracefully close old (50ms grace)
- **Crash recovery**: up to 3 retries with 30s backoff
- **Graceful shutdown**: drain (15s) → stop (5s) → shutdown

---

## 13. Key Packages Reference

### 13.1 `pkg/dipper/` — Utilities

| Function | Description |
|---|---|
| `GetMapData(from, path)` | Navigate nested maps with dotted path |
| `Recursive(from, processor)` | Recursively walk data structure |
| `Interpolate(mode, source, data)` | Template interpolation on all strings |
| `InterpolateDollarStr(v, data)` | `$var` lookup |
| `DecryptAll(rpc, from)` | Decrypt all `@crypt:` values |
| `CompareAll(actual, criteria)` | Deep match (map or glob) |
| `MergeMap(dst, src)` | Recursive deep merge |
| `MergeModifier(dst, src)` | Merge with key removal support |
| `MessageCopy(m)` | Gob deep copy of Message |
| `GetLogger(module, verbosity)` | Logger setup |
| `NewUUID()` | UUID generation |

### 13.2 `internal/driver/` — Driver Runtime

- `NewDriver(feature, meta, data, dynamicData)` → `*Runtime`
- `BuiltinDriver` — spawns binary, pipes stdin/stdout, reads messages
- `RemoteDriver` — downloads + verifies + caches + spawns
- `NullDriver` — no-op

### 13.3 `internal/config/` — Config Management

- `Config.Bootstrap(wd)` — entry point
- `Config.Watch()` — periodic refresh loop
- `Config.Refresh()` — check repos for changes → reassemble → reload
- `Config.AdvanceStage(service, stage, ...)` — synchronized multi-service stage advancement
- `Config.RollBack()` — restore last known good config on panic
- `Config.ResolveStagedDriverMeta()` — resolve remote driver metadata

### 13.4 `internal/api/` — HTTP API

- `Store` — Gin engine, auth middleware, Casbin authorization
- `Request` — API request dispatch via eventbus broadcast
- `Response` — ACK/return to API caller
- `jwtutil` — JWT signing/verification

### 13.5 `pkg/workflow/` — Workflow Engine

- `Session` — runtime state for a workflow instance
- `Store` interface — session persistence (Redis-backed)
- State machine: 30+ states with hooks, caching, exports
- Condition evaluation, loop/iteration control

### 13.6 `internal/agent/` — AI Agent

- `AgentSession` — single turn lifecycle
- `PersistentAgentStore` — eventbus-driven interface
- `ConvoState` — shared conversation state (Redis)
- Tool building from systems, workflows, agents, MCP servers

### 13.7 `drivers/pkg/ai/` — AI Driver Framework

- `ChatWrapper` — locking, history, streaming, cancellation
- `Chatter` interface — AI backend abstraction (OpenAI, Gemini, Ollama)
- `ChatContinue` — polling for streamed chunks
- `ChatStop` — cancel active turn
- `ChatListen` — inject user message into active convo
- `toolCallHandler` — executes tool calls via RPC, workflows, or cache fetches

---

## 14. Eventbus Message Types

| Subject | Direction | Purpose |
|---|---|---|
| `eventbus:message` | → Engine | Incoming event from driver |
| `eventbus:command` | → Operator | Function execution request |
| `eventbus:agent_command` | → Operator | Agent tool call execution |
| `eventbus:return` | → Engine | Function result |
| `eventbus:return_interrupted` | → Operator | Interrupted function (re-queue) |
| `eventbus:agent_continue` | → Engine | Agent response for session |
| `eventbus:agent_response` | → Engine | Agent streaming response |
| `eventbus:agent_workflow` | → Engine | Agent-triggered workflow |
| `scheduler:session` | → Engine | Scheduled session tick |
| `api-broadcast` | → All | API request broadcast |
| `rpc:call` / `rpc:return` | Internal | RPC method calls |
| `broadcast:reload` | → All | Config reload signal |
| `api:candidate` | Internal | API lock contention |

---

## 15. Environment Variables

| Env Var | Purpose |
|---|---|
| `REPO` | Bootstrap repo URL |
| `BRANCH` | Bootstrap branch |
| `BOOTSTRAP_PATH` | Path within repo |
| `BOOTSTRAP_FILE` | Custom init file (default: `init.yaml`) |
| `JOB_FILE` | Job mode workflow file |
| `HD_JWT_SIGNING_KEY` | API JWT signing |
| `HD_JWT_ISSUER` | JWT issuer |
| `HONEYDIPPER_DRIVERS_BUILTIN` | Builtin driver search path |
| `HONEYDIPPER_DRIVERS_CACHE` | Remote driver cache path |
| `HONEYDIPPER_REMOTE_REQUIRE_SIGNATURE` | Enforce Ed25519 sig |
| `DIPPER_SSH_KEY` / `DIPPER_SSH_KEYFILE` / `DIPPER_SSH_PASS` | Git SSH |
| `SSH_AUTH_SOCK` | SSH agent socket |
| `GH_APP_ID` / `GH_APP_INSTALLATION_ID` / `GH_APP_KEY` | GitHub App token |
| `DIPPER_GIT_PASS` / `DIPPER_GIT_PASS_{USERNAME}` | Git HTTP auth |
| `REPO_OVERRIDE` / `REPO_OVERRIDE_*` | Repo path overrides |
| `REPO_MANIFEST_*` | Repo manifest overrides |

---

## 16. Key Design Patterns

### 16.1 Message-Based Everything
All components communicate via the `Message` struct over pipes (stdin/stdout for drivers) or in-memory channels (services). The same wire protocol works for both builtin and remote drivers.

### 16.2 Staged Config with Rollback
Config changes go through a 4-stage pipeline with per-stage WaitGroups. On panic during reload, `RollBack()` restores the last known good config atomically.

### 16.3 Hot vs Cold Driver Reload
When config changes but driver metadata is the same, only new options are sent (hot reload). When metadata changes (different driver binary, different driver type), the old driver is gracefully shutdown and a new one is started.

### 16.4 Recursive Function Resolution
`Operator.collapseFunction` recursively resolves `system.function` references down to raw `driver.action`, merging system data with override semantics at each level.

### 16.5 Unified RPC System
The same `RPCCaller`/`RPCProvider` pattern works for inter-service communication (via eventbus) and driver communication (via stdin/stdout pipes), with support for interrupts, timeouts, and fire-and-forget.

### 16.6 Distributed Locking
Used extensively: API lock contention (`api:candidate`), driver cache acquisition (directory-based mutex), agent conversation state (redis lock), conversation turns (distributed lock via `locker` driver).

---

## 17. Important Constants & Defaults

| Constant | Default | Source |
|---|---|---|
| Config check interval | 60s | `internal/config/config.go` |
| Driver retry count | 3 | `internal/service/service.go` |
| Driver retry backoff | 30s | `internal/service/service.go` |
| Driver graceful timeout | 50ms | `internal/service/service.go` |
| Driver ready timeout | 10s | `internal/service/service.go` |
| RPC timeout | 10s | `pkg/dipper/rpc.go` |
| Agent session TTL | 72h | `internal/agent/agent_session.go` |
| Agent session timeout | 1h | `internal/agent/agent_session.go` |
| Agent poll timeout | 9s | `internal/agent/agent_session.go` |
| Cache TTL | 24h | `pkg/workflow/cache.go` |
| In-memory cache max TTL | 1h | `pkg/workflow/cache.go` |
| Session ID batch | 100 | `pkg/workflow/store.go` |
| Session ID max | 10B | `pkg/workflow/store.go` |
| Entitlement cache TTL | 30min | `internal/api/store.go` |
| Iterate pool size | 100 | `pkg/workflow/execute.go` |
| Sequence channel cap | 100 | `internal/service/seq_process.go` |
| Driver message channel | 10 | `internal/driver/driver.go` |

---

## 18. Testing Patterns

- `main_test.go` at root runs integration tests with bootstrap fixtures
- `cmd/honeydipper/` has `integration_test.go`, `eventbus_test.go`, `state_test.go` with test fixtures under `test_fixtures/bootstrap/`
- Mock interfaces used: `mock_api`, `mock_ai`
- `golang/mock` for generated mocks, `h2non/gock.v1` for HTTP mocking
- `stretchr/testify` for assertions
- Test-specific helpers: `DriverWithReader`/`DriverWithWriter` options for driver testing
- Go interfaces like `GitClient`, `TempDirCreator`, `HeadGetter` for git mocking
