package browserx

import (
	"os"
	"runtime"
	"testing"
)

func TestCanRunHeaded(t *testing.T) {
	ok, reason := canRunHeaded()

	switch runtime.GOOS {
	case "darwin":
		if os.Getenv("SSH_CONNECTION") == "" {
			if !ok {
				t.Errorf("expected headed to be supported on macOS local, got reason: %s", reason)
			}
		}
	case "linux":
		display := os.Getenv("DISPLAY")
		wayland := os.Getenv("WAYLAND_DISPLAY")
		if display == "" && wayland == "" {
			if ok {
				t.Error("expected headed NOT supported on linux without DISPLAY/WAYLAND_DISPLAY")
			}
		} else {
			if !ok {
				t.Errorf("expected headed supported on linux with display, got reason: %s", reason)
			}
		}
	case "windows":
		if !ok {
			t.Errorf("expected headed supported on windows, got reason: %s", reason)
		}
	}
}
