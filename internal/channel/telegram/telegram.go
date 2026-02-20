package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// typingInterval is how often the typing indicator is refreshed.
	// Telegram's typing status expires after ~5 seconds.
	typingInterval = 3 * time.Second
)

var (
	_ channel.Channel = (*Telegram)(nil)

	parseMode = models.ParseModeMarkdown
)

type Telegram struct {
	id          string
	config      Config
	bot         *bot.Bot
	botUsername string // lowercase bot username for mention matching
	botUserID   int64  // bot user ID for text_mention matching
	handler     func(ctx context.Context, msg *channel.Message) error
	mediaGroups *mediaGroupAggregator
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
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
	tg.mediaGroups = newMediaGroupAggregator(tg.flushMediaGroup)

	opts := []bot.Option{
		bot.WithDefaultHandler(tg.handleUpdate),
	}

	tgBot, err := bot.New(cfg.Token, opts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}
	tg.bot = tgBot

	// Fetch bot identity for mention matching in group chats.
	me, err := tgBot.GetMe(ctx)
	if err != nil {
		logs.Warn("[channel:telegram] GetMe failed, group mention filtering disabled: %v", err)
	} else {
		tg.botUsername = strings.ToLower(me.Username)
		tg.botUserID = me.ID
		logs.Info("[channel:telegram] bot identity: @%s (id=%d)", me.Username, me.ID)
	}

	return tg, nil
}

func (c *Telegram) ID() string {
	return c.id
}

func (c *Telegram) Type() channel.Type {
	return channel.Telegram
}

func (c *Telegram) Routes() []channel.Route {
	return []channel.Route{
		//{Method: "POST", Path: l.webhookPath, Handler: l.eventHandler},
	}
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

func (c *Telegram) WorkInProgress(ctx context.Context, chatID string, _ string) (func(), error) {
	_ = c.SendChatAction(ctx, chatID, channel.ChatActionTyping)

	ticker := time.NewTicker(typingInterval)
	done := make(chan struct{})

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = c.SendChatAction(ctx, chatID, channel.ChatActionTyping)
			}
		}
	}()

	return func() { close(done) }, nil
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

	// Compute mention status once (used by both group filter and media group).
	mentioned := c.isBotMentioned(msg.Text, msg.Entities) ||
		c.isBotMentioned(msg.Caption, msg.CaptionEntities)

	// In group/supergroup chats, only process messages that mention the bot.
	if c.botUsername != "" && isGroupChat(msg.Chat.Type) && !mentioned {
		// For media groups, a later update in the same group might carry the
		// caption with the @mention. We cannot know yet, so we must still
		// check if this belongs to an existing pending group.
		if msg.MediaGroupID != "" {
			// If there's already a pending group that was marked as mentioned,
			// we should still accept this photo.
			c.mediaGroups.mu.Lock()
			pg, exists := c.mediaGroups.groups[msg.MediaGroupID]
			alreadyMentioned := exists && pg.mentioned
			c.mediaGroups.mu.Unlock()
			if !alreadyMentioned {
				// Buffer it anyway — the caption (with @mention) might arrive
				// in a later update within this media group.
				c.mediaGroups.add(msg, false)
				return
			}
			// Fall through to add it to the existing mentioned group.
		} else {
			return
		}
	}

	// Media group: aggregate multiple photo updates into one message.
	if msg.MediaGroupID != "" {
		c.mediaGroups.add(msg, mentioned)
		return
	}

	// --- single message path ---
	c.processSingleUpdate(ctx, b, msg)
}

// processSingleUpdate handles a non-media-group update.
func (c *Telegram) processSingleUpdate(ctx context.Context, b *bot.Bot, msg *models.Message) {
	// Determine textual content: prefer Text, fall back to Caption.
	content := msg.Text
	if content == "" {
		content = msg.Caption
	}

	// Strip bot @mention from content so the agent sees clean text.
	if c.botUsername != "" && isGroupChat(msg.Chat.Type) {
		content = c.stripBotMention(content)
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

	channelMsg := c.buildChannelMessage(msg, content, attachments)
	c.dispatchMessage(ctx, b, msg.Chat.ID, channelMsg)
}

// flushMediaGroup is called by the aggregator after the debounce window.
// It downloads all buffered photos and dispatches a single merged message.
func (c *Telegram) flushMediaGroup(pg *pendingMediaGroup) {
	ctx := c.ctx

	// In group chats, drop the entire group if no update had an @mention.
	if c.botUsername != "" && isGroupChat(pg.chat.Type) && !pg.mentioned {
		logs.CtxDebug(ctx, "[channel:telegram] media group dropped: no bot mention")
		return
	}

	content := pg.caption
	if c.botUsername != "" && isGroupChat(pg.chat.Type) {
		content = c.stripBotMention(content)
	}

	// Download all photos concurrently.
	type photoResult struct {
		idx int
		att *channel.Attachment
	}
	results := make(chan photoResult, len(pg.photos))
	var wg sync.WaitGroup
	for i, photo := range pg.photos {
		if int64(photo.FileSize) > maxImageSize {
			logs.CtxDebug(ctx, "[channel:telegram] media group photo too large (%d bytes), skipping", photo.FileSize)
			continue
		}
		wg.Add(1)
		go func(idx int, fileID string) {
			defer wg.Done()
			data, err := c.downloadFile(ctx, fileID)
			if err != nil {
				logs.CtxWarn(ctx, "[channel:telegram] media group download photo: %v", err)
				return
			}
			results <- photoResult{idx: idx, att: &channel.Attachment{
				Type:     channel.AttachmentImage,
				Data:     data,
				MIMEType: "image/jpeg",
			}}
		}(i, photo.FileID)
	}
	wg.Wait()
	close(results)

	// Collect results and sort by original order.
	collected := make([]photoResult, 0, len(pg.photos))
	for r := range results {
		collected = append(collected, r)
	}
	// Sort by index to preserve the order photos were received.
	for i := 0; i < len(collected); i++ {
		for j := i + 1; j < len(collected); j++ {
			if collected[j].idx < collected[i].idx {
				collected[i], collected[j] = collected[j], collected[i]
			}
		}
	}
	attachments := make([]channel.Attachment, 0, len(collected))
	for _, r := range collected {
		attachments = append(attachments, *r.att)
	}

	if content == "" && len(attachments) == 0 {
		return
	}

	// Build a synthetic models.Message for buildChannelMessage.
	syntheticMsg := &models.Message{
		ID:   pg.firstMessageID,
		Chat: pg.chat,
		From: pg.from,
	}

	channelMsg := c.buildChannelMessage(syntheticMsg, content, attachments)
	channelMsg.Metadata["media_group"] = "true"

	logs.CtxInfo(ctx, "[channel:telegram] media group flushed: %d photos, caption=%q",
		len(attachments), content)

	c.dispatchMessage(ctx, nil, pg.chat.ID, channelMsg)
}

// buildChannelMessage constructs a channel.Message from a Telegram message.
func (c *Telegram) buildChannelMessage(msg *models.Message, content string, attachments []channel.Attachment) *channel.Message {
	messageID := strconv.Itoa(msg.ID)
	metadata := map[string]string{
		"message_id": messageID,
		"chat_type":  string(msg.Chat.Type),
	}
	if msg.From != nil {
		metadata["username"] = msg.From.Username
		metadata["first_name"] = msg.From.FirstName
		metadata["last_name"] = msg.From.LastName
	}
	if msg.ForwardOrigin != nil {
		metadata["forwarded"] = "true"
	}

	userID := ""
	if msg.From != nil {
		userID = strconv.FormatInt(msg.From.ID, 10)
	}

	return &channel.Message{
		ID:          messageID,
		ChannelID:   c.id,
		ChannelType: channel.Telegram,
		UserID:      userID,
		ChatID:      strconv.FormatInt(msg.Chat.ID, 10),
		Content:     content,
		Metadata:    metadata,
		Attachments: attachments,
	}
}

// dispatchMessage sends the message to the registered handler.
func (c *Telegram) dispatchMessage(ctx context.Context, b *bot.Bot, chatID int64, channelMsg *channel.Message) {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		if err := handler(ctx, channelMsg); err != nil {
			logs.CtxError(ctx, "[channel:telegram] error handling message: %v", err)
			if b != nil {
				_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID:    chatID,
					Text:      "Sorry, an error occurred while processing your message.",
					ParseMode: parseMode,
				})
			}
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

// isGroupChat returns true for group and supergroup chat types.
func isGroupChat(chatType models.ChatType) bool {
	return chatType == models.ChatTypeGroup || chatType == models.ChatTypeSupergroup
}

// isBotMentioned checks whether entities contain a mention of this bot.
func (c *Telegram) isBotMentioned(text string, entities []models.MessageEntity) bool {
	for _, e := range entities {
		switch e.Type {
		case models.MessageEntityTypeMention:
			// Extract @username from the text using offset/length (byte-safe via runes).
			runes := []rune(text)
			if e.Offset >= 0 && e.Offset+e.Length <= len(runes) {
				mentioned := strings.ToLower(string(runes[e.Offset : e.Offset+e.Length]))
				if mentioned == "@"+c.botUsername {
					return true
				}
			}
		case models.MessageEntityTypeTextMention:
			// text_mention has a User object directly.
			if e.User != nil && e.User.ID == c.botUserID {
				return true
			}
		}
	}
	return false
}

// stripBotMention removes @botUsername from content and trims whitespace.
func (c *Telegram) stripBotMention(content string) string {
	// Case-insensitive replacement of @botUsername.
	lower := strings.ToLower(content)
	mention := "@" + c.botUsername
	for {
		idx := strings.Index(lower, mention)
		if idx < 0 {
			break
		}
		content = content[:idx] + content[idx+len(mention):]
		lower = lower[:idx] + lower[idx+len(mention):]
	}
	return strings.TrimSpace(content)
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
