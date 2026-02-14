package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/bytedance/sonic"
)

type GoJudgeExecutor struct {
	workspace string

	endpoint       string
	authToken      string
	workdirMount   string
	cpuLimitMS     int
	wallLimitMS    int
	memoryLimitKB  int
	procLimit      int
	maxStdoutBytes int
	maxStderrBytes int

	client *http.Client
}

func NewGoJudgeExecutor(workspace string, cfg GoJudgeConfig) *GoJudgeExecutor {
	timeout := time.Duration(cfg.RequestTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 70 * time.Second
	}
	workdirMount := strings.TrimSpace(cfg.WorkdirMount)
	if workdirMount == "" {
		workdirMount = "/w"
	}

	return &GoJudgeExecutor{
		workspace:      filepath.Clean(workspace),
		endpoint:       strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/"),
		authToken:      strings.TrimSpace(cfg.AuthToken),
		workdirMount:   filepath.Clean(workdirMount),
		cpuLimitMS:     cfg.CPULimitMS,
		wallLimitMS:    cfg.WallLimitMS,
		memoryLimitKB:  cfg.MemoryLimitKB,
		procLimit:      cfg.ProcLimit,
		maxStdoutBytes: cfg.MaxStdoutBytes,
		maxStderrBytes: cfg.MaxStderrBytes,
		client:         &http.Client{Timeout: timeout},
	}
}

func (e *GoJudgeExecutor) Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error) {
	if req == nil {
		return nil, fmt.Errorf("exec request is required")
	}
	if e.endpoint == "" {
		return nil, fmt.Errorf("go-judge endpoint is required")
	}

	args, err := e.buildArgs(req.Command)
	if err != nil {
		return nil, err
	}
	sandboxWD, err := e.resolveSandboxWorkingDir(req.WorkingDir)
	if err != nil {
		return nil, err
	}

	wallLimitMS := e.wallLimitMS
	if req.Timeout > 0 {
		reqTimeoutMS := int(req.Timeout / time.Millisecond)
		if reqTimeoutMS > 0 && reqTimeoutMS < wallLimitMS {
			wallLimitMS = reqTimeoutMS
		}
	}
	cpuLimitMS := e.cpuLimitMS
	if cpuLimitMS > wallLimitMS {
		cpuLimitMS = wallLimitMS
	}

	payload := goJudgeRunRequest{
		Cmd: []goJudgeCommand{
			{
				Args:        args,
				Env:         []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
				Cwd:         sandboxWD,
				CPULimit:    cpuLimitMS,
				ClockLimit:  wallLimitMS,
				MemoryLimit: e.memoryLimitKB * 1024,
				ProcLimit:   e.procLimit,
				CopyOut:     []string{"stdout", "stderr"},
				CopyOutMax: map[string]int{
					"stdout": e.maxStdoutBytes,
					"stderr": e.maxStderrBytes,
				},
			},
		},
	}

	rawPayload, err := sonic.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal go-judge request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/run", bytes.NewReader(rawPayload))
	if err != nil {
		return nil, fmt.Errorf("build go-judge request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.authToken)
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("go-judge request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read go-judge response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("go-judge returned status %d: %s", resp.StatusCode, truncate(string(body), 512))
	}

	resultMap, err := parseFirstResult(body)
	if err != nil {
		return nil, err
	}
	stdout, stderr := extractStdStreams(resultMap)
	exitCode := extractExitCode(resultMap)
	timedOut := extractTimedOut(resultMap)
	if timedOut && exitCode == 0 {
		exitCode = -1
	}

	return &ExecResult{
		Stdout:   []byte(truncate(stdout, e.maxStdoutBytes)),
		Stderr:   []byte(truncate(stderr, e.maxStderrBytes)),
		ExitCode: exitCode,
		TimedOut: timedOut,
	}, nil
}

func (e *GoJudgeExecutor) buildArgs(cmd Command) ([]string, error) {
	if cmd.UseShell {
		display := strings.TrimSpace(cmd.Display)
		if display == "" {
			return nil, fmt.Errorf("command is required")
		}
		return []string{"/bin/sh", "-lc", display}, nil
	}
	program := strings.TrimSpace(cmd.Program)
	if program == "" {
		return nil, fmt.Errorf("command program is required")
	}
	args := make([]string, 0, len(cmd.Args)+1)
	args = append(args, program)
	args = append(args, cmd.Args...)
	return args, nil
}

func (e *GoJudgeExecutor) resolveSandboxWorkingDir(hostWorkingDir string) (string, error) {
	if strings.TrimSpace(hostWorkingDir) == "" {
		return filepath.ToSlash(e.workdirMount), nil
	}

	wd := strings.TrimSpace(hostWorkingDir)
	if !filepath.IsAbs(wd) {
		base := e.workspace
		if base == "" {
			base = "."
		}
		wd = filepath.Join(base, wd)
	}
	wd = filepath.Clean(wd)

	if strings.TrimSpace(e.workspace) == "" {
		return filepath.ToSlash(e.workdirMount), nil
	}
	workspace := filepath.Clean(e.workspace)
	within, err := isPathWithin(wd, workspace)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox working dir: %w", err)
	}
	if !within {
		return "", fmt.Errorf("working_dir must be within workspace when sandbox is enabled")
	}

	rel, err := filepath.Rel(workspace, wd)
	if err != nil {
		return "", fmt.Errorf("resolve relative workspace path: %w", err)
	}
	if rel == "." {
		return filepath.ToSlash(e.workdirMount), nil
	}
	return filepath.ToSlash(filepath.Join(e.workdirMount, rel)), nil
}

func parseFirstResult(raw []byte) (map[string]interface{}, error) {
	var decoded interface{}
	if err := sonic.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("parse go-judge response: %w", err)
	}

	if result, ok := extractResultMap(decoded); ok {
		return result, nil
	}
	return nil, fmt.Errorf("go-judge response does not contain command result")
}

func extractResultMap(v interface{}) (map[string]interface{}, bool) {
	switch data := v.(type) {
	case map[string]interface{}:
		if len(data) == 0 {
			return nil, false
		}
		if one, ok := data["result"]; ok {
			return extractResultMap(one)
		}
		if many, ok := data["results"]; ok {
			return extractResultMap(many)
		}
		if one, ok := data["data"]; ok {
			if result, ok := extractResultMap(one); ok {
				return result, true
			}
		}
		return data, true
	case []interface{}:
		if len(data) == 0 {
			return nil, false
		}
		first, ok := data[0].(map[string]interface{})
		return first, ok
	default:
		return nil, false
	}
}

func extractStdStreams(result map[string]interface{}) (string, string) {
	stdout := gconv.To[string](result["stdout"])
	stderr := gconv.To[string](result["stderr"])

	if files, ok := result["files"].(map[string]interface{}); ok {
		if stdout == "" {
			stdout = gconv.To[string](files["stdout"])
		}
		if stderr == "" {
			stderr = gconv.To[string](files["stderr"])
		}
	}

	return stdout, stderr
}

func extractExitCode(result map[string]interface{}) int {
	if _, exists := result["exitStatus"]; exists {
		return gconv.To[int](result["exitStatus"])
	}
	if _, exists := result["exitCode"]; exists {
		return gconv.To[int](result["exitCode"])
	}
	if _, exists := result["code"]; exists {
		return gconv.To[int](result["code"])
	}
	return 0
}

func extractTimedOut(result map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(gconv.To[string](result["status"])))
	return strings.Contains(status, "time limit") || strings.Contains(status, "timeout")
}

func isPathWithin(path string, root string) (bool, error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}

	rel, err := filepath.Rel(filepath.Clean(rootAbs), filepath.Clean(pathAbs))
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return false, nil
	}
	return true, nil
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

type goJudgeRunRequest struct {
	Cmd []goJudgeCommand `json:"cmd"`
}

type goJudgeCommand struct {
	Args        []string       `json:"args"`
	Env         []string       `json:"env,omitempty"`
	Cwd         string         `json:"cwd,omitempty"`
	CPULimit    int            `json:"cpuLimit,omitempty"`
	ClockLimit  int            `json:"clockLimit,omitempty"`
	MemoryLimit int            `json:"memoryLimit,omitempty"`
	ProcLimit   int            `json:"procLimit,omitempty"`
	CopyOut     []string       `json:"copyOut,omitempty"`
	CopyOutMax  map[string]int `json:"copyOutMax,omitempty"`
}
