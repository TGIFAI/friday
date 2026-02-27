package cronjob

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tgifai/friday/internal/consts"
)

const (
	// FlushJobID is the reserved job ID prefix for the memory flush job.
	FlushJobID = "__memory_flush__"
	// FlushJobName is the human-readable name for the flush job.
	FlushJobName = "memory-flush"
	// flushCronSchedule runs at 01:45 daily (15 min before compact at 02:00).
	flushCronSchedule = "45 1 * * *"
)

// NewFlushJob creates the nightly pre-compaction memory flush job for the
// given agent. It runs at 01:45 daily, 15 minutes before the compact job.
func NewFlushJob(agentID, workspace string) Job {
	now := time.Now()
	return Job{
		ID:            flushJobID(agentID),
		Name:          FlushJobName,
		AgentID:       agentID,
		ScheduleType:  ScheduleCron,
		Schedule:      flushCronSchedule,
		SessionTarget: SessionIsolated,
		Enabled:       true,
		Workspace:     workspace,
		CreatedAt:     now,
	}
}

func flushJobID(agentID string) string {
	return FlushJobID + ":" + agentID
}

// IsFlushJob reports whether the job ID belongs to a memory flush job.
func IsFlushJob(jobID string) bool {
	return strings.HasPrefix(jobID, FlushJobID)
}

// BuildFlushPrompt assembles a prompt instructing the agent to review recent
// conversation context and write a summary to today's daily memory file and
// durable facts to MEMORY.md. Returns ("", false) if there is no session
// activity worth flushing.
func BuildFlushPrompt(workspace string, now time.Time) (string, bool) {
	today := now
	dateStr := today.Format("2006-01-02")

	// Check for today's session activity.
	sessionsDir := filepath.Join(workspace, "memory", "sessions")
	excerpts, _ := ExtractUserMessages(sessionsDir, today, maxUserMessagesPerSession)

	if len(excerpts) == 0 {
		return "", false
	}

	// Read existing daily file to include for context (prevent duplicates).
	dailyPath := filepath.Join(workspace, consts.DailyMemoryFile(today))
	dailyContent, dailyErr := os.ReadFile(dailyPath)
	dailyText := ""
	if dailyErr == nil {
		dailyText = strings.TrimSpace(string(dailyContent))
	}

	// Read existing MEMORY.md for context (prevent duplicates).
	memoryPath := filepath.Join(workspace, consts.WorkspaceMemoryFile)
	memoryContent, memErr := os.ReadFile(memoryPath)
	memoryText := ""
	if memErr == nil {
		memoryText = strings.TrimSpace(string(memoryContent))
	}

	var b strings.Builder
	b.Grow(4096)

	fmt.Fprintf(&b, "# Memory Flush — %s\n\n", dateStr)
	b.WriteString("You are performing a memory flush task. Review the session activity below and preserve important context.\n\n")

	// Include session excerpts.
	b.WriteString("## Today's Session Activity\n\n")
	totalChars := 0
	for _, ex := range excerpts {
		if totalChars >= maxTotalExcerptChars {
			break
		}
		fmt.Fprintf(&b, "### Session: %s\n\n", ex.SessionKey)
		for _, msg := range ex.Messages {
			if totalChars >= maxTotalExcerptChars {
				break
			}
			truncated := msg
			if utf8.RuneCountInString(truncated) > maxSingleMessageChars {
				runes := []rune(truncated)
				truncated = string(runes[:maxSingleMessageChars]) + "..."
			}
			fmt.Fprintf(&b, "- %s\n", truncated)
			totalChars += len(truncated)
		}
		b.WriteString("\n")
	}

	// Show existing daily file for context.
	if dailyText != "" {
		fmt.Fprintf(&b, "## Existing Daily File (`memory/daily/%s.md`)\n\n", dateStr)
		b.WriteString("```\n")
		b.WriteString(dailyText)
		b.WriteString("\n```\n\n")
	}

	// Show existing MEMORY.md for context.
	if memoryText != "" {
		const maxMemoryChars = 4000
		b.WriteString("## Existing Memory File (`memory/MEMORY.md`)\n\n")
		b.WriteString("```\n")
		if len(memoryText) > maxMemoryChars {
			b.WriteString(memoryText[:maxMemoryChars])
			b.WriteString("\n... (truncated)\n")
		} else {
			b.WriteString(memoryText)
		}
		b.WriteString("\n```\n\n")
	}

	// Instructions.
	b.WriteString("## Instructions\n\n")
	b.WriteString("Perform these two tasks using the workspace file tools:\n\n")
	fmt.Fprintf(&b, "**Task 1: Update daily file** — Edit `memory/daily/%s.md`:\n", dateStr)
	b.WriteString("- Append a summary of today's key events, decisions, and outcomes from session activity.\n")
	b.WriteString("- Do not duplicate entries already present in the file.\n")
	b.WriteString("- Use concise bullet points with timestamps where meaningful.\n\n")
	b.WriteString("**Task 2: Extract persistent facts** — Edit `memory/MEMORY.md`:\n")
	b.WriteString("- Identify any durable knowledge from the session activity:\n")
	b.WriteString("  user preferences, project decisions, contact info, recurring patterns.\n")
	b.WriteString("- Append new facts to the appropriate section in MEMORY.md.\n")
	b.WriteString("- Do not duplicate information already in MEMORY.md.\n")
	b.WriteString("- If nothing is worth persisting, skip this task.\n\n")
	b.WriteString("Be concise and factual. Do not add commentary beyond the file edits.\n")

	return b.String(), true
}
