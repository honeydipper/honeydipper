# Agent Service Design

Companion planning brief: [Agent Service Planning Brief](./agent_service_plan_brief.md)

## Goal

Evolve Honeydipper from AI chat wrappers embedded in individual drivers into a first-class, durable, policy-governed agent runtime.

The target design should:

- keep `do` for workflow execution
- add `activate` for agent execution
- introduce first-class `agents`
- introduce top-level `security_policies`
- persist `agent_session` state so execution can survive restart
- keep AI drivers as provider adapters rather than session authorities

In one line: build a policy-governed, event-driven, stateful agent platform inside Honeydipper, not just a shared LLM chat helper.

## High-Level Model

### Existing Model

Today, AI behavior is largely implemented as driver-local chat session handling:

- chat history is stored in cache
- locking is managed per conversation
- streaming is handled inside the AI driver
- tool calls may block waiting for workflow completion
- restart behavior is fragile for long-running turns

### Target Model

The target architecture introduces a new `agent` service.

Responsibilities:

- `engine` service:
  - owns `do`
  - executes workflows
- `agent` service:
  - owns `activate`
  - manages persisted `agent_session` state
  - selects provider from agent policy
  - manages conversation history, dedupe, compaction
  - coordinates tools, workflow completions, scheduling, and interruption recovery
- AI drivers:
  - implement provider-specific inference and streaming
  - expose a narrow provider adapter contract

## Top-Level Concepts

### Rules

Rules gain a second execution path:

- `do`: workflow execution
- `activate`: agent execution

These must be mutually exclusive.

### Agents

An `agent` is a reusable execution identity.

An agent defines:

- system prompt
- allowed providers
- default provider
- provider selection policy
- history identity strategy
- dedupe behavior
- context compaction behavior
- tool catalog and tool selection policy
- referenced security policy

An agent is not a provider. It chooses among allowed providers.

### Security Policies

`security_policies` should be a top-level entity.

They define hard boundaries such as:

- allowed workflows
- allowed provider calls
- allowed provider commands
- allowed raw inference modes
- approval requirements
- memory and context access rules
- deny rules

Policy should be the hard boundary. Agent config should operate within that boundary. `activate` should only narrow behavior, never widen it.

### Agent Sessions

The `agent` service should manage durable `agent_session` state using a context store similar in spirit to workflow session storage.

The store is the source of truth, not the streaming RPC.

## Agent Runtime Responsibilities

The `agent` service owns:

- activation from matched rules
- session and turn lifecycle
- provider selection
- prompt assembly
- system prompt application
- context and history loading
- dedupe
- compaction
- security checks
- tool invocation
- workflow result subscription
- scheduler integration
- interruption and restart recovery

The AI driver should not own:

- conversation lock authority
- history lifecycle
- compaction logic
- policy enforcement
- workflow waiting logic
- session authority

## Agent Session Store

The store should support:

- lookup by session identity
- lookup by conversation and history identity
- lookup by active turn
- lookup by pending workflow and tool correlation ID
- restart recovery of interrupted turns
- compaction metadata
- partial streaming progress metadata

### Suggested Stored Entities

- `agent_session`
  - session ID
  - agent name
  - security policy name
  - conversation identity
  - current state
  - current provider selection
  - timestamps
  - current turn ID
  - compaction metadata
  - dedupe metadata
- `agent_turn`
  - turn ID
  - session ID
  - provider and model
  - normalized prompt and messages
  - enabled tools
  - current turn state
  - stream cursor and sequence
  - partial output metadata
  - restart policy
  - pending tool and workflow references
- `agent_history`
  - persisted messages or references
  - summary checkpoints
  - compaction cursor
  - identity metadata

## Agent Session State Machine

Suggested high-level states:

- `created`
- `resolving_context`
- `selecting_provider`
- `deciding_engagement`
- `streaming`
- `waiting_tool`
- `waiting_workflow`
- `waiting_schedule`
- `waiting_user`
- `compacting`
- `completed`
- `failed`
- `cancelled`
- `interrupted`

State transitions must be durable and restart-safe.

## Resume Triggers

The session must be resumable from:

- matched `activate` rule
- workflow completion event
- scheduler event
- user continuation event
- provider stream event
- tool result event
- service restart recovery scan
- cancellation and interruption event

## Workflow Integration

Workflow remains a tool, not the execution substrate for the whole agent.

Recommended pattern:

- agent invokes workflow as a tool
- workflow session store emits completion event
- agent service subscribes to workflow completion events
- agent service correlates result to a pending tool or workflow wait
- agent resumes the relevant turn and session

This replaces blocking wait patterns inside drivers.

## Streaming Design

Long-lived provider streaming should not rely on the service process surviving.

Recommended model:

- `StartTurn` initiates work
- stream events are emitted against a durable `turn_id`
- the store tracks stream progress
- interruption is explicit
- recovery is based on durable turn state plus emitted stream events

The source of truth is the stored `agent_turn`, not the open RPC call.

## Interruption and Retry

Existing interruptible command handling can requeue interrupted commands. For the new agent runtime, resumed commands must know they are resumed after interruption.

Recommended message labels:

- `resumed_after_interrupt=true`
- `resume_reason=<reason>`
- `resume_count=<n>`

Agent commands must be reentry-safe and consult persisted state before repeating side effects.

This is especially important for:

- `StartTurn`
- streaming recovery
- tool invocation
- workflow waits

## AI Driver Responsibilities

AI drivers should be provider adapters with a narrow contract.

Required capabilities:

- `StartTurn`
- `ContinueTurnWithToolResult`
- `CancelTurn`
- `GetCapabilities`

Optional capability:

- `Infer`

### StartTurn

Starts a provider turn and may initiate streaming.

### ContinueTurnWithToolResult

Resumes a provider turn after tool execution.

### CancelTurn

Cancels an active turn.

### GetCapabilities

Returns provider and model capabilities such as:

- supported models
- streaming support
- tool-call support
- structured output support
- raw inference support

### Infer

Optional single-shot raw inference, stateless.

Useful for:

- context compaction
- engagement decisions
- tool selection
- classification
- extraction
- routing

Recommended modes:

- `summary`
- `classify`
- `route_tools`
- `decide_engagement`
- `custom`

`Infer` should use structured output wherever possible.

## Context Management

The agent owns context behavior.

This includes:

- history identity
- dedupe
- compaction
- summary checkpoints
- hot versus archived context

These are agent-level concerns, not provider-level concerns.

## Suggested Config Direction

### Rule-Level `activate`

Per-activation inputs and narrowing:

- `agent`
- `prompt`
- optional requested provider
- optional narrowed tools

### Agent-Level Definition

Stable agent profile:

- `system_prompt`
- provider policy
- tool policy
- context and history policy
- compaction policy
- dedupe policy
- referenced security policy

### Top-Level Security Policy

Reusable authorization policy for:

- workflows
- provider calls
- provider commands
- raw inference modes
- memory and context access
- approval requirements

## Comparison To Other AI Agent Systems

This design is closer to an orchestration-grade agent runtime than to interactive assistant products.

Compared to tools like Claude Desktop, Claude CLI, or GitHub Copilot:

- this design is more durable
- more policy-driven
- more event-driven
- more workflow-native
- more suitable for unattended automation

It is less like a chat product and more like an agent operating system inside Honeydipper.

## Recommended Design Order

Do not start from the final schema.

Recommended sequence:

1. define service boundaries
2. define `agent_session` and `agent_turn` persistence model
3. define the session state machine
4. define resume triggers and recovery logic
5. define tool and workflow completion integration
6. define context identity, dedupe, and compaction behavior
7. define security enforcement points
8. then design config schema

## Migration Direction

Short-term:

- keep existing AI wrapper behavior running
- design the agent runtime separately

Mid-term:

- move session authority to the `agent` service
- move workflow wait and resume to service level
- keep drivers as provider adapters

Long-term:

- deprecate driver-local chat session management
- make `activate` the standard agent execution path
- keep `do` as workflow-native execution path

## Planning Questions

A planning agent should answer these next:

1. What exact persisted fields are required for `agent_session` and `agent_turn`?
2. What are the canonical session and turn states?
3. What are the durable correlation keys for workflow, tool, and provider events?
4. What restart recovery rules apply to active streaming turns?
5. What exact driver RPC contract should be standardized?
6. How should compaction summaries be stored and replayed?
7. What are the precedence rules between `security_policies`, `agents`, and `activate`?
8. What is the incremental migration path from the current driver-local chat wrapper model?