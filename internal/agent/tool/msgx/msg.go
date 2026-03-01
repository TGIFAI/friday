package msgx

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/pkg/logs"
)

type MessageTool struct{}

func NewMessageTool() *MessageTool {
	return &MessageTool{}
}

func (t *MessageTool) Name() string {
	return "message"
}

func (t *MessageTool) Description() string {
	return "Send a message to a specific channel/chat"
}

func (t *MessageTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"chanId": {
				Type:     schema.String,
				Desc:     "Channel ID to send the message to",
				Required: true,
			},
			"chatId": {
				Type:     schema.String,
				Desc:     "Chat ID within the channel",
				Required: true,
			},
			"content": {
				Type:     schema.String,
				Desc:     "Message content to send",
				Required: true,
			},
		}),
	}
}

func (t *MessageTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	chanID := getStringArg(args, "chanId", "chan_id", "channelId", "channel_id")
	if chanID == "" {
		return nil, fmt.Errorf("chanId is required")
	}
	chatID := getStringArg(args, "chatId", "chat_id")
	if chatID == "" {
		return nil, fmt.Errorf("chatId is required")
	}
	content := getStringArg(args, "content")
	if content == "" {
		return nil, fmt.Errorf("send_message: missing required parameter 'content'")
	}
	ch, err := channel.Get(chanID)
	if err != nil {
		return nil, fmt.Errorf("channel not found: %s", chanID)
	}
	if err := ch.SendMessage(ctx, chatID, content); err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}
	logs.CtxInfo(ctx, "[tool:message] sent to chan=%s chat=%s content_len=%d", chanID, chatID, len(content))
	return map[string]interface{}{
		"success": true,
		"chanId":  chanID,
		"chatId":  chatID,
		"content": content,
	}, nil
}

func getStringArg(args map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := args[key]; ok {
			s := strings.TrimSpace(gconv.To[string](v))
			if s != "" {
				return s
			}
		}
	}
	return ""
}
