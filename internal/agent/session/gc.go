package session

import (
	"context"
	"time"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const defaultGCInterval = 10 * time.Minute

func (m *Manager) StartGCLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultGCInterval
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed, err := m.GC()
				if err != nil {
					logs.CtxWarn(ctx, "[session] GC failed: %v", err)
					continue
				}
				if removed > 0 {
					logs.CtxInfo(ctx, "[session] GC removed %d expired session file(s)", removed)
				}
			}
		}
	}()
}
