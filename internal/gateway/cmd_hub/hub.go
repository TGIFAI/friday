package cmd_hub

import (
	"context"
	"strings"
	"sync"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/pkg/logs"
)

// HandlerFunc processes a matched command and returns a text reply.
// An empty reply means no response should be sent.
type HandlerFunc func(ctx context.Context, deps HandlerDeps, msg *channel.Message) (string, error)

// AgentInfo exposes the agent methods that built-in command handlers need.
type AgentInfo interface {
	ID() string
	Name() string
	ResetSession(ctx context.Context, msg *channel.Message) (string, error)
}

// HandlerDeps is the dependency interface for command handlers, implemented
// by the gateway. This breaks the circular dependency between cmd_hub and
// the gateway package.
type HandlerDeps interface {
	GetAgentByChannel(channelID string) (AgentInfo, error)
	Commands() *Hub
}

// Command describes a single channel-agnostic command.
type Command struct {
	Name        string      // e.g. "/start"
	Description string      // short help text
	Handler     HandlerFunc // execution logic
}

// Hub is a thread-safe registry that matches incoming message text against
// registered command prefixes and dispatches the first match. It replaces
// the former CommandRouter and adds the ability to sync commands to
// platform-native UIs (e.g. Telegram bot menus).
type Hub struct {
	commands map[string]*Command // key: lowercase command name
	mu       sync.RWMutex
}

// NewHub creates an empty command hub.
func NewHub() *Hub {
	return &Hub{commands: make(map[string]*Command, 8)}
}

// Register adds a command to the hub.
func (h *Hub) Register(cmd *Command) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commands[strings.ToLower(cmd.Name)] = cmd
}

// Unregister removes a command by name.
func (h *Hub) Unregister(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.commands, strings.ToLower(name))
}

// Match checks whether content starts with a known command.
// It returns the matched command, the remaining arguments, and whether a match
// was found. Commands are matched case-insensitively and may include a
// trailing @botname suffix (e.g. "/start@mybot").
func (h *Hub) Match(content string) (*Command, string, bool) {
	content = strings.TrimSpace(content)
	if content == "" || content[0] != '/' {
		return nil, "", false
	}

	fields := strings.SplitN(content, " ", 2)
	raw := strings.ToLower(fields[0])

	// Strip @botname suffix: "/start@mybot" → "/start"
	if idx := strings.Index(raw, "@"); idx > 0 {
		raw = raw[:idx]
	}

	h.mu.RLock()
	cmd, ok := h.commands[raw]
	h.mu.RUnlock()

	if !ok {
		return nil, "", false
	}

	args := ""
	if len(fields) > 1 {
		args = strings.TrimSpace(fields[1])
	}
	return cmd, args, true
}

// List returns all registered commands.
func (h *Hub) List() []*Command {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Command, 0, len(h.commands))
	for _, cmd := range h.commands {
		out = append(out, cmd)
	}
	return out
}

// SyncToChannels pushes the current command list to every channel that
// implements the CommandRegistrar interface.
func (h *Hub) SyncToChannels(ctx context.Context) {
	cmds := h.List()
	botCmds := make([]channel.BotCommand, len(cmds))
	for i, cmd := range cmds {
		botCmds[i] = channel.BotCommand{
			Command:     strings.TrimPrefix(cmd.Name, "/"),
			Description: cmd.Description,
		}
	}

	for _, ch := range channel.List() {
		reg, ok := ch.(channel.CommandRegistrar)
		if !ok {
			continue
		}
		if err := reg.SetCommands(ctx, botCmds); err != nil {
			logs.CtxWarn(ctx, "[cmd_hub] sync commands to channel %s: %v", ch.ID(), err)
		} else {
			logs.CtxInfo(ctx, "[cmd_hub] synced %d commands to channel %s", len(botCmds), ch.ID())
		}
	}
}

// ResetChannelCommands removes all registered commands from channels that
// implement the CommandRegistrar interface.
func (h *Hub) ResetChannelCommands(ctx context.Context) {
	for _, ch := range channel.List() {
		reg, ok := ch.(channel.CommandRegistrar)
		if !ok {
			continue
		}
		if err := reg.DeleteCommands(ctx); err != nil {
			logs.CtxWarn(ctx, "[cmd_hub] reset commands on channel %s: %v", ch.ID(), err)
		}
	}
}
