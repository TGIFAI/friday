package browserx

import (
	"fmt"
	"os"
	"runtime"
)

// needsNoSandbox returns true when Chrome's OS-level sandbox cannot work.
// This is the case on Linux when running as root (common in CI containers)
// or when a CI environment variable is detected.
func needsNoSandbox() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Geteuid() == 0 {
		return true
	}
	// Common CI environment variables.
	for _, key := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_URL"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

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
