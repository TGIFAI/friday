package agentx

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
)

// isSubpath reports whether child is under parent (or equal to parent).
func isSubpath(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if child == parent {
		return true
	}
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}

const (
	maxSessions    = 8
	defaultTimeout = 600 * time.Second
)

// AgentTool delegates tasks to external CLI agents (Claude Code, Codex).
type AgentTool struct {
	workspace string
	backends  map[string]Backend
	sessions  *SessionManager
}

func NewAgentTool(workspace string) *AgentTool {
	backends := map[string]Backend{
		"claude-code": &ClaudeCodeBackend{},
		"codex":       &CodexBackend{},
	}
	return &AgentTool{
		workspace: workspace,
		backends:  backends,
		sessions:  NewSessionManager(maxSessions),
	}
}

func (t *AgentTool) Name() string { return "agent" }
func (t *AgentTool) Description() string {
	return "Delegate coding tasks to CLI agents (Claude Code or Codex). Supports creating sessions, sending follow-up messages, checking status, and managing session lifecycle."
}

func (t *AgentTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     `Action: "create" (new session), "send" (follow-up message), "status" (check async), "list" (all sessions), "destroy" (terminate)`,
				Required: true,
				Enum:     []string{"create", "send", "status", "list", "destroy"},
			},
			"backend": {
				Type: schema.String,
				Desc: `CLI backend: "claude-code" or "codex". Required for "create".`,
				Enum: []string{"claude-code", "codex"},
			},
			"prompt": {
				Type: schema.String,
				Desc: `The task/prompt to send to the CLI agent. Required for "create" and "send".`,
			},
			"session_id": {
				Type: schema.String,
				Desc: `Session ID (e.g. "as-1"). Required for "send", "status", "destroy".`,
			},
			"working_dir": {
				Type: schema.String,
				Desc: `Working directory for the CLI agent. Optional, defaults to agent workspace.`,
			},
			"system_prompt": {
				Type: schema.String,
				Desc: `Additional system prompt to append. Optional, only for "create".`,
			},
			"max_turns": {
				Type: schema.Integer,
				Desc: `Maximum agentic turns. Optional, only for "create".`,
			},
			"async": {
				Type: schema.Boolean,
				Desc: `If true, run in background and return immediately. Use "status" to poll. Default false.`,
			},
		}),
	}
}

func (t *AgentTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	action := strings.ToLower(strings.TrimSpace(gconv.To[string](args["action"])))
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	switch action {
	case "create":
		return t.executeCreate(ctx, args)
	case "send":
		return t.executeSend(ctx, args)
	case "status":
		return t.executeStatus(args)
	case "list":
		return t.executeList()
	case "destroy":
		return t.executeDestroy(args)
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}

func (t *AgentTool) executeCreate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	backendName := strings.TrimSpace(gconv.To[string](args["backend"]))
	if backendName == "" {
		return nil, fmt.Errorf("backend is required for create action")
	}
	prompt := gconv.To[string](args["prompt"])
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required for create action")
	}

	backend, ok := t.backends[backendName]
	if !ok {
		return nil, fmt.Errorf("unknown backend: %s (available: claude-code, codex)", backendName)
	}
	if !backend.Available() {
		return nil, fmt.Errorf("%s CLI not found in PATH", backendName)
	}

	workingDir := strings.TrimSpace(gconv.To[string](args["working_dir"]))
	if workingDir == "" {
		workingDir = t.workspace
	} else if t.workspace != "" && !isSubpath(t.workspace, workingDir) {
		return nil, fmt.Errorf("working_dir %q is outside agent workspace %q", workingDir, t.workspace)
	}

	req := &RunRequest{
		Prompt:       prompt,
		WorkingDir:   workingDir,
		SystemPrompt: gconv.To[string](args["system_prompt"]),
		MaxTurns:     gconv.To[int](args["max_turns"]),
	}

	sess, err := t.sessions.CreateWithLimit(backendName, workingDir)
	if err != nil {
		return nil, err
	}

	async := gconv.To[bool](args["async"])
	logs.CtxInfo(ctx, "[tool:agent] create session %s, backend=%s, async=%v", sess.ID, backendName, async)

	if async {
		proc, err := backend.Start(ctx, req)
		if err != nil {
			t.sessions.Destroy(sess.ID)
			return nil, err
		}
		sess.process = proc
		go func() {
			<-proc.Done()
			raw := proc.Result()
			parsed := backend.ParseOutput(raw.Output, raw.ExitCode)
			status := StatusCompleted
			if parsed.ExitCode != 0 {
				status = StatusFailed
			}
			sess.SetResult(parsed.CLISessionID, parsed.Output, status)
		}()
		return map[string]interface{}{
			"session_id": sess.ID,
			"backend":    backendName,
			"status":     StatusRunning,
		}, nil
	}

	// Sync execution
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	res, err := backend.Run(timeoutCtx, req)
	if err != nil {
		sess.SetResult("", "", StatusFailed)
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	sess.SetResult(res.CLISessionID, res.Output, StatusCompleted)

	return map[string]interface{}{
		"session_id":     sess.ID,
		"backend":        backendName,
		"cli_session_id": res.CLISessionID,
		"status":         StatusCompleted,
		"result":         res.Output,
	}, nil
}

func (t *AgentTool) executeSend(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sessionID := strings.TrimSpace(gconv.To[string](args["session_id"]))
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required for send action")
	}
	prompt := gconv.To[string](args["prompt"])
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required for send action")
	}

	sess, ok := t.sessions.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	cliSessionID, _, _ := sess.Snapshot()
	if cliSessionID == "" {
		return nil, fmt.Errorf("session %s has no CLI session ID (was it completed?)", sessionID)
	}

	backend, ok := t.backends[sess.Backend]
	if !ok {
		return nil, fmt.Errorf("backend %s not available", sess.Backend)
	}

	req := &RunRequest{
		Prompt:     prompt,
		WorkingDir: sess.WorkingDir,
		ResumeID:   cliSessionID,
	}

	async := gconv.To[bool](args["async"])
	logs.CtxInfo(ctx, "[tool:agent] send to session %s, resume=%s, async=%v", sessionID, sess.CLISessionID, async)

	if async {
		sess.SetResult("", "", StatusRunning)
		proc, err := backend.Start(ctx, req)
		if err != nil {
			return nil, err
		}
		sess.process = proc
		go func() {
			<-proc.Done()
			raw := proc.Result()
			parsed := backend.ParseOutput(raw.Output, raw.ExitCode)
			status := StatusCompleted
			if parsed.ExitCode != 0 {
				status = StatusFailed
			}
			sess.SetResult(parsed.CLISessionID, parsed.Output, status)
		}()
		return map[string]interface{}{
			"session_id": sess.ID,
			"status":     StatusRunning,
		}, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	sess.SetResult("", "", StatusRunning)
	res, err := backend.Run(timeoutCtx, req)
	if err != nil {
		sess.SetResult("", "", StatusFailed)
		return nil, fmt.Errorf("agent send failed: %w", err)
	}

	sess.SetResult(res.CLISessionID, res.Output, StatusCompleted)

	return map[string]interface{}{
		"session_id":     sess.ID,
		"cli_session_id": res.CLISessionID,
		"status":         StatusCompleted,
		"result":         res.Output,
	}, nil
}

func (t *AgentTool) executeStatus(args map[string]interface{}) (interface{}, error) {
	sessionID := strings.TrimSpace(gconv.To[string](args["session_id"]))
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required for status action")
	}

	sess, ok := t.sessions.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	cliSessionID, lastOutput, status := sess.Snapshot()
	result := map[string]interface{}{
		"session_id": sess.ID,
		"backend":    sess.Backend,
		"status":     status,
		"created_at": sess.CreatedAt.Format(time.RFC3339),
	}
	if cliSessionID != "" {
		result["cli_session_id"] = cliSessionID
	}
	if lastOutput != "" {
		result["result"] = lastOutput
	}
	return result, nil
}

func (t *AgentTool) executeList() (interface{}, error) {
	sessions := t.sessions.List()
	list := make([]map[string]interface{}, 0, len(sessions))
	for _, s := range sessions {
		list = append(list, map[string]interface{}{
			"session_id": s.ID,
			"backend":    s.Backend,
			"status":     s.Status,
			"created_at": s.CreatedAt.Format(time.RFC3339),
		})
	}
	return map[string]interface{}{"sessions": list}, nil
}

func (t *AgentTool) executeDestroy(args map[string]interface{}) (interface{}, error) {
	sessionID := strings.TrimSpace(gconv.To[string](args["session_id"]))
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required for destroy action")
	}

	_, ok := t.sessions.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	t.sessions.Destroy(sessionID)
	return map[string]interface{}{"success": true, "session_id": sessionID}, nil
}
