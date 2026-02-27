package cronjob

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// HeartbeatJobID is the reserved job ID for the built-in heartbeat.
	HeartbeatJobID = "__heartbeat__"
	// HeartbeatJobName is the human-readable name for the heartbeat job.
	HeartbeatJobName = "heartbeat"
	// defaultHeartbeatInterval is the default heartbeat period.
	defaultHeartbeatInterval = 30 * time.Minute
	// heartbeatMaxJitter is the upper bound of random delay added to the
	// first heartbeat fire time to prevent thundering-herd across agents.
	heartbeatMaxJitter = 60 * time.Second
	// heartbeatFile is the workspace-relative path to the heartbeat prompt.
	heartbeatFile = "HEARTBEAT.md"
)

// HeartbeatOK is the sentinel response an agent returns when no action is needed.
const HeartbeatOK = "HEARTBEAT_OK"

// NewHeartbeatJob creates the built-in heartbeat job for the given agent.
// Pass interval <= 0 to use the default (30 minutes).
// The returned job has SessionTarget "main" and is not persisted to jobs.json.
func NewHeartbeatJob(agentID, workspace string, interval time.Duration) Job {
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}

	now := time.Now()
	jitter := time.Duration(rand.Int64N(int64(heartbeatMaxJitter)))
	next := now.Add(interval).Add(jitter)
	return Job{
		ID:            heartbeatJobID(agentID),
		Name:          HeartbeatJobName,
		AgentID:       agentID,
		ScheduleType:  ScheduleEvery,
		Schedule:      interval.String(),
		SessionTarget: SessionMain,
		Enabled:       true,
		Workspace:     workspace,
		NextRunAt:     &next,
		CreatedAt:     now,
	}
}

func heartbeatJobID(agentID string) string {
	return HeartbeatJobID + ":" + agentID
}

// IsHeartbeatJob reports whether the job ID belongs to a built-in heartbeat.
func IsHeartbeatJob(jobID string) bool {
	return strings.HasPrefix(jobID, HeartbeatJobID)
}

// BuildHeartbeatPrompt reads HEARTBEAT.md from the workspace and decides
// whether there is actionable work. If the file is missing, empty, or contains
// only markdown headers and HTML comments, it returns ("", false) so the
// scheduler can skip the run and save tokens.
func BuildHeartbeatPrompt(workspace string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(workspace, heartbeatFile))
	if err != nil {
		return "", false
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", false
	}

	// Check whether every non-blank line is a heading or HTML comment.
	// If so, there is no real work — skip.
	hasWork := false
	inComment := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Track multi-line HTML comments.
		if inComment {
			if strings.Contains(trimmed, "-->") {
				inComment = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			if !strings.Contains(trimmed, "-->") {
				inComment = true
			}
			continue
		}
		// Headings are structural, not work items.
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Any other non-blank line indicates real work.
		hasWork = true
		break
	}

	if !hasWork {
		return "", false
	}
	return content, true
}
