# Redis Persistence State Audit (Phase 2 — stale state and duplicate execution)

Phase 1 made the deployment's Redis persistent: the `hd-redis-data` named volume
mounted at `/data`, `--appendonly yes --appendfsync everysec` (AOF) plus the
default RDB snapshots, and `restart: unless-stopped` (deployment-side details and
the backup/restore runbook live in `Charles546/hd-home`, `deploy/README.md`).
Previously Redis was effectively ephemeral, so **a Redis restart was an implicit
clean slate**.

This document answers the follow-up question: *when the daemon boots against a
non-empty Redis, is there harmful stale session/workflow/scheduler state, orphaned
state, or duplicate execution?* It is the Phase 2 deliverable and, like
[Conversation Recovery Contract](./conversation-recovery-contract.md), it records
research plus evidence rather than shipping behavior.

**Verdict: HARMLESS — no daemon code change.** The only residual is a slow,
unbounded Redis memory leak (plus a skewed gauge) when an *in-flight* session is
lost to a hard crash; that is pre-existing behavior surfaced by persistence, not
introduced by it, and is deferred to future work (§6).

## TL;DR

| Question | Answer | Evidence |
|---|---|---|
| Can a persisted in-flight session be double-executed? | **No** | `BLPop` pop is destructive; duplicate delivery rejected by the cursor gate (§2.2) |
| Can the scheduler re-fire / double-fire? | **No** | `ZRANGEBYSCORE`+`ZREM` in one atomic `EVAL` (§2.4) |
| Can a booted daemon resume *wrong* work, or notify a wrong session? | **No** | Resume is message-driven only; no startup enumeration anywhere; cursor must match (§2.1) |
| Can a persisted session get permanently stuck / hang? | **No** | `wait_agent` arms `timeout_message`; every lock is `SETNX`+TTL (§2.5) |
| Do persisted locks deadlock? | **No** | All locks carry an expiry (§1.9) |
| Do ID counters continue instead of restarting at 0? | **Yes — this is the benefit** | Empirical transcript in §3 |
| Anything else left behind? | Dead keys only, never read | §4 |

## Method

Code reading against `dev`, plus a local `redis-server` 8.8.0 (AOF enabled) used
to execute **the repository's own Lua scripts and command sequences** to confirm
delivery/TTL semantics. No access to a deployment host was needed or used.

This is a documentation-only change: no Go source, no test, and no behavior is
modified. `go build ./...`, `go test ./...` and `go vet ./...` are green. The
only `golangci-lint run ./...` findings are two `typecheck` errors in generated
gomock files (`*.go` under `mock_dipper` / `mock_api`) caused by the local
linter's export-data reader being older than the Go 1.27 toolchain; they
reproduce identically on a pristine `dev` checkout and are unrelated to this
document.

## 1. Redis consumer inventory

Every component that touches Redis, its key families, and whether it cleans up.

| Component | Key families | TTL | Startup cleanup | Risk |
|---|---|---|---|---|
| `drivers/cmd/redisqueue` | `honeydipper:events`, `:commands`, `:return`, `:agent_start`, `:agent_continue`, `:agent_command`, `:agent_workflow`, `:agent_response`, `:agent_recover`, `:agent_poll`, `:api:<ip>` | `TopicExpireTimeout = 1800s`, re-applied with `EXPIRE` on every `RPush` (`main.go:27`, `main.go:224-225`) | none needed | low |
| `drivers/cmd/redispubsub` | none — `PUBLISH` / `PSUBSCRIBE` only | n/a | n/a | none |
| `drivers/cmd/redis-cache` | generic RPC surface (`save`, `load`, `rpush`, `lrange`, `ltrim`, `del`, `exists`, `scan`, `expire`, `hset`, `hget`, `hgetall`, `hvals`, `hmget`, `stream_hset`, `stream_hvals`, `incr`, `decr`, `blpop`) | decided by the caller (§1.3) | none | low |
| `drivers/cmd/redislock` | `lock:<name>`, `<prefix><name>:seq`, **`hd_unique_id:<name>`** | locks: per-call expiry; `seq`: `seq_expire`, default 24h; `hd_unique_id`: **no TTL by design** (`main.go:190-205`) | none needed (self-healing) | low, plus the §3 benefit |
| `drivers/cmd/hd-driver-redis-scheduler` | `honeydipper:schedule` (ZSET), `honeydipper:schedule:payload:<type>:<key>` | **none** (`main.go:162-169`) | destructive `ZREM` only | low–medium (§2.4) |
| `pkg/workflow/persisted_store.go` (+ `store.go`) | `stack:<rootID>` (LIST), `event:<eventID>`, `session_stream_*`, `counter:session`, `counter:session:<storeIP>` | done session: 24h; live session: **none**; `event:` live: **none**, done: 48h; stream: 48h; counters: **none** | **none** | low–medium (§2, §4a) |
| `internal/agent/agent_store.go`, `convo_state.go` | `agent_session:<id>`, `convo_state:<id>`, `convo_history:<id>[_g<N>]`, `convo_stream_*`, MCP-tool cache | 336h / 720h / 720h / 720h / 6h | none | none |
| `internal/api` entitlement cache | `l.entitlementCacheKey(...)` (`internal/api/store.go:588`) | 30m | none | none |
| `pkg/workflow/cache.go` workflow cache | `workflow-cache/<key>` via `save` / `hset` | 24h default, or `cache_ttl` | none | none |

Details worth calling out:

### 1.1 Message queues (`redisqueue`)

Topics are Redis **lists** with a per-key expiry, not per-message. `RPush`
re-arms `EXPIRE 1800s` on every write (`main.go:224-225`), so an idle topic
disappears 30 minutes after its last write. Because the TTL is per key rather
than per message, a never-consumed backlog entry can outlive 1800s under
sustained traffic on the same topic — that is a delivery-latency property, not a
persistence regression.

Delivery semantics after a restart (see §2.3): a message that was never consumed
is still in the list and can be delivered again after the restart (legitimate
at-least-once redelivery), while a message already consumed by `BLPop` is gone
for good. No already-consumed job can be duplicated.

### 1.2 Pub/Sub (`redispubsub`)

No keys at all. `PUBLISH`/`PSUBSCRIBE` messages that no one was listening for are
dropped, exactly as before Phase 1. Nothing to persist, nothing to clean up.

### 1.3 Cache RPC (`redis-cache`)

A generic pass-through; **TTL is a caller decision**, and the callers are all
accounted for above:

* `save` → `SET <key> <value>` plus `EXPIRE` when the caller passed `ttl`;
* `rpush` → `EXPIRE` only when `ttl > 0`;
* `hset` → default 24h, `stream_hset` → default 336h with 2h blocks;
* `blpop` → the queue consumers.

`redis-cache` also exposes a `scan` RPC (`main.go:80`) that currently has **zero
callers** — noteworthy only because it is the hook a future reconciliation would
use (§4d, §6).

### 1.4 Locks and ID counters (`redislock`)

* `lock:<name>` — `SETNX` plus expiry; `unlock` deletes. Ownership is optional.
* `<prefix><name>:seq` — `SETNX`, `seq_expire` (24h default).
* `hd_unique_id:<name>` — `INCRBY` in a Lua script with a wrap check
  (`main.go:190-205`). Deliberately **no TTL**: this is the counter that must
  survive a restart (§3).

### 1.5 Scheduler (`hd-driver-redis-scheduler`)

* `honeydipper:schedule` — ZSET of `<type>:<key>` members scored by due time.
* `honeydipper:schedule:payload:<type>:<key>` — the serialized due message to
  deliver when the member becomes due, written with score/payload handoff and
  **no TTL** (`main.go:162-169`).
* `watch()` pops due members atomically (§2.4) and `cancelSchedule` removes a
  member with `ZREM` (`main.go:185`), so cancel and fire are mutually exclusive.

### 1.6 Sessions (`pkg/workflow/persisted_store.go`)

* `stack:<rootID>` — a LIST of marshalled `*Session` frames, one per stack depth.
  `persist()` does `DEL` then one `RPUSH` per frame (`:938-940`, `:962-966`); the
  TTL is `0` (never) unless the root session is done, in which case 24h
  (`:943-945`).
* `event:<eventID>` — event ID → root session ID. Live: no TTL (`:991-994`);
  done: 48h (`:1003-1008`).
* `counter:session` / `counter:session:<storeIP>` — live session counters,
  `INCR` on session creation (`:115-116`) and `DECR` on completion
  (`:1001-1002`); no TTL (the per-instance variant is removed when it hits zero).
* `session_stream_*` — 48h.

### 1.7 Agent state (`internal/agent`)

`agent_session:<id>` (336h), `convo_state:<id>` (720h), `convo_history:<id>[_g<N>]`
(720h), `convo_stream_*` (720h) and the MCP tool cache (6h). Everything is
TTL'd, so nothing here can accumulate across restarts. The concurrency guards are
locks with expiries (§1.9).

### 1.8 API and workflow caches

`internal/api` entitlements are cached for 30 minutes
(`DefaultEntitlementCacheTTL`, `internal/api/store.go:44-45`; the key helper
`entitlementCacheKey` is defined at `:588` and used at `:538`);
workflow `save`/`hset` cache entries live 24h unless a workflow sets
`cache_ttl`. Both are pure caches: a stale hit after a restart is bounded by the
TTL and re-derivable from configuration.

### 1.9 Every lock is `SETNX` + TTL (nothing can deadlock)

| Lock | Expiry | Location |
|---|---|---|
| Session key (persist/continue) | 3600s | `pkg/workflow/persisted_store.go:359`, `:986`, `:1017` |
| Convo state update | 30s | `internal/agent/convo_state.go:283` |
| Agent turn lock | `TurnLockTimeout` (call timeout 30m) | `internal/agent/agent_session.go:165` |
| Agent session (register/poll) | 600s | `internal/agent/agent_session.go:364`, `:419` |
| API response candidacy | milliseconds | `internal/api/response.go:80` |

A lock that survives a crash therefore self-heals on expiry rather than pinning a
session forever.

### 1.10 Exactly two non-TTL'd families

1. `hd_unique_id:*` — intentional and desirable (§3).
2. The scheduler ZSET and its payload keys (§2.4, §4b).

Everything else either has an expiry or is removed by its consumer.

## 2. Risk analysis

### 2.1 Structural fact: there is no boot-time reconciliation

Nothing enumerates or resets state at startup. The `redis-cache` `scan` RPC has
zero callers, nothing walks `stack:*`, `event:*` or `counter:*`, and there is no
`time.Ticker`/`time.AfterFunc`/timer in `internal/service`, `pkg/workflow` or
`internal/daemon` that revives sessions. **Resumption is purely message-driven:**
a session advances only when a message for it arrives.

Every such arrival passes one gate: `loadSession()` (`persisted_store.go:1017`)
loads the persisted stack and compares the message cursor against the tail
frame's cursor — `session ... ignoring mismatched cursor` (`:1046-1056`) — and
returns `nil` on a mismatch, after taking a 3600s session lock. That cursor is
the system's **idempotency guard** for every durable replay path, and it is
covered by `Test_loadSession_mismatch_and_happy` and
`TestPauseResumeCancelSessionByID` (`pkg/workflow`).

### 2.2 Restart scenario matrix

| Scenario | What happens | Verdict |
|---|---|---|
| Daemon-only restart (Redis stays up) | Identical to pre-Phase-1 behavior: the queue and ZSET already lived in Redis | unchanged |
| Both restart, session was in `wait:` / `wait_agent` | The ZSET survives; due items fire at boot via `scheduler:session` → `engine.addResponder("scheduler:session", continueSession)` (`internal/service/engine.go:63`); cursor matches, so the session resumes at the correct frame. A window that elapsed during downtime resumes immediately — correct timer semantics | resumes sanely |
| Both restart, in-flight driver call | The `eventbus:command` may still be in `honeydipper:commands`: it runs once and its return resumes the session on the persisted cursor. If it had already been popped and was lost with the crashing process, nothing resumes the session | resumed sanely, or orphaned residue (§4a) |
| Graceful stop (`SIGTERM`) | `retryInterruptedSession` (`internal/service/operator.go:258`, called from `:187`) re-queues the interrupted command with `interrupted=true`. With persistence that re-queued job also survives a simultaneous Redis restart, where before Phase 1 it was lost | designed resume path, consumed exactly once |
| Hard crash of an in-flight session | `stack:`/`event:`/`counter:session` remain, unread (§4a) | dead keys only |

### 2.3 Queue delivery: at-most-once consumption, at-least-once for unconsumed work

* The pop is `BLPOP`, which is **destructive** — verified against a real Redis:
  after `BLPOP`, `LRANGE` no longer contains the element. This is the key
  property that keeps a persisted queue from re-running a job.
* A message that was **never consumed** by any consumer (for example, the daemon
  was down when it was enqueued) is still in the list and **is** delivered after
  the restart. That is legitimate at-least-once redelivery of work that has not
  happened yet, bounded by the 30-minute per-topic TTL refresh.
* A message **already consumed** is never delivered again; a duplicate of an
  already-processed return is rejected by the cursor gate (§2.1).
* Symmetrically, a message **popped and then lost** with a consumer that crashed
  mid-processing is gone — at-most-once. That loss window exists with or without
  persistence and is unchanged by this phase.

### 2.4 Scheduler: at-most-once per tick, no re-fire

`watch()` performs `ZRANGEBYSCORE` and `ZREM` inside a single `EVAL`
(`main.go:80-88`), so the set of due members is removed exactly once. Verified
empirically: a second tick immediately afterwards returns an empty set, i.e. **no
re-fire and no double-fire**, even if the daemon was down for a long time and
several members are due at boot (they are delivered in one bounded burst, each
one exactly once). Cancel and fire are mutually exclusive because both go through
`ZREM` (`main.go:185`).

The cost of that design is a narrow crash window in each direction, described in
§4b.

### 2.5 Why nothing can be stuck or hang

Time-bound operations always arm a timeout: `wait_agent` registers
`timeout_message` (`pkg/workflow/execute.go:394-411`, consumed at `:218`), so the worst case after a
restart is a deterministic `poll timeout (engine)` failure status rather than an
indefinite wait. Combined with TTL'd locks (§1.9) and the fact that a killed
in-flight message is simply never replayed (§2.3), a resumed session either
continues or fails visibly — it does not hang.

## 3. The `hd_unique_id` benefit (verified)

Phase 1 fixes ID reuse. Previously every Redis restart reset the counters, so a
new session could eventually reuse the ID of a persisted key; now IDs continue
monotonically. Verified by running the repository's own `INCRBY`+wrap Lua script
(`drivers/cmd/redislock/main.go:190-205`) against a Redis with AOF enabled:

```
boot1, batch 1        -> 100
TTL hd_unique_id:session -> -1 (no expiry)
boot1, batch 2        -> 200
# redis shutdown, then restart on the same /data
GET hd_unique_id:session -> 200          # not reset
next batch            -> 300             # monotonic continuation
# pre-Phase-1 equivalent (stateless redis):
FLUSHALL, then next batch -> 100         # the old reset behavior
```

With `wrap = 10e9` and `batch = 100` (`pkg/workflow/store.go:31-33`) and
`nextID = maxID - batch + 1` (`persisted_store.go:1105-1112`, assigned at `:1111`), a freshly allocated
session ID can no longer collide with a `stack:` key that predates the restart.

## 4. Known limitations

None of these is introduced by persistence; each is either a pre-existing window
in the daemon's own logic or a consequence of keeping state that used to be
thrown away. They are recorded here so operators are not surprised, and none of
them affects correctness of resumption, delivery, or scheduling.

### (a) Pre-existing, not introduced by persistence: TTL-less residue after a hard crash

If a process dies **hard** (`SIGKILL`, OOM, power loss) while a session is in
flight *and* the step it was waiting on was already consumed (so nothing will
ever resume it, §2.3), its keys are never cleaned up:

* `stack:<rootID>` — the 24h TTL only applies once the root session is done
  (`persisted_store.go:943-945`), so a live stack is written with no expiry
  (`:962-966`);
* `event:<eventID>` — no TTL while live (`:991-994`);
* `counter:session` and `counter:session:<storeIP>` — incremented on creation
  (`:115-116`) and only decremented on completion (`:1001-1002`);
* the scheduler payload keys (§4b).

Nothing reads these keys at boot (§2.1), so they cannot revive work, block a
session, or cause a duplicate run. The effect is a slow, unbounded growth of the
Redis dataset, proportional to the number of hard crashes of in-flight sessions
multiplied by the size of a marshalled session stack.

One caveat on visibility: `counter:session` is not entirely invisible. It feeds
the telemetry gauge `honey.honeydipper.engine.sessions`
(`internal/service/engine.go:197`, reading `GetNumSessions` at
`persisted_store.go:1144-1158`), so after a crash the "live sessions" metric is
skewed until the counter is cleaned. It is **telemetry only — not used for
admission control, throttling, or scheduling decisions** (the only other
references are test doubles). Note the gauge reads the per-instance variant
keyed by `dipper.GetIP()` (`pkg/dipper/netutil.go:15`), which usually changes on
redeploy, so in practice the gauge self-resets.

Related and of the same class: a `convo_state:<id>` may keep
`ActiveSession` (`internal/agent/convo_state.go:65`) pointing at an agent session
whose backing stack is gone. That is cosmetic UI residue (a conversation appears
to have an active session that will never progress); it expires with the 720h
`convo_state` TTL and starts no work.

### (b) Pre-existing, not introduced by persistence: scheduler crash windows

`scheduleOnce` writes the payload (`main.go:162-165`) and *then* adds the ZSET
member (`main.go:166-169`), and `watch()` removes the member (`:82`) and then
reads and deletes the payload in a second `EVAL`. Both orderings leave a narrow
window:

| Window | Effect |
|---|---|
| Crash between payload `SET` and `ZADD` | A payload key with no ZSET member: the schedule never fires and the key is never deleted (no TTL) |
| Crash between `ZREM` and payload `GET`+`DEL` / `SendMessage` | The fire is lost, and the payload key leaks |

The first is an orphaned payload (silent non-delivery of a schedule that was
being created); the second loses one already-due fire. Both are pre-existing and
neither can cause a duplicate delivery. A stale payload that does get delivered
is harmless: the message's cursor no longer matches and is ignored (§2.1).

### (c) Pre-existing, not introduced by persistence: `persist()` is not atomic

`persist()` issues `DEL stack:<id>` (`:938-940`) and then one `RPUSH` per frame
(`:962-966`) as separate calls. A crash in between leaves the session with no
stack at all. AOF replay does **not** rescue this — the `DEL` is replayed too, so
the outcome is still "that one stack is lost". Persistence neither widens nor
narrows the window; the session is simply unrecoverable in both worlds.

### (d) Pre-existing, not introduced by persistence: unused `cache.scan`

`redis-cache` exposes `scan` (`drivers/cmd/redis-cache/main.go:80`) but nothing
calls it. It is the natural hook for a boot-time or periodic reconciliation, and
is noted here as such; see §6.

## 5. Operator notes

**No action is required at boot.** A daemon starting against a non-empty Redis
needs no migration, no flush, and no warm-up: sessions resume only when a message
for them arrives, and an interrupted session either continues from its persisted
stack or is superseded by a fresh event.

**Boot-time convergence is expected and self-correcting.** If Redis is not yet
accepting connections, the `redisqueue` receiver's `Ping` panics and subscribers
panic-and-retry on `BLPOP`; the container's restart policy converges once Redis
is up. This is documented in the deployment runbook and is unchanged by this
audit.

**Escape hatch: `podman-compose down -v`.** Dropping the `hd-redis-data` volume
returns the deployment to the pre-Phase-1 semantics (empty Redis on next start):
no stale sessions, no orphaned keys, no skewed counters — and no in-flight
sessions or due schedules either. Use it to recover from a state you do not trust,
or before a known-incompatible downgrade; take a quiesced backup first if the
history matters.

**Backups.** Follow the quiesced backup/restore procedure in the deployment
runbook (`deploy/README.md`). A file-level copy of a live AOF is not a
consistent backup.

**Monitoring.** If `honey.honeydipper.engine.sessions` drifts upward without
matching real activity, or the Redis dataset grows steadily on an idle
deployment, treat it as the hard-crash residue of §4a; the remediation today is
`down -v` after a backup.

## 6. Future work (deferred, tracked separately)

Reaping orphaned live-session keys is deliberately **not** part of this phase.
The expected shape of a fix is a **per-daemon lease/epoch-based boot GC**: each
daemon instance stamps the sessions it owns with a lease/epoch, and a booting
instance garbage-collects only keys whose lease has expired, so an orphan can be
distinguished from a session actively owned by another instance. That design can
also repair the skewed `engine.sessions` gauge and the scheduler payload keys in
the same pass.

Options considered and their drawbacks:

| Option | Why not now |
|---|---|
| TTL on live `stack:`/`event:` (e.g. refreshed 7d TTL) | Silently drops legitimately quiet long-running sessions (a `wait:` of weeks); a scheduler fire would then hit "session not found". A behavior change, not a cleanup. |
| Owner-tagged boot scan using the existing `cache.scan` | Requires the ownership/lease concept first; a naive scan cannot distinguish an orphan from a session owned by another live daemon, and would risk destroying live state. |
| Lease/epoch boot GC | The recommended shape: safe under multiple daemons, and fixes the gauge and payload leaks at once. |

Also worth a separate issue, unchanged by persistence: the non-atomic
`DEL`→`RPUSH` in `persist()` (§4c) and the two scheduler handoff windows (§4b),
which could each be collapsed with a single Lua script or `MULTI`/`EXEC`.
