package cronjob

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
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
	dateStr := now.Format("2006-01-02")

	sessionsDir := filepath.Join(workspace, "memory", "sessions")
	excerpts, _ := ExtractUserMessages(sessionsDir, now, maxUserMessagesPerSession)
	if len(excerpts) == 0 {
		return "", false
	}

	dailyText := readDailyFile(workspace, dateStr)
	memoryText := readMemoryMD(workspace)

	var b strings.Builder
	b.Grow(4096)

	fmt.Fprintf(&b, "# Memory Flush — %s\n\n", dateStr)
	b.WriteString("You are performing a memory flush task. Review the session activity below and preserve important context.\n\n")

	writeExcerpts(&b, "Today's Session Activity", excerpts)
	writeDailyFile(&b, "Existing Daily File", dateStr, dailyText)
	writeMemoryFile(&b, memoryText)

	b.WriteString("## Instructions\n\n")
	b.WriteString("Perform these two tasks using the workspace file tools:\n\n")
	fmt.Fprintf(&b, "**Task 1: Update daily file** — Edit `memory/daily/%s.md`:\n", dateStr)
	b.WriteString("- Append a summary of today's key events, decisions, and outcomes from session activity.\n")
	b.WriteString("- Do not duplicate entries already present in the file.\n")
	b.WriteString("- Use concise bullet points with timestamps where meaningful.\n\n")
	writeExtractFactsInstructions(&b)

	return b.String(), true
}
