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
	var b strings.Builder
	b.WriteString("# Runtime Information\n")

	// ---- static section (per-agent, prompt-caching friendly) ----
	fmt.Fprintf(&b, "- agent: %s (id: %s)\n", ag.name, ag.id)
	fmt.Fprintf(&b, "- workspace: %s\n", ag.workspace)
	fmt.Fprintf(&b, "- version: %s\n", friday.VERSION)
	fmt.Fprintf(&b, "- platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- shell: %s\n", detectShell())

	// ---- dynamic section (per-request) ----
	fmt.Fprintf(&b, "- current time: %s\n", time.Now().Format(time.RFC3339))
	if msg != nil {
		fmt.Fprintf(&b, "- channel: %s (id: %s)\n", msg.ChannelType, msg.ChannelID)
		fmt.Fprintf(&b, "- chat id: %s\n", msg.ChatID)
		fmt.Fprintf(&b, "- user id: %s\n", msg.UserID)
	}

	return b.String()
}

// detectShell returns the name of the current user's shell.
func detectShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return filepath.Base(s)
	}
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "sh"
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

	// 2b. load daily memory (yesterday + today)
	ag.loadDailyMemory(&prompt, time.Now())

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
