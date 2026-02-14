//go:build windows

package shellx

import "os/exec"

func setCommandProcessGroup(cmd *exec.Cmd) {
}

func killCommandProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
