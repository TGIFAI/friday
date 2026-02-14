package msg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/channel/telegram"
	"github.com/tgifai/friday/internal/config"
)

var (
	Command = &cli.Command{
		Name:  "msg",
		Usage: "Send a one-off message through a configured channel",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to the runtime config file",
				Value:   "config.yaml",
			},
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
		Action: runMessage,
	}
)

func runMessage(ctx context.Context, cmd *cli.Command) error {
	channelID := strings.TrimSpace(cmd.String("channelId"))
	if channelID == "" {
		return errors.New("--channelId is required")
	}

	chatID := strings.TrimSpace(cmd.String("chatId"))
	if len(chatID) == 0 {
		return errors.New("--chatId is required")
	}

	content := strings.TrimSpace(cmd.String("content"))
	if content == "" {
		return errors.New("--content cannot be empty")
	}

	cfgPath := strings.TrimSpace(cmd.String("config"))
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	chCfg, ok := cfg.Channels[channelID]
	if !ok {
		return fmt.Errorf("channel %q was not found in the configured channels", channelID)
	}
	if err := sendMessage(ctx, chCfg, chatID, content); err != nil {
		return err
	}

	fmt.Printf("Sent message via %s channel %s to target %s\n", chCfg.Type, chCfg.ID, chatID)
	return nil
}

func sendMessage(ctx context.Context, chCfg config.ChannelConfig, chatID string, content string) error {
	switch channel.Type(strings.ToLower(strings.TrimSpace(chCfg.Type))) {
	case channel.Telegram:
		tgCfg, err := telegram.ParseConfig(chCfg.Config)
		if err != nil {
			return fmt.Errorf("parse telegram config: %w", err)
		}

		ch, err := telegram.NewChannel(chCfg.ID, tgCfg)
		if err != nil {
			return fmt.Errorf("create telegram channel: %w", err)
		}
		defer func() {
			_ = ch.Stop(ctx)
		}()

		if err := ch.SendMessage(ctx, chatID, content); err != nil {
			return fmt.Errorf("send telegram message: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("channel type %q is not supported by the msg command yet", chCfg.Type)
	}
}
