package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	// maxImageSize is the upper bound for downloading images (3 MB).
	maxImageSize int64 = 3 * 1024 * 1024
	// maxVoiceSize is the upper bound for downloading voice/audio (1 MB).
	maxVoiceSize int64 = 1 * 1024 * 1024
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

	ctx, cancel := context.WithCancel(context.Background())

	tg := &Telegram{
		id:     chanId,
		config: *cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(tg.handleUpdate),
	}

	tgBot, err := bot.New(cfg.Token, opts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}
	tg.bot = tgBot

	return tg, nil
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

// handleUpdate is the default handler for all incoming Telegram updates.
// It normalizes text, photo, voice, and audio messages into a channel.Message
// and forwards them to the registered handler.
func (c *Telegram) handleUpdate(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}

	// Determine textual content: prefer Text, fall back to Caption.
	content := msg.Text
	if content == "" {
		content = msg.Caption
	}

	// Extract attachments from media fields.
	var attachments []channel.Attachment

	if len(msg.Photo) > 0 {
		att, err := c.extractPhoto(ctx, msg.Photo)
		if err != nil {
			logs.CtxWarn(ctx, "[channel:telegram] download photo: %v", err)
		} else if att != nil {
			attachments = append(attachments, *att)
		}
	}

	if msg.Voice != nil {
		att, err := c.extractVoice(ctx, msg.Voice)
		if err != nil {
			logs.CtxWarn(ctx, "[channel:telegram] download voice: %v", err)
		} else if att != nil {
			attachments = append(attachments, *att)
		}
	}

	if msg.Audio != nil {
		att, err := c.extractAudio(ctx, msg.Audio)
		if err != nil {
			logs.CtxWarn(ctx, "[channel:telegram] download audio: %v", err)
		} else if att != nil {
			attachments = append(attachments, *att)
		}
	}

	// Drop the message if there is no text and no attachment.
	if content == "" && len(attachments) == 0 {
		return
	}

	messageID := strconv.Itoa(msg.ID)
	metadata := map[string]string{
		"message_id": messageID,
		"chat_type":  string(msg.Chat.Type),
		"username":   msg.From.Username,
		"first_name": msg.From.FirstName,
		"last_name":  msg.From.LastName,
	}
	if msg.ForwardOrigin != nil {
		metadata["forwarded"] = "true"
	}

	channelMsg := &channel.Message{
		ID:          messageID,
		ChannelID:   c.id,
		ChannelType: channel.Telegram,
		UserID:      strconv.FormatInt(msg.From.ID, 10),
		ChatID:      strconv.FormatInt(msg.Chat.ID, 10),
		Content:     content,
		Metadata:    metadata,
		Attachments: attachments,
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

// extractPhoto picks the largest photo size and downloads it if within the
// size limit. Returns nil (no error) when the photo is too large.
func (c *Telegram) extractPhoto(ctx context.Context, photos []models.PhotoSize) (*channel.Attachment, error) {
	if len(photos) == 0 {
		return nil, nil
	}
	// Telegram sends multiple sizes; the last one is the largest.
	best := photos[len(photos)-1]
	if int64(best.FileSize) > maxImageSize {
		logs.CtxDebug(ctx, "[channel:telegram] photo too large (%d bytes), skipping", best.FileSize)
		return nil, nil
	}
	data, err := c.downloadFile(ctx, best.FileID)
	if err != nil {
		return nil, err
	}
	return &channel.Attachment{
		Type:     channel.AttachmentImage,
		Data:     data,
		MIMEType: "image/jpeg",
	}, nil
}

// extractVoice downloads a voice message if within the size limit.
func (c *Telegram) extractVoice(ctx context.Context, v *models.Voice) (*channel.Attachment, error) {
	if v.FileSize > maxVoiceSize {
		logs.CtxDebug(ctx, "[channel:telegram] voice too large (%d bytes), skipping", v.FileSize)
		return nil, nil
	}
	data, err := c.downloadFile(ctx, v.FileID)
	if err != nil {
		return nil, err
	}
	mime := v.MimeType
	if mime == "" {
		mime = "audio/ogg"
	}
	return &channel.Attachment{
		Type:     channel.AttachmentVoice,
		Data:     data,
		MIMEType: mime,
	}, nil
}

// extractAudio downloads an audio file if within the size limit.
func (c *Telegram) extractAudio(ctx context.Context, a *models.Audio) (*channel.Attachment, error) {
	if a.FileSize > maxVoiceSize {
		logs.CtxDebug(ctx, "[channel:telegram] audio too large (%d bytes), skipping", a.FileSize)
		return nil, nil
	}
	data, err := c.downloadFile(ctx, a.FileID)
	if err != nil {
		return nil, err
	}
	mime := a.MimeType
	if mime == "" {
		mime = "audio/mpeg"
	}
	return &channel.Attachment{
		Type:     channel.AttachmentVoice,
		Data:     data,
		MIMEType: mime,
		FileName: a.FileName,
	}, nil
}

// downloadFile retrieves a file from the Telegram servers by file ID.
func (c *Telegram) downloadFile(ctx context.Context, fileID string) ([]byte, error) {
	file, err := c.bot.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}
	link := c.bot.FileDownloadLink(file)

	resp, err := http.Get(link) //nolint:gosec // URL comes from Telegram API
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download file: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file body: %w", err)
	}
	return data, nil
}

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
