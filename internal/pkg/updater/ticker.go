package updater

import (
	"context"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const defaultCheckInterval = 5 * time.Minute

// StartAutoUpdate runs a background loop that checks for updates at the given interval.
// When an update is found, it downloads and applies it, then restarts the process.
// Pass interval <= 0 to use the default (5 minutes).
func StartAutoUpdate(ctx context.Context, u *Updater, interval time.Duration) {
	if interval <= 0 {
		interval = defaultCheckInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			performAutoUpdate(ctx, u)
		}
	}
}

func performAutoUpdate(ctx context.Context, u *Updater) {
	needs, release, err := u.NeedsUpdate(ctx)
	if err != nil {
		logs.CtxWarn(ctx, "auto-update check failed: %v", err)
		return
	}
	if !needs {
		logs.CtxDebug(ctx, "auto-update: already up to date")
		return
	}

	logs.CtxInfo(ctx, "auto-update: new version %s available, downloading...", release.TagName)

	tmpDir, err := os.MkdirTemp("", "friday-update-*")
	if err != nil {
		logs.CtxError(ctx, "auto-update: create temp dir: %v", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	binaryPath, err := u.Download(ctx, release, tmpDir)
	if err != nil {
		logs.CtxError(ctx, "auto-update: download failed: %v", err)
		return
	}

	if err := u.Apply(binaryPath); err != nil {
		logs.CtxError(ctx, "auto-update: apply failed: %v", err)
		return
	}

	logs.CtxInfo(ctx, "auto-update: successfully updated to %s, restarting...", release.TagName)

	restartProcess()
}

func restartProcess() {
	exe, err := os.Executable()
	if err != nil {
		logs.Error("auto-update: cannot find executable for restart: %v", err)
		// Fall back to SIGTERM so the process manager can restart us
		p, _ := os.FindProcess(os.Getpid())
		p.Signal(syscall.SIGTERM)
		return
	}

	if runtime.GOOS == "windows" {
		// Windows doesn't support exec; send SIGTERM and let the process manager handle restart
		p, _ := os.FindProcess(os.Getpid())
		p.Signal(syscall.SIGTERM)
		return
	}

	// Unix: replace the current process with the new binary
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		logs.Error("auto-update: exec failed: %v, falling back to SIGTERM", err)
		p, _ := os.FindProcess(os.Getpid())
		p.Signal(syscall.SIGTERM)
	}
}
