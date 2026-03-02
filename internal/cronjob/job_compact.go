package cronjob

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
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

	dailyText := readDailyFile(workspace, dateStr)

	sessionsDir := filepath.Join(workspace, "memory", "sessions")
	excerpts, _ := ExtractUserMessages(sessionsDir, yesterday, maxUserMessagesPerSession)

	if dailyText == "" && len(excerpts) == 0 {
		return "", false
	}

	var b strings.Builder
	b.Grow(4096)

	fmt.Fprintf(&b, "# Memory Compaction — %s\n\n", dateStr)
	b.WriteString("You are performing a nightly memory maintenance task. Review the data below and execute two tasks.\n\n")

	writeDailyFile(&b, "Yesterday's Daily Memory", dateStr, dailyText)
	writeExcerpts(&b, "Session Activity Summary", excerpts)

	b.WriteString("## Instructions\n\n")
	b.WriteString("Perform these two tasks using the workspace file tools:\n\n")
	fmt.Fprintf(&b, "**Task 1: Condense daily file** — Edit `memory/daily/%s.md`:\n", dateStr)
	b.WriteString("- Remove noise, duplicates, and transient details.\n")
	b.WriteString("- Keep a concise summary of key events, decisions, and outcomes.\n")
	b.WriteString("- Preserve timestamps where meaningful.\n\n")
	writeExtractFactsInstructions(&b)

	return b.String(), true
}
