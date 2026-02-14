package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

type LocalExecutor struct {
	workspace string
}

func NewLocalExecutor(workspace string) *LocalExecutor {
	return &LocalExecutor{
		workspace: workspace,
	}
}

func (e *LocalExecutor) Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error) {
	if req == nil {
		return nil, fmt.Errorf("exec request is required")
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := commandWithContext(cmdCtx, req.Command)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	} else if e.workspace != "" {
		cmd.Dir = e.workspace
	}
	setCommandProcessGroup(cmd)

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		killCommandProcessGroup(cmd)
		return &ExecResult{
			Stdout:   stdoutBuf.Bytes(),
			Stderr:   stderrBuf.Bytes(),
			ExitCode: 0,
			TimedOut: true,
		}, nil
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ExecResult{
				Stdout:   stdoutBuf.Bytes(),
				Stderr:   stderrBuf.Bytes(),
				ExitCode: exitErr.ExitCode(),
			}, nil
		}
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	return &ExecResult{
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderrBuf.Bytes(),
		ExitCode: 0,
	}, nil
}

func commandWithContext(ctx context.Context, cmd Command) *exec.Cmd {
	if cmd.UseShell {
		return exec.CommandContext(ctx, "sh", "-c", cmd.Display)
	}
	return exec.CommandContext(ctx, cmd.Program, cmd.Args...)
}
