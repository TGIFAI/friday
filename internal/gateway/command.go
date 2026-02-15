package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/pkg/logs"
)

// CommandHandlerFunc processes a matched command and returns a text reply.
// An empty reply means no response should be sent.
type CommandHandlerFunc func(ctx context.Context, gw *Gateway, msg *channel.Message) (string, error)

// Command describes a single channel-agnostic command.
type Command struct {
	Name        string             // e.g. "/start"
	Description string             // short help text
	Handler     CommandHandlerFunc // execution logic
}

// CommandRouter is a thread-safe registry that matches incoming message text
// against registered command prefixes and dispatches the first match.
type CommandRouter struct {
	commands map[string]*Command // key: lowercase command name
	mu       sync.RWMutex
}

func newCommandRouter() *CommandRouter {
	return &CommandRouter{commands: make(map[string]*Command, 8)}
}

// Register adds a command to the router.
func (r *CommandRouter) Register(cmd *Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[strings.ToLower(cmd.Name)] = cmd
}

// Match checks whether content starts with a known command.
// It returns the matched command, the remaining arguments, and whether a match
// was found. Commands are matched case-insensitively and may include a
// trailing @botname suffix (e.g. "/start@mybot").
func (r *CommandRouter) Match(content string) (*Command, string, bool) {
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

	r.mu.RLock()
	cmd, ok := r.commands[raw]
	r.mu.RUnlock()

	if !ok {
		return nil, "", false
	}

	args := ""
	if len(fields) > 1 {
		args = strings.TrimSpace(fields[1])
	}
	return cmd, args, true
}

// List returns all registered commands (for help text or native command registration).
func (r *CommandRouter) List() []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		out = append(out, cmd)
	}
	return out
}

// ---------------------------------------------------------------------------
// Built-in commands
// ---------------------------------------------------------------------------

func registerBuiltinCommands(r *CommandRouter) {
	r.Register(&Command{
		Name:        "/start",
		Description: "Start the bot and get a welcome message",
		Handler:     cmdStart,
	})
	r.Register(&Command{
		Name:        "/help",
		Description: "Show available commands",
		Handler:     cmdHelp,
	})
	r.Register(&Command{
		Name:        "/status",
		Description: "Show current agent and session status",
		Handler:     cmdStatus,
	})
}

func cmdStart(_ context.Context, _ *Gateway, _ *channel.Message) (string, error) {
	return "Welcome! I'm Friday, your AI assistant. How can I help you today?", nil
}

func cmdHelp(_ context.Context, gw *Gateway, _ *channel.Message) (string, error) {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, cmd := range gw.commands.List() {
		fmt.Fprintf(&b, "  %s - %s\n", cmd.Name, cmd.Description)
	}
	return b.String(), nil
}

func cmdStatus(ctx context.Context, gw *Gateway, msg *channel.Message) (string, error) {
	ag, err := gw.getAgentByChannel(msg.ChannelID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Agent: %s (%s)\n", ag.Name(), ag.ID())
	fmt.Fprintf(&b, "Channel: %s (%s)\n", msg.ChannelID, msg.ChannelType)
	fmt.Fprintf(&b, "Chat: %s\n", msg.ChatID)

	if gw.scheduler != nil {
		jobs := gw.scheduler.ListJobs()
		fmt.Fprintf(&b, "Scheduled jobs: %d\n", len(jobs))
	}

	logs.CtxDebug(ctx, "[cmd:status] %s", b.String())
	return b.String(), nil
}
