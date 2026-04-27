# Agent Service Planning Brief

## Objective

Design and stage the implementation of a new `agent` service in Honeydipper.

The service should provide a durable, policy-governed, event-driven agent runtime that complements existing workflow execution.

It should:

- keep `do` for workflow execution in `engine`
- add `activate` for agent execution in a new `agent` service
- persist `agent_session` and `agent_turn` state for restart safety
- keep AI drivers as provider adapters instead of session authorities
- support workflow tools, provider tools, compaction, dedupe, and policy enforcement

## Desired End State

Honeydipper can:

- activate named agents from rules
- select providers from an agent-scoped allowlist
- persist conversation and turn state durably
- recover safely from interruption or service restart
- use workflows as tools without blocking inside drivers
- apply top-level security policies to agent behavior
- optionally use raw inference for compaction, engagement decisions, and tool selection

## Scope Boundaries

### In Scope

- new `agent` service
- `activate` rule path
- top-level `agents`
- top-level `security_policies`
- persistent `agent_session` and `agent_turn` store
- workflow completion subscription and resume handling
- interruption-aware command replay
- AI driver adapter contract
- optional raw `Infer` capability on AI drivers

### Out Of Scope For First Slice

- complete migration off existing chat wrapper paths
- broad UI changes
- sophisticated multi-agent coordination
- full schema polish before runtime model is stable

## Key Architectural Decisions Already Made

1. `activate` should point to an `agent`, not directly to a provider.
2. `agent` should define system prompt, provider policy, tool strategy, compaction, dedupe, and history identity.
3. `security_policies` should be top-level and reusable.
4. The `agent` service should own session authority and persistence.
5. AI drivers should expose a narrow provider adapter contract.
6. Workflow completions should be consumed via event subscription rather than blocking wait inside drivers.
7. Interruptible command replay should tell handlers explicitly that the command is resumed after interruption.
8. AI drivers should optionally expose stateless raw inference for orchestration tasks.

## Main Risks

### Runtime Complexity

The hard part is not schema naming. The hard part is making restart-safe agent execution coherent across streaming, tools, workflow completions, and scheduler wake-ups.

### Duplicate Work On Resume

Interrupted and replayed commands can create duplicate streams or side effects unless turn identity and replay semantics are explicit.

### Provider Coupling

If agent state leaks back into drivers, the architecture will regress toward the current wrapper model.

### Policy Ambiguity

Precedence between `security_policies`, `agents`, and `activate` must be defined clearly or the model will be hard to reason about.

## Recommended Planning Order

1. Define runtime boundaries between `engine`, `agent`, and AI drivers.
2. Define the persisted `agent_session` and `agent_turn` model.
3. Define the agent session state machine.
4. Define resume triggers and recovery logic.
5. Define workflow-tool completion correlation.
6. Define context identity, dedupe, and compaction behavior.
7. Define policy enforcement points.
8. Only then design the final config schema.

## Deliverables Expected From Planning

### Runtime Model

- `agent_session` fields
- `agent_turn` fields
- session and turn states
- store indexes and correlation keys

### Service Interfaces

- how `activate` enters the agent service
- how workflow completion events resume an agent session
- how scheduler events resume an agent session
- how user continuation resumes an agent session

### Driver Contract

Required:

- `StartTurn`
- `ContinueTurnWithToolResult`
- `CancelTurn`
- `GetCapabilities`

Optional:

- `Infer`

Planning should define exact request and response contracts, including interruption and replay semantics.

### Recovery Model

- startup scan behavior
- interrupted turn handling
- replay labels for interruptible commands
- provider-stream recovery expectations

### Config Model

- `rules[].activate`
- `agents`
- `security_policies`
- precedence rules between the three

## First Implementation Slice

The first delivery should aim for the smallest meaningful vertical slice:

1. Add an `agent` service skeleton.
2. Add a minimal `activate` path.
3. Implement persisted `agent_session` and `agent_turn` records.
4. Support one provider adapter through a narrow driver contract.
5. Support one workflow tool with event-driven completion handling.
6. Support interruption-aware replay on resumed commands.
7. Defer advanced compaction and full policy richness until the runtime loop is stable.

## Phased Execution Plan

### Phase 0: Runtime Contract (Design-Only)

Goal:

- lock architecture boundaries and runtime contracts before schema work

Outputs:

- `agent_session` and `agent_turn` canonical models
- state machine diagram and transition table
- correlation key strategy
- driver adapter API contract draft

Exit criteria:

- no unresolved ownership overlap between `engine`, `agent`, and AI drivers
- all resume triggers map to explicit state transitions
- interruption and replay behavior is deterministic on paper

### Phase 1: Minimal Vertical Slice (Single Provider)

Goal:

- prove end-to-end `activate -> agent service -> provider -> response` with persistence and restart safety

Scope:

- one provider driver
- one tool type (`workflow`)
- minimal policy enforcement
- no advanced compaction

Outputs:

- agent service bootstrap and routing path
- persisted session and turn records
- workflow completion subscription and resume
- interruption-aware replay labels and handler behavior

Exit criteria:

- service restart during active turn does not corrupt session state
- resumed command path avoids duplicate side effects
- workflow tool completion resumes the correct turn by correlation key

### Phase 2: Policy and Capability Expansion

Goal:

- introduce agent-level policy envelope and provider/tool constraints

Scope:

- top-level `security_policies`
- agent-scoped provider allowlist and defaults
- provider capability discovery via `GetCapabilities`
- optional `Infer` for one bounded mode

Outputs:

- policy precedence implementation (`policy` > `agent` > `activate`)
- policy checks at activation, provider selection, and tool invocation
- capability-gated behavior paths

Exit criteria:

- policy violations fail closed with explicit reasons
- provider request from `activate` cannot widen agent allowances
- unsupported infer modes fall back predictably

### Phase 3: Context Intelligence

Goal:

- add context identity, dedupe, and compaction with safe defaults

Scope:

- history identity strategy
- dedupe strategy
- compaction trigger and summarization flow
- one `Infer`-based orchestration use case (e.g. engagement decision or tool shortlist)

Outputs:

- compaction checkpoints and replay behavior
- deterministic fallback when infer or compaction fails
- observability for context operations

Exit criteria:

- compaction never loses required recent context
- failed compaction does not block core turn execution
- dedupe does not incorrectly drop unique user messages

### Phase 4: Migration and Decommissioning

Goal:

- migrate off driver-local session authority without service disruption

Scope:

- compatibility mode for legacy AI wrapper paths
- staged cutover per provider
- rollback path

Outputs:

- migration guide and feature flags
- deprecation plan for old wrapper session authority

Exit criteria:

- target providers run fully under agent service session authority
- rollback tested and documented

## Decision Gates

Before implementation begins, these must be explicitly decided:

1. Canonical correlation keys across turn/tool/workflow/provider events.
2. Replay semantics for interrupted commands (`resume` vs `restart` vs `fail`).
3. Single source of truth for turn progress and stream cursors.
4. Minimum guaranteed driver contract for all supported providers.
5. Policy precedence and default-deny behavior.

## Immediate Next Actions For Planning Agent

1. Produce a state transition table for `agent_session` and `agent_turn` including every resume trigger.
2. Produce a key-value persistence map showing record keys, indexes, and TTL strategy.
3. Draft provider adapter API contracts for `StartTurn`, `ContinueTurnWithToolResult`, `CancelTurn`, `GetCapabilities`, and optional `Infer`.
4. Define interruption replay labels and idempotency requirements for command handlers.
5. Propose a first-provider vertical slice plan with explicit acceptance tests.

## Questions The Planning Agent Should Answer

1. What exact fields are required on `agent_session` and `agent_turn`?
2. What are the canonical states and transitions?
3. Which IDs correlate provider events, workflow completions, and tool waits?
4. How should restart recovery distinguish resume, restart, and fail-terminally?
5. How should interruptible command replay be signaled in labels or payloads?
6. What is the minimum viable AI driver contract for the first provider?
7. What compaction and dedupe features must exist in the first slice versus later phases?
8. What migration path allows old chat-wrapper behavior and new agent-service behavior to coexist safely?