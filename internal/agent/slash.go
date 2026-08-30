package agent

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// slashCommand is a parsed slash command extracted from a user chat message.
type slashCommand struct {
	name string
	args string
}

// slashHandler is the signature implemented by every slash command. It receives
// the session and the raw argument remainder (everything after the command
// token, trimmed of leading whitespace). A built-in carries out its side effect
// and returns true when the command was handled (which terminates run() without
// invoking the model). It returns false when the command could not run and the
// caller should fall back to normal model handling.
type slashHandler func(s *AgentSession, args string) bool

// slashCmd defines a single registered slash command.
type slashCmd struct {
	name        string
	description string
	handler     slashHandler
}

// slashRegistry holds the built-in slash commands plus an extension hook so
// config-driven commands can be added later without reworking the framework.
var (
	slashRegistry     = []slashCmd{}
	slashRegistryOnce sync.Once
)

// ensureSlashBuiltins registers the built-in slash commands exactly once. It is
// called lazily from the dispatcher so no init() side effect is required while
// keeping the framework extensible: config-driven commands can call
// registerSlashCommand later without reworking the parse/dispatch plumbing.
func ensureSlashBuiltins() {
	slashRegistryOnce.Do(func() {
		registerSlashCommand("help", "Show this list of commands", slashHelp)
		registerSlashCommand("refresh", "Rebuild the system prompt from current agent config", slashRefresh)
		registerSlashCommand("model", "Switch the model for this conversation", slashModel)
		registerSlashCommand("compact", "Compact the conversation history to save context", slashCompact)
		registerSlashCommand("retry", "Regenerate the last response", slashRetry)
	})
}

// registerSlashCommand adds a command to the registry, replacing an existing
// entry with the same name if present.
func registerSlashCommand(name, description string, handler slashHandler) {
	name = strings.TrimPrefix(name, "/")
	for i := range slashRegistry {
		if slashRegistry[i].name == name {
			slashRegistry[i].description = description
			slashRegistry[i].handler = handler

			return
		}
	}
	slashRegistry = append(slashRegistry, slashCmd{name: name, description: description, handler: handler})
}

// parseSlashCommand parses leading slash command text. It returns nil when the
// text does not start with '/'. The first whitespace-delimited token is the
// command (without the leading '/'), and everything after it is the raw args.
func parseSlashCommand(text string) *slashCommand {
	trimmed := strings.TrimLeft(text, " ")
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	// Drop the leading slash, then split off the first whitespace token.
	rest := trimmed[1:]
	idx := strings.IndexAny(rest, " \t")
	if idx < 0 {
		return &slashCommand{name: rest, args: ""}
	}

	return &slashCommand{name: rest[:idx], args: strings.TrimLeft(rest[idx+1:], " \t")}
}

// isSlashTurn reports whether the incoming session message should be treated as
// a potential slash command: it must be a chat-turn (conversation) session and
// the text must start with '/'. Non-slash chat text and all inference text flow
// through untouched.
// recordSlashCommandText appends the raw slash command text itself as a
// marked RoleUser message (IsSlash: true) so the UI has a complete record of
// what the user typed. The message is persisted in history but is excluded from
// the model history by sendToDriver() and from token accounting.
func (s *AgentSession) recordSlashCommandText() {
	text, _ := dipper.GetMapDataStr(s.CurrentMsg.Payload, "text")
	user, _ := dipper.GetMapDataStr(s.CurrentMsg.Payload, "user")
	s.appendConvoHistory(&AgentMessage{
		Role:    RoleUser,
		User:    user,
		Content: text,
		IsSlash: true,
	})
}

func (s *AgentSession) isSlashTurn() bool {
	if s.Type != AgentSessionTypeChatTurn {
		return false
	}
	text, ok := dipper.GetMapDataStr(s.CurrentMsg.Payload, "text")
	if !ok {
		return false
	}

	return strings.HasPrefix(strings.TrimLeft(text, " "), "/")
}

// dispatchSlashCommand parses the incoming text, looks up the command in the
// registry, and runs its handler. It returns true when the command was handled
// (run() must return immediately without invoking the model). An unknown or
// empty command replies with an explanatory message and returns true so the
// user always gets feedback rather than silence.
func (s *AgentSession) dispatchSlashCommand() bool {
	ensureSlashBuiltins()
	// Persist the raw command text as a marked RoleUser message so the UI has a
	// complete record of the slash command, independent of any reply. It is
	// excluded from the model history and token accounting via IsSlash.
	s.recordSlashCommandText()
	text, _ := dipper.GetMapDataStr(s.CurrentMsg.Payload, "text")
	cmd := parseSlashCommand(text)
	if cmd == nil {
		return false
	}
	if cmd.name == "" {
		s.reply("Empty slash command. Try /help to see available commands.")

		return true
	}

	for _, reg := range slashRegistry {
		if reg.name == cmd.name {
			return reg.handler(s, cmd.args)
		}
	}

	s.reply(fmt.Sprintf(
		"Unknown command: /%s.\n\n%s",
		cmd.name,
		buildHelpText(),
	))

	return true
}

// reply appends a complete RoleAgent message (IsComplete: true) to the
// conversation history and persists it. This is the sole return mechanism for
// non-turn commands so both the workflow agent_poll/emitPollResponse loop and
// the UI convoHistory read observe the result without invoking the model.
func (s *AgentSession) reply(content string) {
	msg := &AgentMessage{
		Role:       RoleAgent,
		Content:    content,
		IsComplete: true,
		// Every reply produced for a slash command is marked IsSlash so it is
		// treated as slash-origin output: it remains visible to the workflow/UI
		// return paths but is never sent to the model as context.
		IsSlash: true,
	}
	s.appendConvoHistory(msg)
	// Do NOT advance LastPoll past the new message: the workflow agent_poll loop
	// and emitPollResponse collect RoleAgent messages from LastPoll forward, so
	// leaving it at its prior value lets the poll see this reply as a new turn
	// and emit it as an agent_response without any model invocation.
	s.persist(false)
}

// setConvoAgent persists the given agent config into ConvoState.Agent and
// updates the session's in-memory Agent. It is the reusable helper shared by
// /refresh and /model so their overrides both land in the shared conversation
// state and the system prompt rebuilt in sendToDriver() reflects them.
func (s *AgentSession) setConvoAgent(a *config.Agent) {
	lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
		cs.Agent = a
	})
	s.Agent = a
}

// slashHelp implements /help: prints the static list of all commands.
func slashHelp(s *AgentSession, _ string) bool {
	s.reply(buildHelpText())

	return true
}

func buildHelpText() string {
	lines := make([]string, 0, 2+len(slashRegistry)+2)
	lines = append(lines, "**Available commands**")
	lines = append(lines, "")
	cmds := make([]slashCmd, len(slashRegistry))
	copy(cmds, slashRegistry)
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].name < cmds[j].name })
	for _, c := range cmds {
		lines = append(lines, fmt.Sprintf("- `/%s` — %s", c.name, c.description))
	}
	lines = append(lines, "")
	lines = append(lines, "Type a command followed by Enter to run it.")

	return strings.Join(lines, "\n")
}

// slashRefresh implements /refresh: re-runs agent config interpolation and
// writes the result into ConvoState.Agent so the system prompt rebuilt in
// sendToDriver() reflects any config change.
func slashRefresh(s *AgentSession, _ string) bool {
	agentName := s.Agent.Name
	if agentName == "" {
		s.reply("Cannot refresh: agent name is unknown.")

		return true
	}
	// Re-interpolate from the original agent config using the current session
	// context so changed config values are picked up.
	refreshed := interpolateAgentConfig(s.store, agentName, s.CurrentMsg.Payload)
	s.setConvoAgent(refreshed)

	s.reply(fmt.Sprintf("Agent config refreshed for %q. The system prompt has been rebuilt.", agentName))

	return true
}

// slashModel implements /model <name> (or <driver>:<engine>). It validates the
// target against the known engines and persists the override into
// ConvoState.Agent.Engine/.Driver always. Invalid input replies with an error
// listing the valid engines.
func slashModel(s *AgentSession, args string) bool {
	name := strings.TrimSpace(args)
	if name == "" {
		s.reply("Usage: /model <name> or /model <driver>:<engine>\n\n" + listKnownEngines(s))

		return true
	}

	driver, engine, hasColon := strings.Cut(name, ":")
	if !hasColon {
		driver, engine = name, name
	}

	known := CollectAgentDriverEngines(s.store.GetConfig().DataSet.Drivers)
	for _, e := range known {
		if !strings.EqualFold(e.Engine, engine) {
			continue
		}
		// A bare name (<name>) resolves to whichever known driver exposes it.
		modelDriver := e.Driver
		if hasColon && !strings.EqualFold(e.Driver, driver) {
			continue
		}

		setAgent := *s.Agent
		setAgent.Driver = modelDriver
		setAgent.Engine = e.Engine
		s.setConvoAgent(&setAgent)
		s.reply(fmt.Sprintf("Model set to %s:%s for this conversation.", modelDriver, e.Engine))

		return true
	}

	s.reply("Unknown model: " + name + ".\n\n" + listKnownEngines(s))

	return true
}

// listKnownEngines formats the known agent-driver engines for error/usage text.
func listKnownEngines(s *AgentSession) string {
	known := CollectAgentDriverEngines(s.store.GetConfig().DataSet.Drivers)
	if len(known) == 0 {
		return "No known engines are currently configured."
	}
	lines := []string{"Known engines:"}
	for _, e := range known {
		lines = append(lines, fmt.Sprintf("  %s (driver %s)", e.Engine, e.Driver))
	}

	return strings.Join(lines, "\n")
}

// rewindToPreviousUserTurn locates the last real (non-slash) user message that
// has content, walking backwards past any trailing assistant, tool, or tool
// result turns as well as any slash-origin messages (e.g. the /retry command
// text itself, which is recorded as a marked IsSlash RoleUser message by
// recordSlashCommandText and must never be mistaken for the previous user
// turn). It returns the index of that user message, or -1 when no such message
// exists.
func (s *AgentSession) rewindToPreviousUserTurn() int {
	for i := len(s.history) - 1; i >= 0; i-- {
		m := s.history[i]
		if m.IsSlash {
			continue
		}
		if m.Role == RoleUser && m.Content != "" {
			return i
		}
	}

	return -1
}

// persistConvoHistory rewrites the persisted conversation history in the cache
// to exactly match the session's in-memory history. It is used by /retry after
// rewinding the conversation so the stored history (and therefore the UI read
// path and any subsequent model round-trip) reflects the truncation.
func (s *AgentSession) persistConvoHistory() {
	if s.ConvoID == "" {
		return
	}
	_, _ = s.store.Call("cache", "del", map[string]interface{}{"key": ConvoHistoryKeyPrefix + s.ConvoID})
	convoTTL, _ := time.ParseDuration(ConvoStreamTTL)
	fullKey := ConvoHistoryKeyPrefix + s.ConvoID
	for _, m := range s.history {
		_, _ = s.store.Call("cache", "rpush", map[string]interface{}{
			"key":   fullKey,
			"value": string(dipper.SerializeContent(m)),
			"ttl":   float64(convoTTL),
		})
	}
}

// slashRetry implements /retry: re-send the previous real user message to
// regenerate the last response. It locates the last non-slash user message
// that has content, truncates the history back to just before it (dropping the
// assistant/tool messages it already answered), re-appends that user message
// as a normal, non-slash RoleUser message, and sends it to the driver so the
// response is regenerated through the normal model round-trip. When there is
// no prior user message with content it replies with a helpful error and does
// NOT send to the model.
func slashRetry(s *AgentSession, _ string) bool {
	idx := s.rewindToPreviousUserTurn()
	if idx < 0 {
		s.reply("No previous user message to retry. Please send a new message to continue the conversation or use /help for other commands.")

		return true
	}

	user, _ := dipper.GetMapDataStr(s.CurrentMsg.Payload, "user")
	msg := s.history[idx]
	msg.Role = RoleUser
	msg.User = user
	msg.IsSlash = false

	// Preserve any trailing slash-origin messages (in practice the /retry
	// command text recorded by recordSlashCommandText) so the UI keeps a
	// complete record of what the user typed. They are excluded from the model
	// context via their IsSlash marker and must not be mistaken for the
	// previous real user turn.
	var trailing []AgentMessage
	for _, m := range s.history[idx+1:] {
		if m.IsSlash {
			trailing = append(trailing, m)
		}
	}

	// Truncate back to just before the previous user turn (dropping the
	// assistant/tool messages it already answered) and re-append that user
	// message (with the session user) as a normal, non-slash RoleUser message,
	// followed by the preserved slash-origin markers.
	s.history = append(make([]AgentMessage, 0, idx+1+len(trailing)), s.history[:idx]...)
	s.history = append(s.history, msg)
	s.history = append(s.history, trailing...)

	// Persist the rewound history so both the UI read path and any subsequent
	// poll/history return observe the regenerated conversation.
	s.persistConvoHistory()

	// Recalculate ContextTokens from the rewound history since messages were
	// dropped; the re-appended user message must not be double counted.
	if s.TokenCounter != nil {
		lockedConvoStateUpdate(s.ConvoID, s.store, func(cs *ConvoState) {
			cs.ContextTokens = s.countSystemPromptTokens()
			for _, m := range s.history {
				if m.IsSlash {
					continue
				}
				cs.ContextTokens += s.countMessageTokens(m)
			}
		})
	}

	// Point the poll at the tail of the rewound history so the regenerated
	// response (appended when the model returns) is collected as a new turn.
	s.LastPoll = len(s.history)

	// Regenerate the response through the normal model round-trip
	// (ReceiveInference -> processAgentMessage -> history/poll return).
	s.sendToDriver()

	return true
}

// slashCompact implements /compact: force-compacts the conversation history
// regardless of threshold. When compaction isn't configured or possible it
// replies with an explanatory message instead.
func slashCompact(s *AgentSession, _ string) bool {
	if s.Agent == nil || s.Agent.CompactionPolicy == nil {
		s.reply("Compaction is not configured for this conversation, so /compact cannot run.")

		return true
	}

	// Force compaction regardless of threshold. Mark the session as a
	// slash-initiated compaction BEFORE dispatching so handleCompactionResult
	// knows to post the confirmation (and NOT resume the model conversation)
	// when the summarizer returns. The flag is JSON-serialized with the session
	// so it survives restore if the summarizer takes a while.
	s.SlashInitiatedCompaction = true

	// compactHistory() returns true when it dispatched the summarizer
	// sub-agent; in that case we return immediately to the async compaction
	// flow — the confirmation is posted by handleCompactionResult after the
	// summarizer completes (posting it here would be archived away). If it
	// could not run (no summarization agent, or history too short) we clear the
	// flag and reply with an explanation; either way we never call sendToDriver
	// or resume the model conversation.
	if s.compactHistory() {
		return true
	}

	// Compaction could not be dispatched; clear the flag and explain.
	s.SlashInitiatedCompaction = false
	s.reply("Compaction could not run: no summarization agent is configured, or the history is too short to compact.")

	return true
}
