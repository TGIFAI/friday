package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/channel/telegram"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
)

var msgHwd = &MsgRunner{}

type MsgRunner struct{}

func (r *MsgRunner) cmd() *cli.Command {
	return &cli.Command{
		Name:  "msg",
		Usage: "Send a one-off message through a configured channel",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "channelId",
				Aliases: []string{"chanId"},
				Usage:   "Channel ID defined in the config file",
			},
			&cli.StringFlag{
				Name:  "chatId",
				Usage: "Target chat ID or user ID",
			},
			&cli.StringFlag{
				Name:    "content",
				Aliases: []string{"m"},
				Usage:   "Message body",
			},
		},
		Action: r.run,
	}
}

func (r *MsgRunner) run(ctx context.Context, cmd *cli.Command) error {
	channelID := strings.TrimSpace(cmd.String("channelId"))
	if channelID == "" {
		return errors.New("--channelId is required")
	}
	chatID := strings.TrimSpace(cmd.String("chatId"))
	if chatID == "" {
		return errors.New("--chatId is required")
	}
	content := strings.TrimSpace(cmd.String("content"))
	if content == "" {
		return errors.New("--content cannot be empty")
	}

	cfg, err := config.Load(consts.DefaultConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	chCfg, ok := cfg.Channels[channelID]
	if !ok {
		return fmt.Errorf("channel %q was not found in the configured channels", channelID)
	}

	switch channel.Type(strings.ToLower(strings.TrimSpace(chCfg.Type))) {
	case channel.Telegram:
		ch, err := telegram.NewChannel(channelID, &chCfg)
		if err != nil {
			return fmt.Errorf("create telegram channel: %w", err)
		}
		defer func() { _ = ch.Stop(ctx) }()

		if err := ch.SendMessage(ctx, chatID, content); err != nil {
			return fmt.Errorf("send telegram message: %w", err)
		}
	default:
		return fmt.Errorf("channel type %q is not supported by the msg command yet", chCfg.Type)
	}

	fmt.Printf("Sent message via %s channel %s to target %s\n", chCfg.Type, chCfg.ID, chatID)
	return nil
}
