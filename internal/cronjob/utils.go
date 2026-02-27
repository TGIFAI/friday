package cronjob

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// cronParser is a standard 5-field cron expression parser (minute hour dom month dow).
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// calcNextRun computes the next execution time for a job relative to from.
func calcNextRun(job *Job, from time.Time) (time.Time, error) {
	switch job.ScheduleType {
	case ScheduleEvery:
		d, err := time.ParseDuration(job.Schedule)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse every duration %q: %w", job.Schedule, err)
		}
		if d <= 0 {
			return time.Time{}, fmt.Errorf("every duration must be positive, got %v", d)
		}
		return from.Add(d), nil

	case ScheduleCron:
		sched, err := cronParser.Parse(job.Schedule)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse cron expression %q: %w", job.Schedule, err)
		}
		return sched.Next(from), nil

	case ScheduleAt:
		t, err := time.Parse(time.RFC3339, job.Schedule)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse at timestamp %q: %w", job.Schedule, err)
		}
		if t.After(from) {
			return t, nil
		}
		// Already past — signal that this one-shot is done.
		return time.Time{}, nil

	default:
		return time.Time{}, fmt.Errorf("unknown schedule type: %s", job.ScheduleType)
	}
}

// backoffSteps defines exponential retry delays on consecutive failures.
var backoffSteps = []time.Duration{
	30 * time.Second,
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	60 * time.Minute, // cap
}

// backoffDelay returns the retry delay for the given consecutive error count.
func backoffDelay(consecutiveErr int) time.Duration {
	idx := consecutiveErr - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(backoffSteps) {
		idx = len(backoffSteps) - 1
	}
	return backoffSteps[idx]
}
