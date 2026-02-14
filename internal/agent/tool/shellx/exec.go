package shellx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/security/sandbox"
)

type ExecTool struct {
	workspace string
	timeout   time.Duration
	executor  sandbox.Executor
}

func NewExecTool(workspace string, executor ...sandbox.Executor) *ExecTool {
	var one sandbox.Executor
	if len(executor) > 0 {
		one = executor[0]
	}
	return &ExecTool{
		workspace: workspace,
		timeout:   60 * time.Second,
		executor:  one,
	}
}

func (t *ExecTool) Name() string {
	return "exec"
}

func (t *ExecTool) Description() string {
	return "Execute short-lived commands"
}

func (t *ExecTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		Extra: map[string]interface{}{
			"command":     "string|[]string (required) - command string or argv array",
			"working_dir": "string (optional) - working directory",
			"timeout":     "number (optional) - timeout in seconds",
		},
	}
}

func (t *ExecTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	parsedCmd, err := parseCommandArg(args["command"])
	if err != nil {
		return nil, err
	}

	workingDir := t.resolveWorkingDir(args)
	timeout := t.resolveTimeout(args)

	var (
		stdout     []byte
		stderr     []byte
		exitCode   int
		timeoutErr bool
	)
	if t.executor != nil {
		res, execErr := t.executor.Execute(ctx, &sandbox.ExecRequest{
			Workspace:  t.workspace,
			WorkingDir: workingDir,
			Timeout:    timeout,
			Command: sandbox.Command{
				Display:  parsedCmd.display,
				Program:  parsedCmd.program,
				Args:     parsedCmd.argv,
				UseShell: parsedCmd.useShell,
			},
		})
		if execErr != nil {
			return nil, execErr
		}
		if res == nil {
			return nil, fmt.Errorf("sandbox executor returned nil result")
		}
		stdout = res.Stdout
		stderr = res.Stderr
		exitCode = res.ExitCode
		timeoutErr = res.TimedOut
	} else {
		var runErr error
		stdout, stderr, exitCode, timeoutErr, runErr = runExecCommand(ctx, parsedCmd, workingDir, timeout)
		if runErr != nil {
			return nil, runErr
		}
	}

	if timeoutErr {
		return nil, fmt.Errorf("command timeout after %v", timeout)
	}

	logs.CtxInfo(ctx, "[tool:%s] exec: %s (exit_code: %d)", t.Name(), parsedCmd.display, exitCode)

	return map[string]interface{}{
		"success":     exitCode == 0,
		"command":     parsedCmd.display,
		"exit_code":   exitCode,
		"stdout":      string(stdout),
		"stderr":      string(stderr),
		"working_dir": workingDir,
	}, nil
}

func (t *ExecTool) resolveWorkingDir(args map[string]interface{}) string {
	workingDir := t.workspace
	if wd, ok := args["working_dir"].(string); ok && wd != "" {
		workingDir = wd
		if !filepath.IsAbs(workingDir) && t.workspace != "" {
			workingDir = filepath.Join(t.workspace, workingDir)
		}
	}
	return workingDir
}

func (t *ExecTool) resolveTimeout(args map[string]interface{}) time.Duration {
	timeout := t.timeout
	if timeoutSec := gconv.To[float64](args["timeout"]); timeoutSec > 0 {
		timeout = time.Duration(timeoutSec * float64(time.Second))
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return timeout
}

func runExecCommand(
	ctx context.Context,
	parsedCmd *parsedCommand,
	workingDir string,
	timeout time.Duration,
) ([]byte, []byte, int, bool, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := commandWithContext(cmdCtx, parsedCmd)
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	setCommandProcessGroup(cmd)

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		killCommandProcessGroup(cmd)
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), 0, true, nil
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitErr.ExitCode(), false, nil
		}
		return nil, nil, 0, false, fmt.Errorf("command execution failed: %w", err)
	}
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), 0, false, nil
}

func commandWithContext(ctx context.Context, parsedCmd *parsedCommand) *exec.Cmd {
	if parsedCmd.useShell {
		return exec.CommandContext(ctx, "sh", "-c", parsedCmd.display)
	}
	return exec.CommandContext(ctx, parsedCmd.program, parsedCmd.argv...)
}

func commandNoContext(parsedCmd *parsedCommand) *exec.Cmd {
	if parsedCmd.useShell {
		return exec.Command("sh", "-c", parsedCmd.display)
	}
	return exec.Command(parsedCmd.program, parsedCmd.argv...)
}

type parsedCommand struct {
	display  string
	program  string
	argv     []string
	useShell bool
}

func parseCommandArg(raw interface{}) (*parsedCommand, error) {
	switch v := raw.(type) {
	case string:
		cmd := strings.TrimSpace(v)
		if cmd == "" {
			return nil, fmt.Errorf("command is required")
		}
		return &parsedCommand{display: cmd, useShell: true}, nil
	case []string:
		return parseCommandArray(v)
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(gconv.To[string](item))
			if s != "" {
				parts = append(parts, s)
			}
		}
		return parseCommandArray(parts)
	default:
		return nil, fmt.Errorf("command is required and must be string or []string")
	}
}

func parseCommandArray(parts []string) (*parsedCommand, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("command is required")
	}
	program := strings.TrimSpace(parts[0])
	if program == "" {
		return nil, fmt.Errorf("command program is required")
	}
	argv := make([]string, 0, len(parts)-1)
	for _, arg := range parts[1:] {
		argv = append(argv, strings.TrimSpace(arg))
	}
	return &parsedCommand{
		display:  strings.Join(append([]string{program}, argv...), " "),
		program:  program,
		argv:     argv,
		useShell: false,
	}, nil
}
