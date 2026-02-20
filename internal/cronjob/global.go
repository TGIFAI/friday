package cronjob

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/pkg/logs"
)

var (
	globalMu        sync.RWMutex
	globalScheduler *Scheduler
)

// Init creates the global scheduler. Call Start afterwards to begin the
// scheduling loop. Callers should register heartbeat jobs between Init and
// Start.
func Init(cfg config.CronjobConfig, enqueue EnqueueFunc) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalScheduler = NewScheduler(cfg, enqueue)
}

// Default returns the global scheduler, or nil if Init has not been called.
func Default() *Scheduler {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalScheduler
}

// Start loads persisted jobs and begins the global scheduler's loop.
// Returns an error if Init has not been called.
func Start(ctx context.Context) error {
	s := Default()
	if s == nil {
		return fmt.Errorf("cronjob: scheduler not initialized, call Init first")
	}
	return s.Start(ctx)
}

// Stop gracefully stops the global scheduler. Safe to call if Init was never
// called.
func Stop(ctx context.Context) {
	s := Default()
	if s == nil {
		return
	}
	s.Stop(ctx)
	logs.CtxInfo(ctx, "[cronjob] global scheduler stopped")
}

// LoadJobsFromStore reads persisted jobs directly from the store file without
// requiring a running scheduler. This is intended for CLI commands that need
// to inspect jobs offline.
func LoadJobsFromStore() ([]Job, error) {
	storePath := filepath.Join(consts.FridayHomeDir(), defaultStorePath)
	store := NewStore(storePath)
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store.List(), nil
}

// FormatJobList renders a human-readable summary of the given jobs.
func FormatJobList(jobs []Job) string {
	if len(jobs) == 0 {
		return "No scheduled jobs"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Scheduled Jobs (%d):\n", len(jobs))

	for i, j := range jobs {
		fmt.Fprintf(&b, "\n%d. %s [%s]\n", i+1, j.Name, j.ID)
		fmt.Fprintf(&b, "   Schedule: %s %s\n", j.ScheduleType, j.Schedule)
		if j.Enabled {
			b.WriteString("   Enabled: ✓\n")
		} else {
			b.WriteString("   Enabled: ✗\n")
		}
		if j.LastRunAt != nil {
			fmt.Fprintf(&b, "   Last run: %s\n", j.LastRunAt.Format(time.RFC3339))
		}
		if j.NextRunAt != nil {
			fmt.Fprintf(&b, "   Next run: %s\n", j.NextRunAt.Format(time.RFC3339))
		}
	}

	return b.String()
}
