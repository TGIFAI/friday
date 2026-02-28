package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/cronjob"
	"github.com/tgifai/friday/internal/pkg/logs"
)

// RegisterBuiltins registers all built-in commands on the hub.
func RegisterBuiltins(h *Hub) {
	h.Register(&Command{
		Name:        "/start",
		Description: "Start the bot and get a welcome message",
		Handler:     cmdStart,
	})
	h.Register(&Command{
		Name:        "/help",
		Description: "Show available commands",
		Handler:     cmdHelp,
	})
	h.Register(&Command{
		Name:        "/status",
		Description: "Show current agent and session status",
		Handler:     cmdStatus,
	})
	h.Register(&Command{
		Name:        "/cronjob",
		Description: "List all scheduled cron jobs",
		Handler:     cmdCronjob,
	})
	h.Register(&Command{
		Name:        "/new",
		Description: "Clear current session and start a new conversation",
		Handler:     cmdNew,
	})
}

func cmdStart(_ context.Context, _ HandlerDeps, _ *channel.Message) (string, error) {
	return "Welcome! I'm Friday, your AI assistant. How can I help you today?", nil
}

func cmdHelp(_ context.Context, deps HandlerDeps, _ *channel.Message) (string, error) {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, cmd := range deps.Commands().List() {
		fmt.Fprintf(&b, "  %s - %s\n", cmd.Name, cmd.Description)
	}
	return b.String(), nil
}

func cmdStatus(ctx context.Context, deps HandlerDeps, msg *channel.Message) (string, error) {
	ag, err := deps.GetAgentByChannel(msg.ChannelID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Agent: %s (%s)\n", ag.Name(), ag.ID())
	fmt.Fprintf(&b, "Channel: %s (%s)\n", msg.ChannelID, msg.ChannelType)
	fmt.Fprintf(&b, "Chat: %s\n", msg.ChatID)

	if s := cronjob.Default(); s != nil {
		jobs := s.ListJobs()
		fmt.Fprintf(&b, "Scheduled jobs: %d\n", len(jobs))
	}

	logs.CtxDebug(ctx, "[cmd:status] %s", b.String())
	return b.String(), nil
}

func cmdCronjob(_ context.Context, _ HandlerDeps, _ *channel.Message) (string, error) {
	s := cronjob.Default()
	if s == nil {
		return "Cron scheduler is not enabled", nil
	}
	return cronjob.FormatJobList(s.ListJobs()), nil
}

func cmdNew(ctx context.Context, deps HandlerDeps, msg *channel.Message) (string, error) {
	ag, err := deps.GetAgentByChannel(msg.ChannelID)
	if err != nil {
		return "", err
	}
	return ag.ResetSession(ctx, msg)
}
