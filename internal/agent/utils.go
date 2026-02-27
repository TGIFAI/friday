package agent

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cloudwego/eino/schema"
	"github.com/tgifai/friday/internal/channel"
)

// buildUserMessage constructs a schema.Message from a channel message.
func buildUserMessage(msg *channel.Message) *schema.Message {
	if len(msg.Attachments) == 0 {
		return &schema.Message{Role: schema.User, Content: msg.Content}
	}

	var parts []schema.MessageInputPart

	if msg.Content != "" {
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeText,
			Text: msg.Content,
		})
	}

	for _, att := range msg.Attachments {
		b64 := base64.StdEncoding.EncodeToString(att.Data)
		switch att.Type {
		case channel.AttachmentImage:
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						Base64Data: &b64,
						MIMEType:   att.MIMEType,
					},
					Detail: schema.ImageURLDetailAuto,
				},
			})
		case channel.AttachmentVoice:
			// Most LLM providers (Anthropic, Volcengine, etc.) do not support
			// audio_url content parts. Instead of sending an unsupported type
			// that would cause the entire request to fail, we add a text note
			// indicating an audio message was received.
			name := att.FileName
			if name == "" {
				name = "audio"
			}
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeText,
				Text: fmt.Sprintf("[Audio attachment received: %s (%s), but audio input is not supported by the current model]", name, att.MIMEType),
			})
		}
	}

	return &schema.Message{
		Role:                  schema.User,
		UserInputMultiContent: parts,
	}
}

// defaultShell returns the name of the current user's shell.
func defaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return filepath.Base(s)
	}
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "sh"
}
