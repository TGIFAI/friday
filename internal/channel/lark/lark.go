package lark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/hertz/pkg/app"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	// maxImageSize is the upper bound for downloading images (3 MB).
	maxImageSize = 3 * 1024 * 1024
	// maxVoiceSize is the upper bound for downloading voice/audio (1 MB).
	maxVoiceSize = 1 * 1024 * 1024
)

var _ channel.Channel = (*Lark)(nil)

type Lark struct {
	id           string
	config       Config
	client       *lark.Client
	handler      func(ctx context.Context, msg *channel.Message) error
	mu           sync.RWMutex

	// webhook mode
	eventHandler app.HandlerFunc // nil in ws mode
	webhookPath  string          // empty in ws mode

	// ws mode
	wsClient *larkws.Client // nil in webhook mode
}

func NewChannel(chanId string, chCfg *config.ChannelConfig) (channel.Channel, error) {
	cfg, err := ParseConfig(chCfg.Config)
	if err != nil {
		return nil, fmt.Errorf("parse lark config: %w", err)
	}

	client := lark.NewClient(cfg.AppID, cfg.AppSecret)

	l := &Lark{
		id:     chanId,
		config: *cfg,
		client: client,
	}

	// Both modes share the same event dispatcher and message handler.
	eventDispatcher := dispatcher.NewEventDispatcher(cfg.VerificationToken, cfg.EncryptKey)
	eventDispatcher.OnP2MessageReceiveV1(l.onMessageReceive)

	switch cfg.Mode {
	case "ws":
		l.wsClient = larkws.NewClient(cfg.AppID, cfg.AppSecret,
			larkws.WithEventHandler(eventDispatcher),
		)
	default: // "webhook"
		l.webhookPath = fmt.Sprintf("/api/v1/lark/%s/event", chanId)
		l.eventHandler = l.newEventHandler(eventDispatcher)
	}

	return l, nil
}

// Routes implements channel.RouteProvider.
func (l *Lark) Routes() []channel.Route {
	if l.wsClient != nil {
		return nil
	}
	return []channel.Route{
		{Method: "POST", Path: l.webhookPath, Handler: l.eventHandler},
	}
}

func (l *Lark) ID() string {
	return l.id
}

func (l *Lark) Type() channel.Type {
	return channel.Lark
}

// Start blocks until the context is canceled. In webhook mode the route is
// already registered so we just wait. In ws mode we start the WebSocket client.
func (l *Lark) Start(ctx context.Context) error {
	if l.wsClient != nil {
		errCh := make(chan error, 1)
		go func() {
			errCh <- l.wsClient.Start(ctx)
		}()
		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return nil
		}
	}
	<-ctx.Done()
	return nil
}

func (l *Lark) Stop(_ context.Context) error {
	return nil
}

// maxPostContentSize is the upper bound for a Lark post message content (30 KB).
const maxPostContentSize = 30 * 1024

func (l *Lark) SendMessage(ctx context.Context, chatID string, content string) error {
	msgType, body, err := buildPostContent(content)
	if err != nil {
		return fmt.Errorf("build lark post content: %w", err)
	}

	resp, err := l.client.Im.Message.Create(ctx,
		larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(larkim.ReceiveIdTypeChatId).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				MsgType(msgType).
				ReceiveId(chatID).
				Content(body).
				Build()).
			Build())
	if err != nil {
		return fmt.Errorf("lark send message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("lark send message failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// buildPostContent converts markdown to a Lark post message. If the rendered
// post exceeds maxPostContentSize it falls back to plain text (truncated).
func buildPostContent(md string) (msgType string, body string, err error) {
	paragraphs := markdownToPost(md)

	post := map[string]interface{}{
		"zh_cn": map[string]interface{}{
			"content": paragraphs,
		},
	}
	serialized, err := sonic.MarshalString(post)
	if err != nil {
		return "", "", err
	}

	if len(serialized) <= maxPostContentSize {
		return larkim.MsgTypePost, serialized, nil
	}

	// Post too large — truncate paragraphs until it fits.
	for len(paragraphs) > 1 {
		paragraphs = paragraphs[:len(paragraphs)-1]
		post["zh_cn"] = map[string]interface{}{
			"content": append(paragraphs, []postElement{
				{"tag": "text", "text": "… [truncated]"},
			}),
		}
		serialized, err = sonic.MarshalString(post)
		if err != nil {
			return "", "", err
		}
		if len(serialized) <= maxPostContentSize {
			return larkim.MsgTypePost, serialized, nil
		}
	}

	// Still too large — fall back to plain text, truncated.
	text := md
	if len(text) > maxPostContentSize-20 {
		text = text[:maxPostContentSize-20] + "… [truncated]"
	}
	plain, err := sonic.MarshalString(map[string]string{"text": text})
	if err != nil {
		return "", "", err
	}
	return larkim.MsgTypeText, plain, nil
}

func (l *Lark) SendChatAction(_ context.Context, _ string, _ channel.ChatAction) error {
	return channel.ErrUnsupportedOperation
}

func (l *Lark) ReactMessage(ctx context.Context, _ string, messageID string, reaction string) error {
	resp, err := l.client.Im.MessageReaction.Create(ctx,
		larkim.NewCreateMessageReactionReqBuilder().
			MessageId(messageID).
			Body(larkim.NewCreateMessageReactionReqBodyBuilder().
				ReactionType(larkim.NewEmojiBuilder().EmojiType(reaction).Build()).
				Build()).
			Build())
	if err != nil {
		return fmt.Errorf("lark react message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("lark react message failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

func (l *Lark) RegisterMessageHandler(handler func(ctx context.Context, msg *channel.Message) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if handler == nil {
		return errors.New("handler cannot be nil")
	}
	l.handler = handler
	return nil
}

// onMessageReceive is the SDK callback for im.message.receive_v1.
func (l *Lark) onMessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	msg := event.Event.Message
	if msg == nil || msg.MessageId == nil {
		return nil
	}

	msgType := ""
	if msg.MessageType != nil {
		msgType = *msg.MessageType
	}

	var content string
	var attachments []channel.Attachment

	switch msgType {
	case "text":
		text, err := extractText(msg.Content)
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] failed to extract text: %v", err)
			return nil
		}
		// Strip @mention placeholders (e.g. @_user_1) from group messages.
		content = stripMentionPlaceholders(text, msg.Mentions)

	case "image":
		imageKey, err := extractKey(msg.Content, "image_key")
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] failed to extract image_key: %v", err)
			return nil
		}
		att, err := l.downloadResource(ctx, *msg.MessageId, imageKey, "image", channel.AttachmentImage, "image/png")
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] download image: %v", err)
		} else if att != nil {
			attachments = append(attachments, *att)
		}

	case "audio":
		fileKey, err := extractKey(msg.Content, "file_key")
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] failed to extract audio file_key: %v", err)
			return nil
		}
		att, err := l.downloadResource(ctx, *msg.MessageId, fileKey, "file", channel.AttachmentVoice, "audio/ogg")
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] download audio: %v", err)
		} else if att != nil {
			attachments = append(attachments, *att)
		}

	default:
		logs.CtxDebug(ctx, "[channel:lark] ignoring message type: %s", msgType)
		return nil
	}

	if content == "" && len(attachments) == 0 {
		return nil
	}

	var userID string
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil && event.Event.Sender.SenderId.OpenId != nil {
		userID = *event.Event.Sender.SenderId.OpenId
	}

	var chatID string
	if msg.ChatId != nil {
		chatID = *msg.ChatId
	}

	metadata := map[string]string{}
	if msg.MessageType != nil {
		metadata["message_type"] = *msg.MessageType
	}
	if msg.ChatType != nil {
		metadata["chat_type"] = *msg.ChatType
	}

	channelMsg := &channel.Message{
		ID:          *msg.MessageId,
		ChannelID:   l.id,
		ChannelType: channel.Lark,
		UserID:      userID,
		ChatID:      chatID,
		Content:     content,
		Metadata:    metadata,
		Attachments: attachments,
	}

	l.mu.RLock()
	handler := l.handler
	l.mu.RUnlock()

	if handler != nil {
		if err := handler(ctx, channelMsg); err != nil {
			logs.CtxError(ctx, "[channel:lark] error handling message: %v", err)
		}
	}
	return nil
}

// downloadResource downloads a message resource (image or file) from Lark.
// Returns nil (no error) if the downloaded data exceeds the size limit.
func (l *Lark) downloadResource(ctx context.Context, messageID, fileKey, resType string, attType channel.AttachmentType, defaultMIME string) (*channel.Attachment, error) {
	resp, err := l.client.Im.MessageResource.Get(ctx,
		larkim.NewGetMessageResourceReqBuilder().
			MessageId(messageID).
			FileKey(fileKey).
			Type(resType).
			Build())
	if err != nil {
		return nil, fmt.Errorf("get resource: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("get resource failed: code=%d msg=%s", resp.Code, resp.Msg)
	}

	data, err := io.ReadAll(resp.File)
	if err != nil {
		return nil, fmt.Errorf("read resource body: %w", err)
	}

	var limit int
	switch attType {
	case channel.AttachmentImage:
		limit = maxImageSize
	default:
		limit = maxVoiceSize
	}
	if len(data) > limit {
		logs.CtxDebug(ctx, "[channel:lark] resource too large (%d bytes), skipping", len(data))
		return nil, nil
	}

	return &channel.Attachment{
		Type:     attType,
		Data:     data,
		MIMEType: defaultMIME,
		FileName: resp.FileName,
	}, nil
}

// newEventHandler returns a Hertz handler that adapts the SDK event dispatcher.
func (l *Lark) newEventHandler(eventDispatcher *dispatcher.EventDispatcher) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		body, err := io.ReadAll(c.GetRequest().BodyStream())
		if err != nil {
			// Fall back to reading the buffered body.
			body = c.GetRequest().Body()
		}
		if len(body) == 0 {
			body = c.GetRequest().Body()
		}

		// Build larkevent.EventReq from the Hertz request.
		header := make(http.Header)
		c.GetRequest().Header.VisitAll(func(key, value []byte) {
			header.Set(string(key), string(value))
		})

		eventReq := &larkevent.EventReq{
			Header:     header,
			Body:       body,
			RequestURI: string(c.GetRequest().RequestURI()),
		}

		eventResp := eventDispatcher.Handle(ctx, eventReq)

		c.SetStatusCode(eventResp.StatusCode)
		for key, values := range eventResp.Header {
			for _, value := range values {
				c.Response.Header.Set(key, value)
			}
		}
		c.Response.SetBody(eventResp.Body)
	}
}

// stripMentionPlaceholders removes @mention placeholders (e.g. @_user_1)
// from Lark message text. In group chats, Lark replaces @mentions with
// placeholder keys in the content and provides a Mentions list.
func stripMentionPlaceholders(text string, mentions []*larkim.MentionEvent) string {
	for _, m := range mentions {
		if m.Key == nil || *m.Key == "" {
			continue
		}
		text = strings.ReplaceAll(text, *m.Key, "")
	}
	return strings.TrimSpace(text)
}

// extractText parses the JSON content field from a Lark message.
// Lark wraps text messages as {"text":"actual text"}.
func extractText(content *string) (string, error) {
	if content == nil || *content == "" {
		return "", nil
	}
	var parsed struct {
		Text string `json:"text"`
	}
	if err := sonic.UnmarshalString(*content, &parsed); err != nil {
		return "", fmt.Errorf("unmarshal lark message content: %w", err)
	}
	return parsed.Text, nil
}

// extractKey parses a named key from the JSON content field.
// Lark uses {"image_key":"..."} for images, {"file_key":"..."} for audio/files.
func extractKey(content *string, key string) (string, error) {
	if content == nil || *content == "" {
		return "", fmt.Errorf("content is empty")
	}
	var parsed map[string]string
	if err := sonic.UnmarshalString(*content, &parsed); err != nil {
		return "", fmt.Errorf("unmarshal content: %w", err)
	}
	val, ok := parsed[key]
	if !ok || val == "" {
		return "", fmt.Errorf("key %q not found in content", key)
	}
	return val, nil
}
