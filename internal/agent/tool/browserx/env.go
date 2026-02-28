package browserx

import (
	"fmt"
	"os"
	"runtime"
)

// canRunHeaded checks if the current environment supports headed (GUI) browser mode.
func canRunHeaded() (bool, string) {
	switch runtime.GOOS {
	case "darwin":
		if os.Getenv("SSH_CONNECTION") != "" && os.Getenv("DISPLAY") == "" {
			return false, "macOS SSH session without display forwarding"
		}
		return true, ""
	case "linux":
		display := os.Getenv("DISPLAY")
		waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
		if display == "" && waylandDisplay == "" {
			return false, "no DISPLAY or WAYLAND_DISPLAY set (headless Linux server?)"
		}
		return true, ""
	case "windows":
		return true, ""
	default:
		return false, fmt.Sprintf("unsupported OS: %s", runtime.GOOS)
	}
}
