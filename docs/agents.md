# Agent System Guide

> **Introduced in v4** — The agent system is a major v4 feature that enables Honeydipper to interact with AI models through persistent, stateful conversations. It is fully integrated with the Honeydipper workflow engine, allowing agents to call systems, workflows, sub-agents, and remote MCP tools.

## Table of Contents

- [Overview](#overview)
- [Core Concepts](#core-concepts)
  - [AgentSession](#agentsession)
  - [ConvoState](#convostate)
  - [PersistentAgentStore](#persistentagentstore)
- [Configuration](#configuration)
  - [Agent Definition](#agent-definition)
  - [CompactionPolicy](#compactionpolicy)
  - [AgentToolDef](#agenttooldef)
- [Tool Building](#tool-building)
  - [System Tools (`sys_` prefix)](#system-tools-sys_-prefix)
  - [Workflow Tools (`wf__` prefix)](#workflow-tools-wf__-prefix)
  - [Agent Tools (`ag__` prefix)](#agent-tools-ag__-prefix)
  - [MCP Tools (`mcp__` prefix)](#mcp-tools-mcp__-prefix)
  - [Skill Loading Tool (`hd_load_skill`)](#skill-loading-tool-hd_load_skill)
- [Polling Mechanism](#polling-mechanism)
- [Compaction Policies](#compaction-policies)
- [Message Flow](#message-flow)
- [Pre-Context and Skills](#pre-context-and-skills)
- [Session Lifecycle](#session-lifecycle)
- [Practical Examples](#practical-examples)

---

## Overview

The agent system enables Honeydipper to run stateful, multi-turn conversations with AI models. Unlike traditional stateless API calls to LLMs, the agent system:

- **Persists conversation history** across sessions using a distributed cache (Redis), surviving daemon restarts and configuration reloads.
- **Supports tool calling** — agents can invoke Honeydipper systems, workflows, other agents, and remote MCP servers.
- **Emits streaming responses** back to the caller (workflow or UI) as the model generates output.
- **Auto-compacts** conversation history using configurable LLM-based summarization to stay within token limits.
- **Integrates seamlessly** with the Honeydipper eventbus — workflows can trigger agents and await their results.

---

## Core Concepts

### AgentSession

`AgentSession` (`internal/agent/agent_session.go`) holds the runtime state of a single agent inference or chat-turn session. It is the central object in the agent system.

| Field | Type | Description |
|---|---|---|
| `ID` | `string` | Unique session identifier (UUID) |
| `ConvoID` | `string` | Conversation identifier grouping multiple sessions |
| `UnifiedConvoID` | `string` | Cross-conversation identifier for sub-agent chains |
| `Agent` | `*config.Agent` | Resolved agent configuration |
| `history` | `[]AgentMessage` | Conversation history (chronological message list) |
| `Type` | `string` | `SessionTypeInference` or `SessionTypeChatTurn` |
| `TTL` | `string` | Session cache TTL (default: `72h`) |
| `ToolCalls` | `[]AgentToolCall` | Pending tool calls from the model |
| `ToolResults` | `[]map[string]interface{}` | Collected tool call results |
| `ParentSessionID` | `string` | Parent session ID (set for sub-agent sessions) |

Sessions are stored in the distributed cache under the key prefix `agent_session:<ID>`.

### ConvoState

`ConvoState` (`internal/agent/convo_state.go`) is the **persisted, queryable view of a conversation**. While an `AgentSession` tracks a single inference turn, the `ConvoState` spans the entire conversation.

| Field | Type | Description |
|---|---|---|
| `ConvoID` | `string` | Canonical conversation ID |
| `UnifiedConvoID` | `string` | Shared ID across sub-agent conversations |
| `FirstSession` | `*ConvoSessionRef` | First session in this conversation |
| `LastSession` | `*ConvoSessionRef` | Most recent session (UI-visible) |
| `ActiveSession` | `*ConvoSessionRef` | Currently active session (for cancel targeting) |
| `Cancelled` | `bool` | Whether the conversation has been cancelled |
| `TotalTokens` | `int` | Cumulative input+output tokens |
| `Generation` | `int` | Number of times compaction has occurred |
| `ArchivedConvos` | `[]string` | Keys of archived pre-compaction histories |
| `Agent` | `*config.Agent` | Resolved agent configuration (shared across sessions) |
| `Skills` | `map[string]string` | Loaded skill name → path mapping |

Sessions in a conversation go through status transitions:

```
active → complete
        → failed
        → cancelled
```

ConvoState is stored under key prefix `convo_state:<ConvoID>` and is also published to a stream (`convo_stream_`) for UI consumption.

A `ConvoSessionRef` records a compact summary of each session:

```go
type ConvoSessionRef struct {
    SessionID    string
    AgentName    string
    Type         string       // "inference" or "chat_turn"
    Status       string       // "active", "complete", "failed", "cancelled"
    ErrorReason  string
    InputTokens  int
    OutputTokens int
    TotalTokens  int
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

### PersistentAgentStore

`PersistentAgentStore` (`internal/agent/agent_store.go`) is the **eventbus-driven interface** that bridges Honeydipper's workflow engine and the agent system. It implements the `AgentStore` interface and handles lifecycle management of agent sessions.

Key methods:

| Method | Trigger | Description |
|---|---|---|
| `StartInference` | `eventbus:agent_start` | Creates a new agent session and begins inference |
| `ContinueInference` | `eventbus:agent_continue` | Continues a session with tool call results |
| `ReceiveInference` | `agentbus:receive` | Processes model responses from AI drivers |
| `PollInference` | `eventbus:agent_poll` | Polls session for streaming output |
| `StartAgentCall` | `eventbus:agent_call` | Starts a sub-agent session (agent-as-tool) |
| `StartMCPCall` | `eventbus:mcp_call` | Calls a remote MCP server tool |
| `CancelConvo` | `eventbus:convo_cancel` | Marks a conversation as cancelled |
| `StartTurn` | HTTP API | Starts a new chat turn from the UI |
| `StartNewConvo` | HTTP API | Creates a brand-new conversation from the UI |

The store uses a `sync.WaitGroup` to track in-flight sessions and supports graceful draining via `Stop()` / `Wait()`.

---

## Configuration

### Agent Definition

Agents are defined in the Honeydipper configuration under the `agents` section. Each agent maps to an AI model driver and a set of tools.

```yaml
agents:
  my_assistant:
    driver: openai          # AI model driver name
    engine: gpt-4o          # Model/engine name
    system_prompt: |
      You are a helpful DevOps assistant. You can help with deployments,
      monitoring, and infrastructure management.
    should_stream: true
    max_history_len: 100
    pre_context:
      - /path/to/CONTEXT.md
    skills:
      - /path/to/skills/
    file_tool: sys__files__read
    compaction_policy:
      strategy: summarize
      threshold_type: history_len
      threshold: 50
      preserve_recent: 10
      summarization_agent: fast_summarizer
      summarization_prompt: "Summarize preserving key decisions."
    tools:
      - type: system
        name: kubernetes
      - type: workflow
        name: deploy_app
      - type: agent
        name: code_reviewer
      - type: mcp
        name: github_server
```

### CompactionPolicy

```go
type CompactionPolicy struct {
    Strategy           string // "summarize"
    Threshold          int    // when to trigger compaction
    ThresholdType      string // "history_len" or "total_tokens"
    PreserveRecent     int    // number of recent messages to keep verbatim
    SummarizationAgent string // agent name to produce the summary
    SummarizationPrompt string // optional custom prompt for summarization
}
```

When `ThresholdType` is `"history_len"`, compaction triggers when `len(history) >= Threshold`.
When `ThresholdType` is `"total_tokens"`, compaction triggers when cumulative tokens `>= Threshold`.

The default `PreserveRecent` is 10 messages. The default summarization prompt is:

> "Summarize the above conversation history, preserving key decisions, context, and any critical information that will be needed to continue the conversation. Be concise but thorough. Explain what is currently happening at the end."

### AgentToolDef

```go
type AgentToolDef struct {
    Type     string   // "system", "workflow", "agent", or "mcp"
    Name     string   // System/workflow/agent/MCP server name
    Only     []string // (MCP only) Whitelist of tool names to expose
    Excludes []string // (MCP only) Blacklist of tool names to exclude
}
```

---

## Tool Building

At the start of each model call, the agent session assembles a tool map from its configured tools. Tools are exposed to the AI model using a naming convention based on their type.

### System Tools (`sys_` prefix)

Each function of a named system is exposed as a callable tool:

```
sys_<system_name>__<function_name>
```

**Example:** If a system named `kubernetes` has functions `deploy`, `get_pods`, and `scale`, the following tools are generated:

| Tool Name | Maps To |
|---|---|
| `sys__kubernetes__deploy` | `kubernetes.deploy()` |
| `sys__kubernetes__get_pods` | `kubernetes.get_pods()` |
| `sys__kubernetes__scale` | `kubernetes.scale()` |

System functions with `skip_agent: true` in their metadata are excluded.

### Workflow Tools (`wf__` prefix)

Named workflows are exposed directly:

```
wf__<workflow_name>
```

**Example:** A workflow named `deploy_app` becomes `wf__deploy_app`.

### Agent Tools (`ag__` prefix)

Other agents can be called as sub-agents:

```
ag__<agent_name>
```

**Example:** An agent named `code_reviewer` becomes `ag__code_reviewer`.

Agent tools accept these parameters:

| Parameter | Type | Description |
|---|---|---|
| `input` | `string` | Input text to send to the sub-agent |
| `one_shot` | `boolean` | If true, runs without persisting conversation history |
| `forget_history` | `boolean` | If true, clears the agent's previous conversation history |
| `compaction_id` | `string` | (Internal) Used during compaction for loading archived history |

### MCP Tools (`mcp__` prefix)

Remote MCP (Model Context Protocol) server tools are exposed with a two-underscore separator:

```
mcp__<server_name>__<tool_name>
```

**Example:** If an MCP server named `github_server` exposes a tool `create_issue`, the resulting tool is `mcp__github_server__create_issue`.

MCP tool lists are **cached** (default TTL: 6 hours) to avoid slow chat turns on repeated calls. The cache TTL is configurable via `daemon.services.agent.tools_cache_ttl`.

Using `Only` and `Excludes` in `AgentToolDef`, you can filter which MCP tools are exposed:

```yaml
tools:
  - type: mcp
    name: github_server
    only:
      - create_issue
      - list_repos
```

### Skill Loading Tool (`hd_load_skill`)

When the agent's `systemPrompt` contains the skills header marker and `SkillsPaths` are configured, a special `hd_load_skill` tool is automatically registered. This allows the model to dynamically load skill content during a conversation:

```
hd_load_skill(skill_name: "file_ops/read_file")
```

---

## Polling Mechanism

The agent system uses **long-polling** for streaming model output. This allows real-time delivery of model responses without requiring WebSocket or SSE support from the HTTP layer.

**Flow:**

1. A workflow or UI calls the agent with an `agent_poll` message.
2. The session checks for new agent messages since the last poll.
3. If new content is available (full messages or streaming chunks), it is returned immediately.
4. If no new content is available, the poll blocks (up to a configurable timeout, default 9 seconds).
5. After the timeout, an error response is returned — the caller should re-poll.

The polling mechanism includes:
- **Rate limiting**: Minimum interval between polls is 2 seconds to prevent Redis abuse.
- **Cancellation checking**: Each poll cycle checks whether the conversation has been cancelled.
- **Streaming chunk accumulation**: Partial model outputs (`PendingContent`, `PendingThoughts`) are accumulated and flushed on poll.

---

## Compaction Policies

When conversation history grows too long, the agent system automatically compacts it to reduce token usage. Compaction is triggered **lazily**, before the next model call, when the configured threshold is exceeded.

**Compaction process:**

1. `shouldCompact()` checks if the threshold is reached.
2. `compactHistory()` archives the current history under a generation-suffixed key (`convo_history:<ConvoID>_g<N>`).
3. A summarization sub-agent is invoked with `compaction_id` and `preserve` parameters.
4. The sub-agent reads the archived history, produces a summary, and returns it via `eventbus:agent_continue`.
5. `handleCompactionResult()` replaces the history with the summary (as a system message) plus the most recent `PreserveRecent` messages.
6. The conversation resumes with the compacted history.

**Key constraints:**
- Compaction only triggers on **user messages** (not tool results).
- The summarization agent must be configured and reference a valid driver.
- Archived generations are discoverable: `convo_history:<ConvoID>_g1`, `_g2`, etc.

---

## Message Flow

```
┌──────────────┐  eventbus:agent_start   ┌──────────────────┐
│   Workflow   │ ──────────────────────► │  Agent Service   │
│   /   UI     │                         │ (agent_svc.go)   │
└──────┬───────┘                         └────────┬─────────┘
       │                                          │
       │  eventbus:agent_poll                     │ agentbus:send_to_model
       │ ◄──────── agent_response ───────        │ ──────────────► AI Driver
       │  (full_messages, live_message,           │
       │   state: {history_len, total_tokens})    │ agentbus:receive
       │                                          │ ◄────────────── AI Driver
       │                                          │
       │                    eventbus:agent_continue (tool result)
       │                    ◄─────────────────────┤
       │                                          │
       │  Tool calls dispatched via:              │
       │  eventbus:agent_command (→ sys_*)        │
       │  eventbus:agent_workflow  (→ wf__*)      │
       │  eventbus:agent_call       (→ ag__*)     │
       │  eventbus:mcp_call         (→ mcp__*)    │
```

1. **Start**: Workflow emits `eventbus:agent_start` with `resume_key`. The agent service creates a session and begins inference.
2. **Model Call**: The session sends history and tools to the AI driver via `agentbus:send_to_model`.
3. **Tool Dispatch**: When the model requests tool calls, the session emits the appropriate eventbus message. Results return via `eventbus:agent_continue`.
4. **Polling**: The workflow/UI polls with `eventbus:agent_poll` to receive streaming output.
5. **Completion**: Final agent message triggers `agent_continue` back to the workflow, which resumes.

---

## Pre-Context and Skills

Agents can be configured with **pre-context files** and **skill directories** that are loaded at the start of a new conversation using the configured `file_tool`.

**Pre-Context** (`pre_context`): Files whose content is appended to the system prompt before the first turn. Useful for providing project-specific context, coding standards, or environment details.

```yaml
pre_context:
  - /workspace/CONTEXT.md
  - /workspace/architecture.md
```

**Skills** (`skills`): Directories containing `SKILL.md` files. At conversation start, all skills are parsed and listed in the system prompt. The model can then use `hd_load_skill` to load a specific skill's full content during the conversation.

```yaml
skills:
  - /workspace/skills/
```

Skills are parsed as YAML frontmatter. Each `SKILL.md` must have:

```yaml
---
name: read_file
description: Read the contents of a file from the workspace.
---

# Skill content here...
```

---

## Session Lifecycle

| Stage | Description |
|---|---|
| **Creation** | `setup()` initializes a new session or restores from cache. Acquires distributed lock. Acquiring the lock uses a 600-second expiry. |
| **Pre-Context** | `loadPreContextAndSkills()` loads configured files and skills before the first run. |
| **Run** | `run()` appends the user message, then sends history + tools to the AI driver via `sendToDriver()`. |
| **Streaming** | The driver returns partial chunks. `processAgentMessage()` accumulates content and thoughts. |
| **Tool Call** | When the model requests tools, `nextToolCall()` dispatches to the appropriate handler. |
| **Tool Result** | `processToolResult()` collects results. If more tools are pending, dispatches the next. Otherwise, feeds all results back to the model. |
| **Polling** | `processAgentPoll()` blocks until new content is available or timeout (default 9s). |
| **Compaction** | Triggered when history exceeds threshold. Archives old history, summarizes, and resumes. |
| **Completion** | Final agent message is emitted. Session state is synced to `ConvoState`. Lock is released. |
| **Caching** | `persist()` serializes the session to the distributed cache. Cache TTL defaults to 72 hours. |
| **Cancellation** | `checkCancelled()` is called before processing. Cancelled sessions abort with error. |

### Session Recovery

If the daemon restarts mid-inference, interrupted sessions are recovered via `eventbus:agent_recover`:

1. The AI driver emits `agentbus:rpc/interrupted` with the `agent_session_id` label.
2. The service queues an `agent_recover` message through Redis for durability.
3. On restart, the recover handler loads the session from cache and calls `recover()`, which re-sends the history to the driver.

---

## Practical Examples

### Basic Chat Agent Configuration

```yaml
agents:
  chat_helper:
    driver: openai
    engine: gpt-4o-mini
    system_prompt: "You are a helpful assistant for the DevOps team."
    should_stream: true
    tools:
      - type: system
        name: pagerduty
      - type: workflow
        name: manage_incident
```

### Agent with Compaction

```yaml
agents:
  long_running_agent:
    driver: openai
    engine: gpt-4o
    system_prompt: "You are an expert code reviewer."
    max_history_len: 200
    compaction_policy:
      strategy: summarize
      threshold_type: history_len
      threshold: 80
      preserve_recent: 10
      summarization_agent: fast_summarizer

agents:
  fast_summarizer:
    driver: openai
    engine: gpt-4o-mini
    system_prompt: "You are a summarization expert. Create concise summaries."
```

### Agent with MCP Tools and Filtering

```yaml
agents:
  github_assistant:
    driver: openai
    engine: gpt-4o
    system_prompt: "You help manage GitHub repositories and issues."
    tools:
      - type: mcp
        name: github_mcp
        only:
          - create_issue
          - list_issues
          - create_pull_request
          - get_file_contents
```

### Agent with Pre-Context and Skills

```yaml
agents:
  codebase_assistant:
    driver: openai
    engine: gpt-4o
    file_tool: sys__workspace__read
    system_prompt: |
      You are a codebase assistant. You have access to the project files
      and skills to help with development tasks.
    pre_context:
      - /repo/docs/ARCHITECTURE.md
      - /repo/docs/CONTRIBUTING.md
    skills:
      - /repo/skills/
      - /shared/skills/
    tools:
      - type: system
        name: git
      - type: system
        name: github
      - type: workflow
        name: run_tests
```

### Nested Agent Calling

```yaml
agents:
  orchestrator:
    driver: openai
    engine: gpt-4o
    system_prompt: "You coordinate between specialist agents."
    tools:
      - type: agent
        name: code_reviewer
      - type: agent
        name: security_scanner

  code_reviewer:
    driver: openai
    engine: gpt-4o-mini
    system_prompt: "You are a code reviewer. Review code for quality and best practices."

  security_scanner:
    driver: openai
    engine: gpt-4o-mini
    system_prompt: "You are a security expert. Scan code for vulnerabilities."
```

### Workflow Triggering an Agent

```yaml
workflows:
  ask_assistant:
    steps:
      - call_agent:
          agent: chat_helper
          prompt: "Check the current incident status for team Alpha."
      - notify:
          channel: slack
          message: "Agent response: $result"

rules:
  - when:
      source:
        system: webhook
        trigger: alert
    do:
      call_workflow: ask_assistant
```
