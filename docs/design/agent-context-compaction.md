# Design: Agent Context Compaction

## 1. Goal
The goal of this feature is to implement **Agentic Context Compaction** within the Honeydipper agent service. This will transition the agent from a simple "hard-prune" sliding window history management to an intelligent "summarization" model. This ensures that long-term conversation context is preserved and synthesized rather than being permanently lost when the history limit is reached.

## 2. Problem Statement
Currently, the `AgentSession` manages conversation history using a fixed `MaxHistoryLen`. Once the number of messages exceeds this limit, the oldest messages are pruned from memory and the persistent store (Redis) using `ltrim`.

**Limitations of the current approach:**
- **Information Loss**: All data, decisions, and context contained in pruned messages are lost forever.
- **Token Unawareness**: The system is count-based rather than token-based, making it prone to context window overflows if individual messages are large.
- **Lack of Synthesis**: There is no mechanism to summarize past interactions to maintain a high-density context for the LLM.

## 3. Proposed Architecture: Agentic Compaction

Instead of treating compaction as a simple truncation, we treat it as a specialized task performed by a **Summarizer Agent**. This leverages the existing agentic abstraction, allowing the summarization process to be as simple or as complex as needed.

### 3.1 Core Concepts
- **Summarizer Agent**: An abstract agent unit (which could be a different driver, a different model, or a complex multi-step agent) responsible for condensing history.
- **Compaction Threshold**: A configurable limit that triggers the compaction process.
- **Compaction Engine**: A new component that orchestrates the "History $\\rightarrow$ Summarizer $\\rightarrow$ New History" workflow.

### 3.2 Workflow
1. **Detection**: During the `appendConvoHistory` phase in an `AgentSession`, the system checks if `len(history) >= CompactionThreshold`.
2. **Task Orchestration**: 
    - The `CompactionEngine` identifies the "old" segment of history (e.g., messages $0$ to $N-K$).
    - It constructs a specialized task for the `SummarizerAgent`.
    - The task instruction: *"Summarize the following conversation history into a concise system prompt that preserves all critical facts, user preferences, and recent decisions."*
3. **Execution**: The task is dispatched via the `eventbus` to the `SummarizerAgent`.
4. **Integration**:
    - The `SummarizerAgent` returns a summary.
    - The `AgentSession` wraps this summary in a `RoleSystem` message.
    - The history is reconstructed: `[New System Summary Message] + [Remaining Recent Messages]`.
5. **Persistence**: The new, compacted history is written back to the `AgentStore` (Redis), replacing the old list.

## 4. Design Details

### 4.1 Configuration Changes (`pkg/agent`)
We will introduce a `CompactionConfig` struct to the `Agent` definition:

```go
type CompactionConfig struct {
    Strategy          string // "agent_summary" | "none"
    Threshold         int    // Trigger point for compaction
    SummarizerAgent   string // The identifier/name of the agent to invoke
}

type Agent struct {
    // ... existing fields ...
    MaxHistoryLen     int
    Compaction        *CompactionConfig // New field
}
```

### 4.2 Component Responsibilities
- **`AgentSession` (Minimal Changes)**: Will call the `CompactionEngine` when the threshold is met.
- **`CompactionEngine` (New File)**: Logic for preparing the summarization task and handling the response.
- **`SummarizerAgent` (External/User Defined)**: A user-configured agent that follows the summarization prompt.

## 5. Implementation Plan

### Phase 1: Schema & Configuration
- Update `pkg/agent` to include `CompactionConfig`.
- Update `Agent` struct to support the new configuration.

### Phase 2: The Compaction Engine
- Implement `internal/agent/compaction_engine.go`.
- Develop the logic to package historical messages into a task for the `eventbus`.
- Implement the history reconstruction logic (`[Summary] + [Recent]`).

### Phase 3: Integration
- Modify `internal/agent/agent_session.go` to trigger the `CompactionEngine`.
- Implement the atomic update of the Redis history list to prevent race conditions or data corruption during the swap.

### Phase 4: Verification
- Unit tests for the `CompactionEngine` logic.
- Integration tests demonstrating that facts from "pruned" messages are successfully carried over via the `SummarizerAgent`.

## 6. Risks & Mitigations
- **Complexity/Latency**: Summarization adds a step to the conversation. 
    - *Mitigation*: Compaction is performed asynchronously or at specific intervals to minimize impact on the user experience.
- **Recursion/Loops**: If the summary agent itself triggers compaction.
    - *Mitigation*: The `CompactionEngine` will explicitly exclude "Compaction Tasks" from being eligible for further compaction.
- **Failure of Summarizer**: If the summarizer agent fails.
    - *Mitigation*: Fallback to the existing `MaxHistoryLen` hard-pruning logic to ensure system stability.
