package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/pkg/logs"
)

var (
	_ channel.Channel = (*Telegram)(nil)

	parseMode = models.ParseModeMarkdown
)

type Telegram struct {
	id      string
	config  Config
	bot     *bot.Bot
	handler func(ctx context.Context, msg *channel.Message) error
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewChannel(chanId string, chCfg *config.ChannelConfig) (channel.Channel, error) {
	cfg, err := ParseConfig(chCfg.Config)
	if err != nil {
		return nil, fmt.Errorf("parse telegram config: %w", err)
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(defaultHandler),
	}

	tgBot, err := bot.New(cfg.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Telegram{
		id:     chanId,
		config: *cfg,
		bot:    tgBot,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (c *Telegram) ID() string {
	return c.id
}

func (c *Telegram) Type() channel.Type {
	return channel.Telegram
}

func (c *Telegram) Start(ctx context.Context) error {
	if c.config.WebhookURL != "" && c.config.WebhookPort > 0 {
		return c.startWebhook(ctx)
	}

	return c.startPolling(ctx)
}

func (c *Telegram) startPolling(ctx context.Context) error {
	c.bot.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, c.handleMessage)
	c.bot.Start(ctx)
	return nil
}

func (c *Telegram) startWebhook(ctx context.Context) error {
	logs.CtxInfo(ctx, "Starting Telegram bot in webhook mode: %s", c.config.WebhookURL)

	_, err := c.bot.SetWebhook(ctx, &bot.SetWebhookParams{
		URL: c.config.WebhookURL,
	})
	if err != nil {
		return fmt.Errorf("failed to set webhook: %w", err)
	}

	return errors.New("webhook mode not yet fully implemented")
}

func (c *Telegram) Stop(ctx context.Context) error {
	c.cancel()
	if c.bot != nil {
		c.bot.Close(ctx)
	}
	return nil
}

func (c *Telegram) SendMessage(ctx context.Context, chatID string, content string) error {
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}

	entityText, entities := convertMarkdownEntities(content)
	if entityText == "" {
		entityText = content
	}

	_, err = c.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:   chatIDInt,
		Text:     entityText,
		Entities: entities,
	})

	if err != nil {
		logs.CtxWarn(ctx, "[channel:telegram] HTML parse failed, falling back to plain text: %v", err)
		_, err = c.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatIDInt,
			Text:   content,
		})
	}

	return err
}

func (c *Telegram) SendChatAction(ctx context.Context, chatID string, action channel.ChatAction) error {
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}

	tgAction, err := toTelegramChatAction(action)
	if err != nil {
		return err
	}

	ok, err := c.bot.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: chatIDInt,
		Action: tgAction,
	})
	if err != nil {
		return fmt.Errorf("failed to send chat action: %w", err)
	}
	if !ok {
		return errors.New("telegram send chat action failed")
	}

	return nil
}

func (c *Telegram) ReactMessage(ctx context.Context, chatID string, messageID string, reaction string) error {
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}

	messageIDInt, err := strconv.Atoi(messageID)
	if err != nil {
		return fmt.Errorf("invalid message ID: %w", err)
	}

	params := &bot.SetMessageReactionParams{
		ChatID:    chatIDInt,
		MessageID: messageIDInt,
	}
	if reaction != "" {
		params.Reaction = []models.ReactionType{
			{
				Type: models.ReactionTypeTypeEmoji,
				ReactionTypeEmoji: &models.ReactionTypeEmoji{
					Emoji: reaction,
				},
			},
		}
	} else {
		params.Reaction = []models.ReactionType{}
	}

	ok, err := c.bot.SetMessageReaction(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to set message reaction: %w", err)
	}
	if !ok {
		return errors.New("telegram set message reaction failed")
	}

	return nil
}

func (c *Telegram) RegisterMessageHandler(handler func(ctx context.Context, msg *channel.Message) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if handler == nil {
		return errors.New("handler cannot be nil")
	}

	c.handler = handler
	return nil
}

// handleMessage normalizes a Telegram update into a channel.Message and
// forwards it to the registered handler. All security, command routing, and
// ACL checks are performed by the gateway layer.
func (c *Telegram) handleMessage(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil || msg.Text == "" {
		return
	}

	messageID := strconv.Itoa(msg.ID)
	channelMsg := &channel.Message{
		ID:          messageID,
		ChannelID:   c.id,
		ChannelType: channel.Telegram,
		UserID:      strconv.FormatInt(msg.From.ID, 10),
		ChatID:      strconv.FormatInt(msg.Chat.ID, 10),
		Content:     msg.Text,
		Metadata: map[string]string{
			"message_id": messageID,
			"chat_type":  string(msg.Chat.Type),
			"username":   msg.From.Username,
			"first_name": msg.From.FirstName,
			"last_name":  msg.From.LastName,
		},
	}

	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		if err := handler(ctx, channelMsg); err != nil {
			logs.CtxError(ctx, "[channel:telegram] error handling message: %v", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.Chat.ID,
				Text:      "Sorry, an error occurred while processing your message.",
				ParseMode: parseMode,
			})
		}
	}
}

func defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {}

func toTelegramChatAction(action channel.ChatAction) (models.ChatAction, error) {
	switch action {
	case "", channel.ChatActionTyping:
		return models.ChatActionTyping, nil
	case channel.ChatActionUploadPhoto:
		return models.ChatActionUploadPhoto, nil
	case channel.ChatActionRecordVideo:
		return models.ChatActionRecordVideo, nil
	case channel.ChatActionUploadVideo:
		return models.ChatActionUploadVideo, nil
	case channel.ChatActionRecordVoice:
		return models.ChatActionRecordVoice, nil
	case channel.ChatActionUploadVoice:
		return models.ChatActionUploadVoice, nil
	case channel.ChatActionUploadDocument:
		return models.ChatActionUploadDocument, nil
	case channel.ChatActionChooseSticker:
		return models.ChatActionChooseSticker, nil
	case channel.ChatActionFindLocation:
		return models.ChatActionFindLocation, nil
	case channel.ChatActionRecordVideoNote:
		return models.ChatActionRecordVideoNote, nil
	case channel.ChatActionUploadVideoNote:
		return models.ChatActionUploadVideoNote, nil
	default:
		return "", fmt.Errorf("unsupported chat action: %s", action)
	}
}
