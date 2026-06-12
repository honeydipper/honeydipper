# Eventbus Message Types

> Honeydipper's internal communication is built around an **eventbus** — a message bus that routes messages between services (engine, operator, receiver, agent, API) and external drivers. Each message has a `Channel` and `Subject` that determine its routing.

## Table of Contents

- [Overview](#overview)
- [Message Structure](#message-structure)
- [Channels](#channels)
- [Core Subjects](#core-subjects)
  - [Eventbus Subjects](#eventbus-subjects)
  - [Agent Subjects](#agent-subjects)
  - [RPC Subjects](#rpc-subjects)
  - [Broadcast Subjects](#broadcast-subjects)
  - [API Subjects](#api-subjects)
  - [Scheduler Subjects](#scheduler-subjects)
- [Agent Session Message Flow](#agent-session-message-flow)
- [Message Labels](#message-labels)
- [Subjects Reference Table](#subjects-reference-table)

---

## Overview

The eventbus is Honeydipper's nervous system. All inter-service communication flows through it:

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│ Receiver │────►│  Engine  │────►│ Operator │
│          │     │          │     │          │
└──────────┘     └────┬─────┘     └────┬─────┘
                      │                │
                eventbus:return  eventbus:command
                      │                │
                      ▼                ▼
               ┌──────────┐     ┌──────────┐
               │  Engine  │     │ Operator │────► Drivers
               │(continue)│     │(execute) │
               └──────────┘     └──────────┘

┌──────────┐     ┌──────────┐
│   API    │◄───►│  Agent   │────► AI Drivers
│ (HTTP)   │     │ Service  │     (agentbus)
└──────────┘     └──────────┘
```

Messages are routed by their `Channel` and `Subject`. The eventbus driver (typically Redis-based) handles the actual message delivery.

---

## Message Structure

The core message type is defined in `pkg/dipper/comm.go`:

```go
type Message struct {
    Channel string            // Routing channel (e.g., "eventbus", "rpc")
    Subject string            // Message subject/type (e.g., "message", "command")
    Labels  map[string]string // Metadata for routing, tracking, and context
    Payload interface{}       // Message data (arbitrary JSON-serializable content)
}
```

### Key Fields

| Field | Type | Description |
|---|---|---|
| `Channel` | `string` | High-level routing channel. Core channels: `eventbus`, `rpc`, `command`, `state`, `agentbus`. |
| `Subject` | `string` | Specific message type within the channel. Combined with channel as `channel:subject` for routing. |
| `Labels` | `map[string]string` | Key-value metadata. Used for session tracking (`sessionID`, `agent_session_id`), routing hints (`feature`, `method`), status reporting (`status`, `reason`), and context propagation. |
| `Payload` | `interface{}` | The actual message data. Structure varies by subject type. |

---

## Channels

| Channel | Constant | Description |
|---|---|---|
| `eventbus` | `dipper.ChannelEventbus` | Primary inter-service and service-to-driver event channel. |
| `rpc` | `dipper.ChannelRPC` | RPC-style request/reply calls. |
| `command` | `dipper.ChannelCommand` | Command execution channel (service-to-driver). |
| `state` | `dipper.ChannelState` | State persistence channel. |
| `agentbus` | `"agentbus"` | Agent service to/from AI model drivers. |

---

## Core Subjects

### Eventbus Subjects

These are the core eventbus subjects defined in `pkg/dipper/comm.go`:

| Subject | Constant | Direction | Description |
|---|---|---|---|
| `message` | `dipper.EventbusMessage` | → Engine | Incoming event from a driver (triggered by external source). |
| `command` | `dipper.EventbusCommand` | → Operator | Function execution request. The operator resolves the target system/function and calls the appropriate driver. |
| `agent_command` | `dipper.EventbusAgentCommand` | → Operator | Agent tool call execution request. Same as `command` but for agent tool invocations. |
| `return` | `dipper.EventbusReturn` | → Engine | Function execution result. Contains the output of a completed system function or workflow step. |
| `return/interrupted` | `dipper.EventbusReturnInterrupted` | → Operator | Interrupted function notification. The operator re-queues the message for retry. |
| `agent_continue` | `dipper.EventbusAgentContinue` | → Engine | Agent tool call result. Returns tool execution output to the agent service for session continuation. |

### Agent Subjects

The agent service uses these subjects for AI model interaction and session management (defined in `internal/service/agent_svc.go`):

| Subject | Direction | Description |
|---|---|---|
| `agent_start` | Store → Agent Service | Initiates a new inference session. Contains `resume_key` for workflow callback and `agent_name` label. |
| `agent_continue` | Operator/Engine → Agent Service | Delivers tool call results back to an existing session. |
| `agent_recover` | Store → Agent Service | Recovers an interrupted session after daemon restart. Carries `agent_session_id` from the RPC interrupted notification. |
| `agent_poll` | Workflow/UI → Agent Service | Polls a session for new streaming output (full messages and/or pending chunks). |
| `agent_call` | Agent Service Store | Dispatches a sub-agent invocation. Carries `sub_agent_name`, `input`, `agent_session_id`. |
| `mcp_call` | Agent Service Store | Dispatches a remote MCP tool call. Carries `server`, `tool`, `args`. |
| `convo_cancel` | API/UI → Agent Service | Marks a conversation as cancelled. |

**Agentbus subjects** (used between agent service and AI model drivers via RPC):

| Subject | Direction | Description |
|---|---|---|
| `agentbus:receive` | AI Driver → Agent Service | Model inference result delivered to the agent service. |
| `agentbus:rpc/interrupted` | AI Driver → Agent Service | Notification that a model's RPC context was cancelled mid-inference. |

### RPC Subjects

| Subject | Direction | Description |
|---|---|---|
| `rpc:call` | Internal | RPC method invocation. |
| `rpc:return` | Internal | RPC method result. |
| `rpc/interrupted` | Internal | RPC call was interrupted (e.g., context cancelled). |

### Broadcast Subjects

| Subject | Description |
|---|---|
| `broadcast:reload` | Configuration reload signal. All services reload their configuration. |

### API Subjects

| Subject | Direction | Description |
|---|---|---|
| `api-broadcast` | API → All | Broadcast API requests to matching services. |
| `api:candidate` | Internal | API lock contention resolution. |
| `eventbus:api` | → API Service | API messages received from the eventbus (ACKs and results). |

### Scheduler Subjects

| Subject | Direction | Description |
|---|---|---|
| `scheduler:session` | → Engine | Scheduled session tick. Triggers time-based workflow execution. |

---

## Agent Session Message Flow

A complete agent inference session follows this message flow:

### 1. Session Start

```
Workflow/UI ──eventbus:resume_key──► Agent Service
                                     (agent_svc.go:handleAgentStart)

Labels: {
  "resume_key": "<uuid>",
  "agent_name": "my_assistant"
}
Payload: {
  "text": "user message",
  "type": "chat_turn" | "inference",
  "data": { ... },
  "user": "alice@example.com"
}
```

The agent service emits an `agent_response` immediately:

```json
{
  "channel": "eventbus",
  "subject": "agent_response",
  "labels": {
    "resume_key": "<uuid>",
    "agent_session_id": "<session-uuid>"
  }
}
```

### 2. Model Inference Request

The agent session sends to the AI driver via RPC:

```
Agent Service ──► driver:<driver_name>.send_to_model
                 (agentbus:receive on return)

Parameters: {
  "engine": "gpt-4o",
  "history": [ ... ],
  "tools": [ ... ],
  "type": "chat_turn",
  "model_data": { ... },
  "should_stream": true,
  "agent_settings": { ... }
}
Labels: {
  "agent_session_id": "<session-uuid>",
  "timeout": "1h"
}
```

### 3. Model Response

```
AI Driver ──agentbus:receive──► Agent Service
                                (agent_svc.go:handleAgentReceive)

Payload: {
  "message": {
    "role": "agent",
    "content": "response text",
    "tool_calls": [ ... ],
    "is_complete": true
  }
}
Labels: {
  "agent_session_id": "<session-uuid>",
  "status": "success" | "failure",
  "reason": "<error message>"
}
```

### 4. Tool Call Dispatch

When the model requests tool calls, the agent session dispatches based on prefix:

| Tool Prefix | Eventbus Subject | Direction |
|---|---|---|
| `sys_*` | `eventbus:agent_command` | → Operator |
| `wf__*` | `eventbus:agent_workflow` | → Engine |
| `ag__*` | `eventbus:agent_call` | → Agent Service (Store) |
| `mcp__*` | `eventbus:mcp_call` | → Agent Service (Store) |

### 5. Tool Result

```
Operator/Engine/Store ──eventbus:agent_continue──► Agent Service

Labels: {
  "agent_session_id": "<session-uuid>",
  "turn_id": "<turn-number>",
  "tool_call_id": "<call-index>",
  "status": "success" | "failure",
  "reason": "<error message>"
}
Payload: {
  "data": {
    "output": "<tool result>"
  }
}
```

### 6. Polling for Output

```
Workflow/UI ──eventbus:agent_poll──► Agent Service
                                    (agent_svc.go:handleAgentPoll)

Labels: {
  "agent_session_id": "<session-uuid>",
  "resume_key": "<uuid>",
  "timeout": "9s"
}
```

Response:

```json
{
  "channel": "eventbus",
  "subject": "agent_response",
  "labels": {
    "status": "success",
    "last_poll": "5"
  },
  "payload": {
    "full_messages": [
      { "content": "...", "is_thinking": "false", "thoughts": "" }
    ],
    "live_message": {
      "content": "partial streaming content..."
    },
    "state": {
      "history_len": 6,
      "total_tokens": 1500,
      "convo_id": "<convo-uuid>"
    }
  }
}
```

---

## Message Labels

Labels are used extensively for routing, tracking, and context propagation.

### Common Labels

| Label | Used By | Description |
|---|---|---|
| `sessionID` | Engine | Workflow session identifier |
| `agent_session_id` | Agent Service | Agent session identifier |
| `resume_key` | Agent Service | Key for workflow/UI callback |
| `turn_id` | Agent Service | Current turn number within a session |
| `tool_call_id` | Agent Service | Index of the tool call being processed |
| `unified_convo_id` | Agent Service | Cross-conversation identifier |
| `agent_name` | Agent Service | Name of the invoked agent |
| `sub_agent_name` | Agent Service | Name of the sub-agent being called |
| `convo_id` | Agent Service | Conversation identifier |
| `status` | Universal | `success`, `failure`, or `error` |
| `reason` | Universal | Error description when status is not success |
| `feature` | Operator | Driver feature identifier (e.g., `driver:kubernetes`) |
| `method` | Operator | Driver method name |
| `user` | API/Agent | Authenticated user name |
| `user_provider` | API/Agent | User's authentication provider |
| `error` | API | Error message from a responder |

---

## Subjects Reference Table

### Complete Subject List

| Full Address | Constant | Direction | Description |
|---|---|---|---|
| `eventbus:message` | `dipper.ChannelEventbus + ":message"` | → Engine | Incoming driver event |
| `eventbus:command` | `dipper.ChannelEventbus + ":command"` | → Operator | Function execution |
| `eventbus:agent_command` | `dipper.ChannelEventbus + ":agent_command"` | → Operator | Agent tool execution |
| `eventbus:return` | `dipper.ChannelEventbus + ":return"` | → Engine | Function result |
| `eventbus:return/interrupted` | `dipper.ChannelEventbus + ":return/interrupted"` | → Operator | Interrupted function |
| `eventbus:agent_continue` | `dipper.ChannelEventbus + ":agent_continue"` | → Engine | Agent tool result |
| `eventbus:agent_workflow` | `dipper.ChannelEventbus + ":agent_workflow"` | → Engine | Agent workflow trigger |
| `eventbus:agent_response` | `dipper.ChannelEventbus + ":agent_response"` | → Engine/UI | Agent streaming response |
| `eventbus:agent_call` | `dipper.ChannelEventbus + ":agent_call"` | → Store | Sub-agent invocation |
| `eventbus:mcp_call` | `dipper.ChannelEventbus + ":mcp_call"` | → Store | MCP tool call |
| `agentbus:receive` | `"agentbus:receive"` | AI Driver → Agent | Model inference result |
| `agentbus:rpc/interrupted` | `"agentbus:rpc/interrupted"` | AI Driver → Agent | RPC interruption |
| `broadcast:reload` | `"broadcast:reload"` | → All | Config reload |
| `scheduler:session` | `"scheduler:session"` | → Engine | Scheduled tick |
| `api-broadcast` | `"api-broadcast"` | → All | API request |
| `api:candidate` | `"api:candidate"` | Internal | API lock contention |
| `eventbus:api` | `"eventbus:api"` | → API | API message |
