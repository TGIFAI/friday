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
	// CompactJobID is the reserved job ID prefix for the memory compact job.
	CompactJobID = "__memory_compact__"
	// CompactJobName is the human-readable name for the compact job.
	CompactJobName = "memory-compact"
	// compactCronSchedule runs at 02:00 daily.
	compactCronSchedule = "0 2 * * *"

	maxUserMessagesPerSession = 20
	maxTotalExcerptChars      = 16000
	maxSingleMessageChars     = 500
)

// NewCompactJob creates the nightly memory compaction job for the given agent.
func NewCompactJob(agentID, workspace string) Job {
	now := time.Now()
	return Job{
		ID:            compactJobID(agentID),
		Name:          CompactJobName,
		AgentID:       agentID,
		ScheduleType:  ScheduleCron,
		Schedule:      compactCronSchedule,
		SessionTarget: SessionIsolated,
		Enabled:       true,
		Workspace:     workspace,
		CreatedAt:     now,
	}
}

func compactJobID(agentID string) string {
	return CompactJobID + ":" + agentID
}

// IsCompactJob reports whether the job ID belongs to a memory compact job.
func IsCompactJob(jobID string) bool {
	return strings.HasPrefix(jobID, CompactJobID)
}

// BuildCompactPrompt reads yesterday's daily memory file and extracts session
// activity from the previous day, then assembles a self-contained prompt that
// instructs the LLM to condense the daily file and extract persistent facts
// into MEMORY.md. Returns ("", false) if there is nothing to compact.
// The now parameter anchors all date calculations to a single snapshot.
func BuildCompactPrompt(workspace string, now time.Time) (string, bool) {
	yesterday := now.AddDate(0, 0, -1)
	dateStr := yesterday.Format("2006-01-02")

	// 1. Read yesterday's daily memory file.
	dailyPath := filepath.Join(workspace, consts.DailyMemoryFile(yesterday))
	dailyContent, dailyErr := os.ReadFile(dailyPath)
	dailyText := ""
	if dailyErr == nil {
		dailyText = strings.TrimSpace(string(dailyContent))
	}

	// 2. Extract user messages from yesterday's sessions.
	sessionsDir := filepath.Join(workspace, "memory", "sessions")
	excerpts, _ := ExtractUserMessages(sessionsDir, yesterday, maxUserMessagesPerSession)

	// If no daily file and no session activity, skip.
	if dailyText == "" && len(excerpts) == 0 {
		return "", false
	}

	// 3. Build the prompt.
	var b strings.Builder
	b.Grow(4096)

	fmt.Fprintf(&b, "# Memory Compaction — %s\n\n", dateStr)
	b.WriteString("You are performing a nightly memory maintenance task. Review the data below and execute two tasks.\n\n")

	// Include daily file content.
	if dailyText != "" {
		fmt.Fprintf(&b, "## Yesterday's Daily Memory (`memory/daily/%s.md`)\n\n", dateStr)
		b.WriteString("```\n")
		b.WriteString(dailyText)
		b.WriteString("\n```\n\n")
	}

	// Include session excerpts (with token budget).
	if len(excerpts) > 0 {
		b.WriteString("## Session Activity Summary\n\n")
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
	}

	// Instructions.
	b.WriteString("## Instructions\n\n")
	b.WriteString("Perform these two tasks using the workspace file tools:\n\n")
	fmt.Fprintf(&b, "**Task 1: Condense daily file** — Edit `memory/daily/%s.md`:\n", dateStr)
	b.WriteString("- Remove noise, duplicates, and transient details.\n")
	b.WriteString("- Keep a concise summary of key events, decisions, and outcomes.\n")
	b.WriteString("- Preserve timestamps where meaningful.\n\n")
	b.WriteString("**Task 2: Extract persistent facts** — Edit `memory/MEMORY.md`:\n")
	b.WriteString("- Identify any durable knowledge from the daily file or session activity:\n")
	b.WriteString("  user preferences, project decisions, contact info, recurring patterns.\n")
	b.WriteString("- Append new facts to the appropriate section in MEMORY.md.\n")
	b.WriteString("- Do not duplicate information already in MEMORY.md.\n")
	b.WriteString("- If nothing is worth persisting, skip this task.\n\n")
	b.WriteString("Be concise and factual. Do not add commentary beyond the file edits.\n")

	return b.String(), true
}
