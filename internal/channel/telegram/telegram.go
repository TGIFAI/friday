package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/pkg/utils"
	"github.com/tgifai/friday/internal/security/pairing"
)

var (
	_ channel.Channel = (*Telegram)(nil)

	parseMode = models.ParseModeMarkdown
)

const (
	defaultPairingPromptTemplate = ""
	pairCommandPrefix            = "/pair"
)

type Telegram struct {
	id      string
	config  Config
	bot     *bot.Bot
	pairing *pairing.Manager
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
	cfg.Security = chCfg.Security
	cfg.ACL = chCfg.ACL

	opts := []bot.Option{
		bot.WithDefaultHandler(defaultHandler),
	}

	tgBot, err := bot.New(cfg.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Telegram{
		id:      chanId,
		config:  *cfg,
		bot:     tgBot,
		ctx:     ctx,
		cancel:  cancel,
		pairing: pairing.Get(pairing.GetKey(string(channel.Telegram), chanId)),
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

	c.bot.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, c.handleStartCommand)
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

func (c *Telegram) handleStartCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}

	if c.isPairingEnabled() {
		if c.handlePairingIngress(ctx, b, msg) {
			return
		}
	} else {
		if !c.isUserAllowed(msg.From.ID) {
			return
		}
	}

	response := "Welcome! I'm Friday, your AI assistant. How can I help you today?"
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      response,
		ParseMode: parseMode,
	})
}

func (c *Telegram) handleMessage(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil || msg.Text == "" {
		return
	}

	if c.isPairingEnabled() {
		if c.handlePairingIngress(ctx, b, msg) {
			return
		}
	} else {
		if !c.isUserAllowed(msg.From.ID) {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.Chat.ID,
				Text:      "Sorry, you are not authorized to use this bot.",
				ParseMode: parseMode,
			})
			return
		}

		if msg.Chat.Type != "private" && !c.isGroupAllowed(msg.Chat.ID) {
			return
		}
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

	logs.CtxDebug(ctx, "[channel:telegram] received message from user %s (chat %s): %s",
		channelMsg.UserID, channelMsg.ChatID, utils.Truncate80(channelMsg.Content))

	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		if err := handler(ctx, channelMsg); err != nil {
			logs.CtxError(ctx, "Error handling message: %v", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.Chat.ID,
				Text:      "Sorry, an error occurred while processing your message.",
				ParseMode: parseMode,
			})
		}
	}
}

func (c *Telegram) isPairingEnabled() bool {
	return c.config.Security.Policy != "" && c.config.Security.Policy != consts.SecurityPolicySilent
}

func (c *Telegram) handlePairingIngress(ctx context.Context, b *bot.Bot, msg *models.Message) bool {
	chatKey := buildPairingChatKey(msg.Chat)
	if chatKey == "" {
		return false
	}

	userID := strconv.FormatInt(msg.From.ID, 10)
	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	allowed, err := c.pairing.IsAllowed(chatKey, userID)
	if err != nil {
		logs.CtxError(ctx, "[channel:telegram] pairing allowlist check failed: %v", err)
		return true
	}
	if allowed {
		return false
	}

	if code, ok := parsePairingCommand(msg.Text); ok {
		c.handlePairCommand(ctx, b, msg, chatKey, chatID, userID, code)
		return true
	}

	principal := c.buildPairingPrincipal(chatKey, userID)
	decision, err := c.pairing.EvaluateUnknownUser(principal, chatID, userID, defaultPairingPromptTemplate)
	if err != nil {
		logs.CtxError(ctx, "[channel:telegram] pairing evaluate failed: %v", err)
		return true
	}

	logs.CtxInfo(
		ctx,
		"[channel:telegram] channel_pairing_user_reached channel_id=%s user_id=%s chat_id=%s chat_key=%s req_id=%s pairing_code=%s expire_at=%s",
		c.id,
		userID,
		chatID,
		chatKey,
		decision.Challenge.ReqID,
		decision.Challenge.Code,
		decision.Challenge.ExpiresAt.Format(time.RFC3339),
	)

	if decision.Respond && strings.TrimSpace(decision.Message) != "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   decision.Message,
		})
	}
	return true
}

func (c *Telegram) handlePairCommand(
	ctx context.Context,
	b *bot.Bot,
	msg *models.Message,
	chatKey string,
	chatID string,
	userID string,
	code string,
) {
	principal := c.buildPairingPrincipal(chatKey, userID)
	challenge, err := c.pairing.VerifyCode(principal, code)
	if err != nil {
		logs.CtxInfo(
			ctx,
			"[channel:telegram] channel_pairing_result channel_id=%s user_id=%s chat_id=%s chat_key=%s req_id= success=false reason=%v",
			c.id,
			userID,
			chatID,
			chatKey,
			err,
		)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Invalid or expired pairing code.",
		})
		return
	}

	changed, grantErr := c.pairing.GrantACL(chatKey, userID)
	if grantErr != nil {
		logs.CtxError(ctx, "[channel:telegram] grant acl failed: %v", grantErr)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Pairing failed due to an internal error.",
		})
		return
	}

	resultReason := "ok"
	if !changed {
		resultReason = "already_granted"
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "Pairing successful. You can now use this bot.",
	})
	logs.CtxInfo(
		ctx,
		"[channel:telegram] channel_pairing_result channel_id=%s user_id=%s chat_id=%s chat_key=%s req_id=%s success=true reason=%s",
		c.id,
		userID,
		chatID,
		chatKey,
		challenge.ReqID,
		resultReason,
	)
}

func (c *Telegram) buildPairingPrincipal(chatKey string, userID string) string {
	return fmt.Sprintf("telegram:%s:%s:%s", c.id, chatKey, userID)
}

func buildPairingChatKey(chat models.Chat) string {
	chatID := strconv.FormatInt(chat.ID, 10)
	if strings.EqualFold(string(chat.Type), "private") {
		return "user:" + chatID
	}
	return "group:" + chatID
}

func parsePairingCommand(content string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(content))
	if len(fields) < 2 {
		return "", false
	}

	cmd := strings.ToLower(strings.TrimSpace(fields[0]))
	if strings.HasPrefix(cmd, pairCommandPrefix+"@") {
		cmd = pairCommandPrefix
	}
	if cmd != pairCommandPrefix {
		return "", false
	}

	code := strings.TrimSpace(fields[1])
	if code == "" {
		return "", false
	}
	return code, true
}

func (c *Telegram) isUserAllowed(userID int64) bool {
	if len(c.config.AllowedUsers) == 0 {
		return true
	}

	for _, allowedID := range c.config.AllowedUsers {
		if allowedID == userID {
			return true
		}
	}
	return false
}

func (c *Telegram) isGroupAllowed(groupID int64) bool {
	if len(c.config.AllowedGroups) == 0 {
		return true
	}

	for _, allowedID := range c.config.AllowedGroups {
		if allowedID == groupID {
			return true
		}
	}
	return false
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
