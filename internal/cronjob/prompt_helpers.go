package cronjob

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/tgifai/friday/internal/consts"
)

// writeExcerpts renders session excerpts into b, respecting the global
// character budget defined by maxTotalExcerptChars / maxSingleMessageChars.
func writeExcerpts(b *strings.Builder, heading string, excerpts []SessionExcerpt) {
	if len(excerpts) == 0 {
		return
	}
	b.WriteString("## ")
	b.WriteString(heading)
	b.WriteString("\n\n")
	totalChars := 0
	for _, ex := range excerpts {
		if totalChars >= maxTotalExcerptChars {
			break
		}
		fmt.Fprintf(b, "### Session: %s\n\n", ex.SessionKey)
		for _, msg := range ex.Messages {
			if totalChars >= maxTotalExcerptChars {
				break
			}
			truncated := msg
			if utf8.RuneCountInString(truncated) > maxSingleMessageChars {
				runes := []rune(truncated)
				truncated = string(runes[:maxSingleMessageChars]) + "..."
			}
			fmt.Fprintf(b, "- %s\n", truncated)
			totalChars += len(truncated)
		}
		b.WriteString("\n")
	}
}

// writeDailyFile renders the daily memory file content into b as a fenced code block.
func writeDailyFile(b *strings.Builder, label, dateStr, text string) {
	if text == "" {
		return
	}
	fmt.Fprintf(b, "## %s (`memory/daily/%s.md`)\n\n", label, dateStr)
	b.WriteString("```\n")
	b.WriteString(text)
	b.WriteString("\n```\n\n")
}

// writeMemoryFile renders the MEMORY.md content into b, truncating if necessary.
func writeMemoryFile(b *strings.Builder, text string) {
	if text == "" {
		return
	}
	const maxMemoryChars = 4000
	b.WriteString("## Existing Memory File (`memory/MEMORY.md`)\n\n")
	b.WriteString("```\n")
	if len(text) > maxMemoryChars {
		b.WriteString(text[:maxMemoryChars])
		b.WriteString("\n... (truncated)\n")
	} else {
		b.WriteString(text)
	}
	b.WriteString("\n```\n\n")
}

// writeExtractFactsInstructions writes the shared "extract persistent facts"
// instruction block used by both flush and compact prompts.
func writeExtractFactsInstructions(b *strings.Builder) {
	b.WriteString("**Task 2: Extract persistent facts** — Edit `memory/MEMORY.md`:\n")
	b.WriteString("- Identify any durable knowledge from the session activity:\n")
	b.WriteString("  user preferences, project decisions, contact info, recurring patterns.\n")
	b.WriteString("- Append new facts to the appropriate section in MEMORY.md.\n")
	b.WriteString("- Do not duplicate information already in MEMORY.md.\n")
	b.WriteString("- If nothing is worth persisting, skip this task.\n\n")
	b.WriteString("Be concise and factual. Do not add commentary beyond the file edits.\n")
}

// readTrimmedFile reads a file and returns its trimmed content, or "" on error.
func readTrimmedFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// readDailyFile reads the daily memory file for the given workspace and date.
func readDailyFile(workspace string, dateStr string) string {
	// consts.DailyMemoryFile expects a time.Time, but we already have a path pattern.
	// Re-derive from the dateStr is fragile, so accept the path directly.
	return readTrimmedFile(filepath.Join(workspace, "memory", "daily", dateStr+".md"))
}

// readMemoryMD reads the workspace MEMORY.md file.
func readMemoryMD(workspace string) string {
	return readTrimmedFile(filepath.Join(workspace, consts.WorkspaceMemoryFile))
}
