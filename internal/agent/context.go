package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday"
	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/provider"
)

func (ag *Agent) buildMessages(sess *session.Session, msg *channel.Message) []*schema.Message {
	msgs := make([]*schema.Message, 0, 32)

	// System ①: built-in definitions (binary-stable, highest cache value)
	if text := ag.buildBuiltinPrompt(); text != "" {
		msgs = append(msgs, &schema.Message{Role: schema.System, Content: text, Extra: map[string]any{provider.L0Cache: true}})
	}

	// System ②: workspace persona (user-editable, rarely changes)
	if text := ag.buildWorkspacePrompt(); text != "" {
		msgs = append(msgs, &schema.Message{Role: schema.System, Content: text, Extra: map[string]any{provider.L1Cache: true}})
	}

	// System ③: dynamic context (changes per-day or per-request)
	if text := ag.buildDynamicPrompt(msg); text != "" {
		msgs = append(msgs, &schema.Message{Role: schema.System, Content: text, Extra: map[string]any{provider.L2Cache: true}})
	}

	// Session history
	if sess != nil {
		msgs = append(msgs, sess.History()...)
	}

	return msgs
}

// buildBuiltinPrompt returns the binary-stable system prompt containing
// built-in tool definitions and built-in skills. This content only changes
// when the binary is rebuilt, making it ideal for prefix caching.
func (ag *Agent) buildBuiltinPrompt() string {
	var b strings.Builder
	b.Grow(1 << 11)

	// Built-in tool definitions from embedded const.
	if text := strings.TrimSpace(consts.WorkspaceToolsTemplate); text != "" {
		b.WriteString(text)
	}

	// Built-in skills.
	if builtInSkills, _ := ag.skills.GetBuiltInSkills(); len(builtInSkills) > 0 {
		if text := ag.skills.BuildPrompt(builtInSkills); text != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(text)
		}
	}

	return b.String()
}

// buildWorkspacePrompt returns the workspace persona prompt assembled from
// user-editable markdown files. This content changes only when the user
// edits configuration files, so it stays stable across normal conversations.
func (ag *Agent) buildWorkspacePrompt() string {
	var b strings.Builder

	for _, name := range consts.WorkspaceMarkdownFiles {
		content, err := os.ReadFile(filepath.Join(ag.workspace, name))
		if err != nil {
			logs.Warn("[agent:%s] failed to read prompt file %s: %v", ag.id, name, err)
			continue
		}
		if text := strings.TrimSpace(string(content)); text != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(text)
		}
	}

	return b.String()
}

// buildDynamicPrompt returns the per-request dynamic context including
// runtime information, long-term memory, and daily memory. This is placed
// last among system messages so that the more stable prefixes remain cacheable.
func (ag *Agent) buildDynamicPrompt(msg *channel.Message) string {
	var b strings.Builder

	// Runtime information.
	b.WriteString("# Runtime Information\n")
	fmt.Fprintf(&b, "- agent: %s (id: %s)\n", ag.name, ag.id)
	fmt.Fprintf(&b, "- workspace: %s\n", ag.workspace)
	fmt.Fprintf(&b, "- version: %s\n", friday.VERSION)
	fmt.Fprintf(&b, "- platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- shell: %s\n", defaultShell())
	if msg != nil {
		fmt.Fprintf(&b, "- channel: %s (id: %s)\n", msg.ChannelType, msg.ChannelID)
		fmt.Fprintf(&b, "- chat id: %s\n", msg.ChatID)
		fmt.Fprintf(&b, "- user id: %s\n", msg.UserID)
	}

	// Long-term memory (memory/MEMORY.md).
	content, err := os.ReadFile(filepath.Join(ag.workspace, consts.WorkspaceMemoryFile))
	if err != nil {
		logs.Warn("[agent:%s] failed to read memory file: %v", ag.id, err)
	}
	if text := strings.TrimSpace(string(content)); text != "" {
		b.WriteString("\n\n")
		b.WriteString(text)
	}

	// Daily memory (yesterday + today).
	ag.loadDailyMemory(&b, time.Now())

	return b.String()
}

// loadDailyMemory appends yesterday's and today's daily memory files to the
// system prompt, giving the agent short-term temporal context.
func (ag *Agent) loadDailyMemory(prompt *strings.Builder, now time.Time) {
	dates := []time.Time{now.AddDate(0, 0, -1), now} // yesterday + today
	for _, d := range dates {
		relPath := consts.DailyMemoryFile(d)
		content, err := os.ReadFile(filepath.Join(ag.workspace, relPath))
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(content))
		if text == "" {
			continue
		}
		if prompt.Len() > 0 {
			prompt.WriteString("\n\n")
		}
		fmt.Fprintf(prompt, "<!-- daily memory: %s -->\n%s", d.Format("2006-01-02"), text)
	}
}
