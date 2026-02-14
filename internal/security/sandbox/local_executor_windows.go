//go:build windows

package sandbox

import "os/exec"

func setCommandProcessGroup(cmd *exec.Cmd) {
	_ = cmd
}

func killCommandProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
