package sandbox

import (
	"context"
	"time"
)

type Command struct {
	Display  string
	Program  string
	Args     []string
	UseShell bool
}

type ExecRequest struct {
	Workspace  string
	WorkingDir string
	Timeout    time.Duration
	Command    Command
}

type ExecResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	TimedOut bool
}

type Executor interface {
	Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error)
}
