package shellx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	defaultProcessLogTail = 4096
	maxProcessBufferBytes = 1 << 20 // 1 MiB per stream
)

type ProcessTool struct {
	workspace string

	mu    sync.RWMutex
	procs map[string]*managedProcess
	seq   atomic.Int64
}

type managedProcess struct {
	id         string
	command    string
	workingDir string
	startedAt  time.Time
	endedAt    time.Time
	cmd        *exec.Cmd

	stdout *tailBuffer
	stderr *tailBuffer

	mu          sync.RWMutex
	running     bool
	exitCode    int
	hasExitCode bool
	waitErr     string
}

func NewProcessTool(workspace string) *ProcessTool {
	return &ProcessTool{
		workspace: workspace,
		procs:     make(map[string]*managedProcess),
	}
}

func (t *ProcessTool) Name() string {
	return "process"
}

func (t *ProcessTool) Description() string {
	return "Manage long-running background processes"
}

func (t *ProcessTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		Extra: map[string]interface{}{
			"action":      "string (required) - start|status|log|kill|list",
			"process_id":  "string (required for status|log|kill)",
			"command":     "string|[]string (required for start)",
			"working_dir": "string (optional, start only)",
			"tail":        "number (optional, log only) - max bytes per stream",
		},
	}
}

func (t *ProcessTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	action := strings.ToLower(strings.TrimSpace(gconv.To[string](args["action"])))
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	switch action {
	case "start":
		return t.startProcess(ctx, args)
	case "status":
		return t.statusProcess(args)
	case "log":
		return t.logProcess(args)
	case "kill":
		return t.killProcess(args)
	case "list":
		return t.listProcesses(), nil
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}

func (t *ProcessTool) startProcess(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	parsedCmd, err := parseCommandArg(args["command"])
	if err != nil {
		return nil, err
	}

	workingDir := t.resolveWorkingDir(args)
	cmd := commandNoContext(parsedCmd)
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	setCommandProcessGroup(cmd)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("prepare stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("prepare stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process failed: %w", err)
	}

	id := t.nextProcessID()
	proc := &managedProcess{
		id:         id,
		command:    parsedCmd.display,
		workingDir: workingDir,
		startedAt:  time.Now(),
		cmd:        cmd,
		stdout:     newTailBuffer(maxProcessBufferBytes),
		stderr:     newTailBuffer(maxProcessBufferBytes),
		running:    true,
	}

	t.mu.Lock()
	t.procs[id] = proc
	t.mu.Unlock()

	go io.Copy(proc.stdout, stdoutPipe)
	go io.Copy(proc.stderr, stderrPipe)
	go t.waitProcess(proc)

	logs.CtxInfo(ctx, "[tool:process] started process_id=%s command=%q", id, parsedCmd.display)

	return map[string]interface{}{
		"success":     true,
		"process_id":  id,
		"running":     true,
		"command":     parsedCmd.display,
		"working_dir": workingDir,
		"started_at":  proc.startedAt.Format(time.RFC3339),
	}, nil
}

func (t *ProcessTool) statusProcess(args map[string]interface{}) (interface{}, error) {
	proc, err := t.getProcessByArgs(args)
	if err != nil {
		return nil, err
	}
	return proc.snapshotStatus(), nil
}

func (t *ProcessTool) logProcess(args map[string]interface{}) (interface{}, error) {
	proc, err := t.getProcessByArgs(args)
	if err != nil {
		return nil, err
	}

	tail := gconv.To[int](args["tail"])
	if tail <= 0 {
		tail = defaultProcessLogTail
	}
	if tail > maxProcessBufferBytes {
		tail = maxProcessBufferBytes
	}

	status := proc.snapshotStatus()
	status["stdout"] = proc.stdout.Tail(tail)
	status["stderr"] = proc.stderr.Tail(tail)
	status["tail"] = tail
	return status, nil
}

func (t *ProcessTool) killProcess(args map[string]interface{}) (interface{}, error) {
	proc, err := t.getProcessByArgs(args)
	if err != nil {
		return nil, err
	}

	proc.mu.RLock()
	running := proc.running
	proc.mu.RUnlock()
	if running {
		killCommandProcessGroup(proc.cmd)
	}

	status := proc.snapshotStatus()
	status["killed"] = running
	return status, nil
}

func (t *ProcessTool) listProcesses() []map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]map[string]interface{}, 0, len(t.procs))
	for _, proc := range t.procs {
		out = append(out, proc.snapshotStatus())
	}
	return out
}

func (t *ProcessTool) waitProcess(proc *managedProcess) {
	waitErr := proc.cmd.Wait()

	proc.mu.Lock()
	defer proc.mu.Unlock()

	proc.running = false
	proc.endedAt = time.Now()
	if waitErr != nil {
		proc.waitErr = waitErr.Error()
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			proc.exitCode = exitErr.ExitCode()
			proc.hasExitCode = true
			return
		}
		proc.exitCode = -1
		proc.hasExitCode = true
		return
	}

	proc.exitCode = 0
	proc.hasExitCode = true
}

func (t *ProcessTool) resolveWorkingDir(args map[string]interface{}) string {
	workingDir := t.workspace
	if wd, ok := args["working_dir"].(string); ok && wd != "" {
		workingDir = wd
		if !filepath.IsAbs(workingDir) && t.workspace != "" {
			workingDir = filepath.Join(t.workspace, workingDir)
		}
	}
	return workingDir
}

func (t *ProcessTool) nextProcessID() string {
	id := t.seq.Add(1)
	return fmt.Sprintf("proc-%d", id)
}

func (t *ProcessTool) getProcessByArgs(args map[string]interface{}) (*managedProcess, error) {
	id := strings.TrimSpace(gconv.To[string](args["process_id"]))
	if id == "" {
		id = strings.TrimSpace(gconv.To[string](args["pid"]))
	}
	if id == "" {
		return nil, fmt.Errorf("process_id is required")
	}

	t.mu.RLock()
	proc, ok := t.procs[id]
	t.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("process not found: %s", id)
	}
	return proc, nil
}

func (p *managedProcess) snapshotStatus() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := map[string]interface{}{
		"process_id":  p.id,
		"command":     p.command,
		"working_dir": p.workingDir,
		"running":     p.running,
		"started_at":  p.startedAt.Format(time.RFC3339),
	}
	if !p.endedAt.IsZero() {
		result["ended_at"] = p.endedAt.Format(time.RFC3339)
	}
	if p.hasExitCode {
		result["exit_code"] = p.exitCode
	}
	if p.waitErr != "" {
		result["error"] = p.waitErr
	}
	return result
}

type tailBuffer struct {
	mu   sync.RWMutex
	max  int
	data []byte
}

func newTailBuffer(maxBytes int) *tailBuffer {
	if maxBytes <= 0 {
		maxBytes = maxProcessBufferBytes
	}
	return &tailBuffer{max: maxBytes}
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = append(b.data, p...)
	if over := len(b.data) - b.max; over > 0 {
		b.data = append([]byte(nil), b.data[over:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) Tail(n int) string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n <= 0 || n > len(b.data) {
		n = len(b.data)
	}
	if n == 0 {
		return ""
	}
	out := make([]byte, n)
	copy(out, b.data[len(b.data)-n:])
	return string(out)
}
