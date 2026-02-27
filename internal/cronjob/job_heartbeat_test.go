package cronjob

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildHeartbeatPrompt_MissingFile(t *testing.T) {
	_, hasWork := BuildHeartbeatPrompt(t.TempDir())
	if hasWork {
		t.Fatal("expected no work for missing file")
	}
}

func TestBuildHeartbeatPrompt_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, heartbeatFile), []byte(""), 0o644)

	_, hasWork := BuildHeartbeatPrompt(dir)
	if hasWork {
		t.Fatal("expected no work for empty file")
	}
}

func TestBuildHeartbeatPrompt_OnlyHeadersAndComments(t *testing.T) {
	content := `# HEARTBEAT.md - Friday Periodic Tasks

This file is checked periodically by the Friday agent.

<!-- Add periodic tasks below this line -->

## Active Tasks

<!-- nothing here -->

## Completed

<!-- Move completed tasks here or remove them -->
`
	// The template has descriptive text lines like "This file is checked..."
	// which count as real content. Test with pure headers + comments only.
	headersOnly := `# HEARTBEAT.md
## Active Tasks
<!-- no tasks -->
## Completed
<!-- nothing -->
`
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, heartbeatFile), []byte(headersOnly), 0o644)

	_, hasWork := BuildHeartbeatPrompt(dir)
	if hasWork {
		t.Fatal("expected no work for headers-only file")
	}

	// With the full template (has descriptive text), it counts as work.
	os.WriteFile(filepath.Join(dir, heartbeatFile), []byte(content), 0o644)
	_, hasWork = BuildHeartbeatPrompt(dir)
	if !hasWork {
		t.Fatal("expected work for file with descriptive text")
	}
}

func TestBuildHeartbeatPrompt_WithTasks(t *testing.T) {
	content := `# HEARTBEAT.md

## Active Tasks

- Check email inbox every 30 minutes
- Review calendar for upcoming meetings
`
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, heartbeatFile), []byte(content), 0o644)

	prompt, hasWork := BuildHeartbeatPrompt(dir)
	if !hasWork {
		t.Fatal("expected work for file with tasks")
	}
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
}

func TestNewHeartbeatJob(t *testing.T) {
	job := NewHeartbeatJob("agent-1", "/workspace", 0)

	if !IsHeartbeatJob(job.ID) {
		t.Errorf("job ID %q should be detected as heartbeat", job.ID)
	}
	if job.AgentID != "agent-1" {
		t.Errorf("agent ID = %q, want agent-1", job.AgentID)
	}
	if job.SessionTarget != SessionMain {
		t.Errorf("session target = %q, want main", job.SessionTarget)
	}
	if job.NextRunAt == nil {
		t.Fatal("NextRunAt should be set")
	}
}

func TestIsHeartbeatJob(t *testing.T) {
	if IsHeartbeatJob("regular-job") {
		t.Error("regular job should not be heartbeat")
	}
	if !IsHeartbeatJob(HeartbeatJobID + ":agent-1") {
		t.Error("heartbeat job should be detected")
	}
}
