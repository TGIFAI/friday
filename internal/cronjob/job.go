package cronjob

import "time"

// ScheduleType defines how a job's execution time is determined.
type ScheduleType string

const (
	// ScheduleEvery runs at a fixed interval (Go duration string, e.g. "5m", "1h30m").
	ScheduleEvery ScheduleType = "every"
	// ScheduleCron uses a standard 5-field cron expression.
	ScheduleCron ScheduleType = "cron"
	// ScheduleAt fires once at a specific ISO 8601 timestamp.
	ScheduleAt ScheduleType = "at"
)

// SessionTarget controls which conversation context a job runs in.
type SessionTarget string

const (
	// SessionMain injects the job prompt into the agent's primary session,
	// giving it full conversation history.
	SessionMain SessionTarget = "main"
	// SessionIsolated runs the job in a dedicated session keyed "cron:<jobId>",
	// preventing main-chat noise.
	SessionIsolated SessionTarget = "isolated"
)

// Job describes a single scheduled unit of work.
type Job struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	AgentID       string        `json:"agent_id"`
	ScheduleType  ScheduleType  `json:"schedule_type"`
	Schedule      string        `json:"schedule"`       // "5m" | "0 9 * * *" | "2026-03-01T09:00:00Z"
	Prompt        string        `json:"prompt"`         // message content sent to the agent
	SessionTarget SessionTarget `json:"session_target"` // "main" or "isolated"
	ChannelID     string        `json:"channel_id"`     // delivery channel (isolated jobs only)
	ChatID        string        `json:"chat_id"`        // delivery chat   (isolated jobs only)
	Enabled       bool          `json:"enabled"`

	// Workspace is set at runtime for heartbeat jobs; not persisted.
	Workspace string `json:"-"`

	// --- runtime state ---
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty"`
	ConsecutiveErr int        `json:"consecutive_err,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}
