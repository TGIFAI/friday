package consts

import (
	"os"
	"path/filepath"

	"github.com/tgifai/friday/internal/pkg/utils"
)

// Runtime identifies the execution environment.
type Runtime string

const (
	RuntimeCLI      Runtime = "cli"
	RuntimeMacOSApp Runtime = "macos-app"

	runtimeEnvKey = "FRIDAY_RUNTIME"

	// macOSAppChannelID is the built-in HTTP channel ID injected in macOS app mode.
	MacOSAppChannelID = "macos-app"

	// macOSTokenFile stores the shared auth token within FRIDAY_HOME.
	macOSTokenFile = ".macos-token"
)

// GetRuntime returns the current execution runtime.
func GetRuntime() Runtime {
	if os.Getenv(runtimeEnvKey) == string(RuntimeMacOSApp) {
		return RuntimeMacOSApp
	}
	return RuntimeCLI
}

// IsMacOSApp returns true when running inside the macOS app wrapper.
func IsMacOSApp() bool {
	return GetRuntime() == RuntimeMacOSApp
}

// MacOSTokenPath returns the path to the shared token file.
func MacOSTokenPath() string {
	return filepath.Join(FridayHomeDir(), macOSTokenFile)
}

// ReadOrCreateMacOSToken reads the token from disk, or creates a new one
// if it doesn't exist. Both the SwiftUI app and the Go process use this
// file as the shared secret for the built-in HTTP channel.
func ReadOrCreateMacOSToken() (string, error) {
	path := MacOSTokenPath()

	data, err := os.ReadFile(path)
	if err == nil {
		if token := string(data); token != "" {
			return token, nil
		}
	}

	token := utils.RandStr(32)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return "", err
	}
	return token, nil
}
