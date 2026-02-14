package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/pkg/logs"
)

func (ag *Agent) buildMessages(sess *session.Session, msg *channel.Message, includeCurrentUser bool) []*schema.Message {

	msgs := make([]*schema.Message, 0, 32)
	systemPrompt := ag.buildRuntimeInformation(msg)

	// 1. build system prompt
	if prompt, _ := ag.buildSystemPrompt(); len(prompt) > 0 {
		systemPrompt += "\n\n" + prompt
	}

	msgs = append(msgs, &schema.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	// 2. append history messages
	if sess != nil {
		msgs = append(msgs, sess.History()...)
	}

	return msgs
}

func (ag *Agent) buildRuntimeInformation(msg *channel.Message) string {
	formatValue := func(value string) string {
		if strings.TrimSpace(value) == "" {
			return "N/A"
		}
		return value
	}

	channelType := "N/A"
	channelID := "N/A"
	chatID := "N/A"
	userID := "N/A"
	sessionKey := "N/A"

	if msg != nil {
		channelType = formatValue(string(msg.ChannelType))
		channelID = formatValue(msg.ChannelID)
		chatID = formatValue(msg.ChatID)
		userID = formatValue(msg.UserID)
		sessionKey = formatValue(msg.SessionKey)
	}

	prompt := fmt.Sprintf(
		"# Runtime Information\n- goos: %s\n- goarch: %s\n- session key: %s\n- channel type: %s\n- channel id: %s\n- chat id: %s\n- user id: %s",
		runtime.GOOS, runtime.GOARCH, sessionKey, channelType, channelID, chatID, userID,
	)

	prompt += "\n- current time: " + time.Now().Format(time.RFC3339)

	return prompt
}

func (ag *Agent) buildSystemPrompt() (string, error) {
	var prompt strings.Builder
	prompt.Grow(1 << 11)

	// 1. load bootstrap prompt files (without memory)
	for _, name := range consts.WorkspaceMarkdownFiles {
		content, err := os.ReadFile(filepath.Join(ag.workspace, name))
		if err != nil {
			logs.Warn("[agent:%s] failed to read prompt file %s: %v", ag.id, name, err)
			continue
		}
		if text := strings.TrimSpace(string(content)); text != "" {
			if prompt.Len() > 0 {
				prompt.WriteString("\n\n")
			}
			prompt.WriteString(text)
		}
	}

	// 2. load memory "memory/MEMORY.md"
	content, err := os.ReadFile(filepath.Join(ag.workspace, consts.WorkspaceMemoryFile))
	if err != nil {
		logs.Warn("[agent:%s] failed to read memory file: %v", ag.id, err)
	}

	if len(content) > 0 {
		if text := strings.TrimSpace(string(content)); text != "" {
			if prompt.Len() > 0 {
				prompt.WriteString("\n\n")
			}
			prompt.WriteString(text)
		}
	}

	// 3. load built-in skills
	if builtInSkills, _ := ag.skills.GetBuiltInSkills(); len(builtInSkills) > 0 {
		if text := ag.skills.BuildPrompt(builtInSkills); text != "" {
			if prompt.Len() > 0 {
				prompt.WriteString("\n\n")
			}
			prompt.WriteString(text)
		}
	}

	prompt.WriteString("\n\n---\n\n")
	return prompt.String(), nil
}
