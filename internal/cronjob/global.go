package cronjob

import (
	"context"
	"fmt"
	"sync"

	"github.com/tgifai/friday/internal/config"
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
