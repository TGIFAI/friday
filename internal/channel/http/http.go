package http

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/google/uuid"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	// maxImageSize is the upper bound for inline images (3 MB raw).
	maxImageSize = 3 * 1024 * 1024
	// maxVoiceSize is the upper bound for inline audio (1 MB raw).
	maxVoiceSize = 1 * 1024 * 1024
	// responseTimeout is how long the HTTP handler waits for the agent reply.
	responseTimeout = 5 * time.Minute
)

var _ channel.Channel = (*HTTP)(nil)

// inboundRequest is the JSON body expected on the message endpoint.
type inboundRequest struct {
	UserID      string              `json:"user_id"`
	ChatID      string              `json:"chat_id"`
	Content     string              `json:"content"`
	Attachments []inboundAttachment `json:"attachments,omitempty"`
	Metadata    map[string]string   `json:"metadata,omitempty"`
}

type inboundAttachment struct {
	Type     string `json:"type"`      // "image" or "voice"
	Data     string `json:"data"`      // base64-encoded binary
	MIMEType string `json:"mime_type"` // e.g. "image/jpeg"
	FileName string `json:"file_name,omitempty"`
}

// outboundResponse is the JSON body returned to the caller.
type outboundResponse struct {
	ID       string            `json:"id"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// pendingReply is a channel through which the gateway delivers the agent
// response back to the waiting HTTP handler.
type pendingReply struct {
	ch      chan string
	created time.Time
}

type HTTP struct {
	id      string
	config  Config
	handler func(ctx context.Context, msg *channel.Message) error
	mu      sync.RWMutex

	// pending maps a request-scoped chatID to its reply channel.
	// The gateway calls SendMessage with this chatID to deliver the response.
	pendingMu   sync.Mutex
	pending     map[string]*pendingReply
	messagePath string
}

func NewChannel(chanId string, chCfg *config.ChannelConfig) (channel.Channel, error) {
	cfg, err := ParseConfig(chCfg.Config)
	if err != nil {
		return nil, fmt.Errorf("parse http config: %w", err)
	}

	h := &HTTP{
		id:          chanId,
		config:      *cfg,
		pending:     make(map[string]*pendingReply),
		messagePath: fmt.Sprintf("/api/v1/http/%s/message", chanId),
	}

	return h, nil
}

// Routes implements channel.RouteProvider.
func (h *HTTP) Routes() []channel.Route {
	return []channel.Route{
		{Method: "POST", Path: h.messagePath, Handler: h.handleMessage},
	}
}

func (h *HTTP) ID() string         { return h.id }
func (h *HTTP) Type() channel.Type { return channel.HTTP }

func (h *HTTP) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (h *HTTP) Stop(_ context.Context) error {
	return nil
}

// SendMessage delivers the agent reply to the pending HTTP request identified
// by chatID. If no pending request is found (e.g. timed out), the message is
// silently dropped.
func (h *HTTP) SendMessage(_ context.Context, chatID string, content string) error {
	h.pendingMu.Lock()
	pr, ok := h.pending[chatID]
	if ok {
		delete(h.pending, chatID)
	}
	h.pendingMu.Unlock()

	if !ok {
		return nil // request already timed out, nothing to deliver
	}

	// Non-blocking send; if the channel is full the response is lost (shouldn't
	// happen with a buffer of 1).
	select {
	case pr.ch <- content:
	default:
	}
	return nil
}

func (h *HTTP) SendChatAction(_ context.Context, _ string, _ channel.ChatAction) error {
	return channel.ErrUnsupportedOperation
}

func (h *HTTP) ReactMessage(_ context.Context, _ string, _ string, _ string) error {
	return channel.ErrUnsupportedOperation
}

func (h *HTTP) RegisterMessageHandler(handler func(ctx context.Context, msg *channel.Message) error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if handler == nil {
		return errors.New("handler cannot be nil")
	}
	h.handler = handler
	return nil
}

// handleMessage is the Hertz handler for incoming HTTP messages.
func (h *HTTP) handleMessage(ctx context.Context, c *app.RequestContext) {
	// --- auth ---
	if h.config.APIKey != "" {
		auth := string(c.GetHeader("Authorization"))
		if auth != "Bearer "+h.config.APIKey {
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
	}

	// --- parse body ---
	var req inboundRequest
	if err := sonic.Unmarshal(c.GetRequest().Body(), &req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Content == "" && len(req.Attachments) == 0 {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "content or attachments required"})
		return
	}

	// --- build channel.Message ---
	requestID := uuid.New().String()
	chatID := requestID // use unique request ID as chatID for reply routing

	attachments, err := h.decodeAttachments(req.Attachments)
	if err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = make(map[string]string)
	}

	channelMsg := &channel.Message{
		ID:          requestID,
		ChannelID:   h.id,
		ChannelType: channel.HTTP,
		UserID:      req.UserID,
		ChatID:      chatID,
		Content:     req.Content,
		Metadata:    metadata,
		Attachments: attachments,
	}

	// --- register pending reply ---
	pr := &pendingReply{
		ch:      make(chan string, 1),
		created: time.Now(),
	}
	h.pendingMu.Lock()
	h.pending[chatID] = pr
	h.pendingMu.Unlock()

	// Ensure cleanup on any exit path.
	defer func() {
		h.pendingMu.Lock()
		delete(h.pending, chatID)
		h.pendingMu.Unlock()
	}()

	// --- enqueue ---
	h.mu.RLock()
	handler := h.handler
	h.mu.RUnlock()

	if handler == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "no handler registered"})
		return
	}
	if err := handler(ctx, channelMsg); err != nil {
		logs.CtxError(ctx, "[channel:http] error enqueuing message: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "failed to process message"})
		return
	}

	// --- wait for reply ---
	select {
	case content := <-pr.ch:
		resp := outboundResponse{
			ID:      requestID,
			Content: content,
		}
		body, _ := sonic.Marshal(resp)
		c.SetStatusCode(consts.StatusOK)
		c.SetContentType("application/json")
		c.Response.SetBody(body)

	case <-time.After(responseTimeout):
		c.JSON(consts.StatusGatewayTimeout, map[string]string{"error": "response timeout"})

	case <-ctx.Done():
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "server shutting down"})
	}
}

// decodeAttachments converts inbound base64 attachments to channel.Attachment,
// applying the same size guards as Telegram and Lark channels.
func (h *HTTP) decodeAttachments(in []inboundAttachment) ([]channel.Attachment, error) {
	if len(in) == 0 {
		return nil, nil
	}

	var out []channel.Attachment
	for _, a := range in {
		attType := channel.AttachmentType(a.Type)
		switch attType {
		case channel.AttachmentImage, channel.AttachmentVoice:
		default:
			return nil, fmt.Errorf("unsupported attachment type: %s", a.Type)
		}

		data, err := base64.StdEncoding.DecodeString(a.Data)
		if err != nil {
			return nil, fmt.Errorf("decode base64 attachment: %w", err)
		}

		// Size guard.
		switch attType {
		case channel.AttachmentImage:
			if len(data) > maxImageSize {
				continue // skip oversized, don't fail
			}
		case channel.AttachmentVoice:
			if len(data) > maxVoiceSize {
				continue
			}
		}

		out = append(out, channel.Attachment{
			Type:     attType,
			Data:     data,
			MIMEType: a.MIMEType,
			FileName: a.FileName,
		})
	}
	return out, nil
}
