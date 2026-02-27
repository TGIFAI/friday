package lark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/hertz/pkg/app"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
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
	id      string
	config  Config
	client  *lark.Client
	handler func(ctx context.Context, msg *channel.Message) error
	mu      sync.RWMutex

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

	larkLogger := logs.NewLarkLogger(logs.DefaultLogger())
	larkLogLevel := larkcore.LogLevelInfo

	client := lark.NewClient(cfg.AppID, cfg.AppSecret,
		lark.WithLogger(larkLogger),
		lark.WithLogLevel(larkLogLevel),
	)

	l := &Lark{
		id:     chanId,
		config: *cfg,
		client: client,
	}

	// Both modes share the same event dispatcher and message handler.
	eventDispatcher := dispatcher.NewEventDispatcher(cfg.VerificationToken, cfg.EncryptKey)
	eventDispatcher.InitConfig(
		larkevent.WithLogger(larkLogger),
		larkevent.WithLogLevel(larkLogLevel),
	)
	eventDispatcher.OnP2MessageReceiveV1(l.onMessageReceive)

	switch strings.ToLower(cfg.Mode) {
	case "ws":
		l.wsClient = larkws.NewClient(cfg.AppID, cfg.AppSecret,
			larkws.WithEventHandler(eventDispatcher),
			larkws.WithLogger(larkLogger),
			larkws.WithLogLevel(larkLogLevel),
		)
	default: // "webhook"
		l.webhookPath = fmt.Sprintf("/api/v1/lark/%s/webhook", chanId)
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

// wsConnectTimeout is how long we wait for the initial WS connection before
// assuming success. The Lark SDK's Start() blocks forever on success (empty
// select{}) and only returns on connection failure, so a timeout firing
// without an error means the connection is established.
const wsConnectTimeout = 10 * time.Second

// Start blocks until the context is canceled. In webhook mode the route is
// already registered so we just wait. In ws mode we start the WebSocket client.
func (l *Lark) Start(ctx context.Context) error {
	if l.wsClient != nil {
		return l.startWS(ctx)
	}
	// webhook mode — routes are already registered; wait for shutdown.
	<-ctx.Done()
	return nil
}

func (l *Lark) startWS(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		// NOTE: The Lark SDK's Start() blocks forever on success via an
		// empty `select{}` that does NOT check ctx.Done(). It only returns
		// when connect() (or reconnect()) fails. This goroutine will leak
		// on context cancellation — acceptable since the process is shutting
		// down at that point.
		errCh <- l.wsClient.Start(ctx)
	}()

	// The SDK's connect() does a synchronous HTTP request followed by a
	// gorilla/websocket Dial (which has NO context support). If the Lark
	// endpoint is unreachable the dial can hang indefinitely. Use a timer
	// to detect successful connection vs a hung dial.
	timer := time.NewTimer(wsConnectTimeout)
	defer timer.Stop()

	select {
	case err := <-errCh:
		return fmt.Errorf("lark ws connect: %w", err)
	case <-timer.C:
		// No error within timeout — connection succeeded and the SDK is
		// now in its blocking select{} loop, actively receiving messages.
		logs.CtxInfo(ctx, "[channel:lark] ws connected")
	case <-ctx.Done():
		return nil
	}

	// Stay alive: surface any late errors from the SDK (e.g. reconnect
	// failure after an unexpected disconnect), or return cleanly on shutdown.
	select {
	case err := <-errCh:
		return fmt.Errorf("lark ws disconnected: %w", err)
	case <-ctx.Done():
		return nil
	}
}

func (l *Lark) Stop(_ context.Context) error {
	return nil
}

// maxPostContentSize is the upper bound for a Lark post message content (30 KB).
const maxPostContentSize = 30 * 1024

func (l *Lark) SendMessage(ctx context.Context, chatID, content string) error {
	// refer to https://open.feishu.cn/document/server-docs/im-v1/message-content-description/create_json#45e0953e
	post := map[string]interface{}{
		"zh_cn": map[string]interface{}{
			"content": [][]map[string]any{
				{
					{"tag": "md", "text": content},
				},
			},
		},
	}
	serialized, _ := sonic.MarshalString(post)

	resp, err := l.client.Im.Message.Create(ctx,
		larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(larkim.ReceiveIdTypeChatId).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				MsgType(larkim.MsgTypePost).
				ReceiveId(chatID).
				Content(serialized).
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

func (l *Lark) SendChatAction(_ context.Context, _ string, _ channel.ChatAction) error {
	return channel.ErrUnsupportedOperation
}

func (l *Lark) WorkInProgress(ctx context.Context, _ string, messageID string) (func(), error) {
	resp, err := l.client.Im.MessageReaction.Create(ctx,
		larkim.NewCreateMessageReactionReqBuilder().
			MessageId(messageID).
			Body(larkim.NewCreateMessageReactionReqBodyBuilder().
				ReactionType(larkim.NewEmojiBuilder().EmojiType("OnIt").Build()).
				Build()).
			Build())
	if err != nil || !resp.Success() {
		return func() {}, nil
	}

	reactionID := *resp.Data.ReactionId

	return func() {
		delCtx, delCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer delCancel()
		// Remove "OnIt" reaction.
		_, _ = l.client.Im.MessageReaction.Delete(delCtx,
			larkim.NewDeleteMessageReactionReqBuilder().
				MessageId(messageID).
				ReactionId(reactionID).
				Build())
		// Add "DONE" reaction.
		_, _ = l.client.Im.MessageReaction.Create(delCtx,
			larkim.NewCreateMessageReactionReqBuilder().
				MessageId(messageID).
				Body(larkim.NewCreateMessageReactionReqBodyBuilder().
					ReactionType(larkim.NewEmojiBuilder().EmojiType("DONE").Build()).
					Build()).
				Build())
	}, nil
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
// It dispatches to handleMessage in a goroutine to avoid blocking the SDK's
// WebSocket receive loop. Blocking here would stall ping/pong frames and
// prevent subsequent messages from being processed, which is the root cause
// of the "Lark WS gets stuck" symptom.
func (l *Lark) onMessageReceive(_ context.Context, event *larkim.P2MessageReceiveV1) error {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		l.handleMessage(ctx, event)
	}()
	return nil
}

func (l *Lark) handleMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) {
	msg := event.Event.Message
	if msg == nil || msg.MessageId == nil {
		return
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
			return
		}
		// Strip @mention placeholders (e.g. @_user_1) from group messages.
		content = stripMentionPlaceholders(text, msg.Mentions)

	case "post":
		text, imageKeys, err := extractPost(msg.Content)
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] failed to extract post: %v", err)
			return
		}
		content = stripMentionPlaceholders(text, msg.Mentions)
		for _, key := range imageKeys {
			att, dlErr := l.downloadResource(ctx, *msg.MessageId, key, "image", channel.AttachmentImage, "image/png")
			if dlErr != nil {
				logs.CtxWarn(ctx, "[channel:lark] download post image: %v", dlErr)
			} else if att != nil {
				attachments = append(attachments, *att)
			}
		}

	case "image":
		imageKey, err := extractKey(msg.Content, "image_key")
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] failed to extract image_key: %v", err)
			return
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
			return
		}
		att, err := l.downloadResource(ctx, *msg.MessageId, fileKey, "file", channel.AttachmentVoice, "audio/ogg")
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] download audio: %v", err)
		} else if att != nil {
			attachments = append(attachments, *att)
		}

	case "file":
		fileKey, err := extractKey(msg.Content, "file_key")
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] failed to extract file key: %v", err)
			return
		}
		att, err := l.downloadResource(ctx, *msg.MessageId, fileKey, "file", channel.AttachmentFile, "application/octet-stream")
		if err != nil {
			logs.CtxWarn(ctx, "[channel:lark] download file: %v", err)
		} else if att != nil {
			attachments = append(attachments, *att)
		}

	default:
		logs.CtxDebug(ctx, "[channel:lark] ignoring message type: %s", msgType)
		return
	}

	if content == "" && len(attachments) == 0 {
		return
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

// extractPost parses a Lark "post" (rich text) message content.
// The format is {"<locale>":{"title":"...","content":[[elements]]}}.
// It flattens all text/link/at elements into plain text and collects image keys.
func extractPost(content *string) (text string, imageKeys []string, err error) {
	if content == nil || *content == "" {
		return "", nil, nil
	}

	// Post content is keyed by locale (zh_cn, en_us, ja_jp, etc.).
	var locales map[string]struct {
		Title   string              `json:"title"`
		Content [][]postContentItem `json:"content"`
	}
	if err := sonic.UnmarshalString(*content, &locales); err != nil {
		return "", nil, fmt.Errorf("unmarshal post content: %w", err)
	}

	// Pick the first available locale.
	for _, locale := range locales {
		var b strings.Builder
		if locale.Title != "" {
			b.WriteString(locale.Title)
			b.WriteByte('\n')
		}
		for i, paragraph := range locale.Content {
			if i > 0 {
				b.WriteByte('\n')
			}
			for _, elem := range paragraph {
				switch elem.Tag {
				case "text":
					b.WriteString(elem.Text)
				case "a":
					if elem.Href != "" {
						b.WriteString(elem.Text + "(" + elem.Href + ")")
					} else {
						b.WriteString(elem.Text)
					}
				case "at":
					if elem.UserName != "" {
						b.WriteString("@" + elem.UserName)
					} else if elem.UserID != "" {
						b.WriteString("@" + elem.UserID)
					}
				case "img":
					if elem.ImageKey != "" {
						imageKeys = append(imageKeys, elem.ImageKey)
					}
				}
			}
		}
		return strings.TrimSpace(b.String()), imageKeys, nil
	}

	return "", nil, nil
}

// postContentItem represents a single element inside a Lark post paragraph.
type postContentItem struct {
	Tag      string `json:"tag"`
	Text     string `json:"text,omitempty"`
	Href     string `json:"href,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	UserName string `json:"user_name,omitempty"`
	ImageKey string `json:"image_key,omitempty"`
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
