package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/agent/skill"
	"github.com/tgifai/friday/internal/agent/tool"
	"github.com/tgifai/friday/internal/agent/tool/agentx"
	"github.com/tgifai/friday/internal/agent/tool/browserx"
	"github.com/tgifai/friday/internal/agent/tool/cronx"
	"github.com/tgifai/friday/internal/agent/tool/filex"
	"github.com/tgifai/friday/internal/agent/tool/httpx"
	"github.com/tgifai/friday/internal/agent/tool/mcpx"
	"github.com/tgifai/friday/internal/agent/tool/msgx"
	"github.com/tgifai/friday/internal/agent/tool/qmdx"
	"github.com/tgifai/friday/internal/agent/tool/shellx"
	"github.com/tgifai/friday/internal/agent/tool/timex"
	"github.com/tgifai/friday/internal/agent/tool/webx"
	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/cronjob"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/pkg/utils"
	"github.com/tgifai/friday/internal/provider"
)

const (
	defaultContextBudget = 128_000 // 128k
	defaultReserveTokens = 20_000
)

// EnqueueFunc is a callback to submit messages into the gateway pipeline.
type EnqueueFunc func(ctx context.Context, msg *channel.Message) error

type Agent struct {
	id        string
	name      string
	workspace string

	tools   *tool.Registry
	skills  *skill.Registry
	mcpMgr  *mcpx.MCPTool
	sessMgr *session.Manager

	enqueue          EnqueueFunc // allows agent to self-enqueue messages (set by gateway)
	consolidateEvery int
	flushCooldown    time.Duration
	contextBudget    int
	reserveTokens    int
	toolsRegistered  sync.Map // providerID → true; ensures RegisterTools is called once per provider
}

func NewAgent(_ context.Context, cfg config.AgentConfig) (*Agent, error) {
	// session manager
	sessMgr, err := session.NewJSONLManager(cfg.ID, cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("init session manager: %w", err)
	}

	// Wire TTL from config.
	if cfg.Session.TTL != "" {
		ttl, err := time.ParseDuration(cfg.Session.TTL)
		if err != nil {
			return nil, fmt.Errorf("parse session TTL %q: %w", cfg.Session.TTL, err)
		}
		sessMgr.SetTTL(ttl)
	}

	consolidateEvery := cfg.Session.ConsolidateEvery
	if consolidateEvery <= 0 {
		consolidateEvery = 50
	}

	flushCooldown := 2 * time.Hour
	if cfg.Session.FlushCooldown != "" {
		cd, err := time.ParseDuration(cfg.Session.FlushCooldown)
		if err != nil {
			return nil, fmt.Errorf("parse flush cooldown %q: %w", cfg.Session.FlushCooldown, err)
		}
		flushCooldown = cd
	}

	contextBudget := cfg.Session.ContextBudget
	if contextBudget <= 0 {
		contextBudget = defaultContextBudget
	}
	reserveTokens := cfg.Session.ReserveTokens
	if reserveTokens <= 0 {
		reserveTokens = defaultReserveTokens
	}

	ag := &Agent{
		id:               cfg.ID,
		name:             cfg.Name,
		workspace:        cfg.Workspace,
		sessMgr:          sessMgr,
		tools:            tool.NewRegistry(),
		skills:           skill.NewRegistry(cfg.Workspace),
		consolidateEvery: consolidateEvery,
		flushCooldown:    flushCooldown,
		contextBudget:    contextBudget,
		reserveTokens:    reserveTokens,
	}

	return ag, nil
}

func (ag *Agent) ID() string {
	return ag.id
}

func (ag *Agent) Name() string {
	return ag.name
}

func (ag *Agent) Workspace() string {
	return ag.workspace
}

func (ag *Agent) Init(ctx context.Context) error {
	// Bootstrap workspace prompt files from embedded templates.
	ag.bootstrapWorkspace()

	// Start session GC if TTL is configured.
	if ag.sessMgr.TTL() > 0 {
		ag.sessMgr.StartGCLoop(ctx, 0) // 0 = default 10min interval
	}

	// skills
	if err := ag.skills.LoadAll(); err != nil {
		logs.Warn("[agent:%s] failed to load skills: %v", ag.id, err)
	}

	allowedPaths := []string{ag.workspace}
	// Merge extra allowed paths from macOS sandbox bookmarks (colon-separated).
	if paths := os.Getenv("FRIDAY_ALLOWED_PATHS"); paths != "" {
		for _, p := range strings.Split(paths, ":") {
			p = strings.TrimSpace(p)
			if p != "" {
				allowedPaths = append(allowedPaths, p)
			}
		}
	}
	// file related tools
	_ = ag.tools.Register(filex.NewFileTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewReadTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewWriteTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewListTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewDeleteTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewEditTool(ag.workspace, allowedPaths))

	// time tool
	_ = ag.tools.Register(timex.NewTimeTool())

	// msg related tools
	_ = ag.tools.Register(msgx.NewMessageTool())

	// shell related tools
	_ = ag.tools.Register(shellx.NewExecTool(ag.workspace))
	_ = ag.tools.Register(shellx.NewProcessTool(ag.workspace))

	// knowledge base tools (only if qmd CLI is available)
	if qmdx.Available() {
		_ = ag.tools.Register(qmdx.NewSearchTool())
		_ = ag.tools.Register(qmdx.NewGetTool())
	}

	// web tools
	_ = ag.tools.Register(webx.NewFetchTool())
	_ = ag.tools.Register(webx.NewSearchTool())

	// http tools
	_ = ag.tools.Register(httpx.NewRequestTool())

	// cron tools
	_ = ag.tools.Register(cronx.NewCronTool())

	// agent delegation tools
	_ = ag.tools.Register(agentx.NewAgentTool(ag.workspace))

	// browser automation tools
	_ = ag.tools.Register(browserx.NewBrowserTool())

	// MCP server tools
	ag.mcpMgr = mcpx.NewMCPTool()
	if err := ag.mcpMgr.LoadConfig(ctx, ag.workspace); err != nil {
		logs.Warn("[agent:%s] failed to load mcp config: %v", ag.id, err)
	}
	_ = ag.tools.Register(ag.mcpMgr)

	return nil
}

// bootstrapWorkspace writes embedded prompt templates to the workspace directory.
// Managed files (e.g. TOOLS.md, SECURITY.md) are always overwritten to stay in
// sync with the binary. User-editable files are only created when missing.
func (ag *Agent) bootstrapWorkspace() {
	for name, content := range consts.WorkspaceMarkdownTemplates {
		dst := filepath.Join(ag.workspace, name)

		if consts.WorkspaceManagedFiles[name] {
			// Always overwrite managed files.
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				logs.Warn("[agent:%s] bootstrap: mkdir %s: %v", ag.id, filepath.Dir(dst), err)
				continue
			}
			if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
				logs.Warn("[agent:%s] bootstrap: write %s: %v", ag.id, name, err)
			}
			continue
		}

		// User-editable files: create only if missing.
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			logs.Warn("[agent:%s] bootstrap: mkdir %s: %v", ag.id, filepath.Dir(dst), err)
			continue
		}
		if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
			logs.Warn("[agent:%s] bootstrap: write %s: %v", ag.id, name, err)
		}
	}
}

func (ag *Agent) ProcessMessage(ctx context.Context, msg *channel.Message) (*channel.Response, error) {
	logs.CtxDebug(ctx, "[agent:%s] received message from channel %s, user %s: %s",
		ag.id, string(msg.ChannelType), msg.UserID, utils.Truncate80(msg.Content))
	if ag.sessMgr == nil {
		return nil, fmt.Errorf("session manager is not initialized for agent: %s", ag.id)
	}
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}
	agCfg, ok := cfg.Agents[ag.id]
	if !ok {
		return nil, fmt.Errorf("agent %s not found", ag.id)
	}

	// get or create current session
	var sess *session.Session
	if msg.SessionKey != "" {
		sess = ag.sessMgr.GetOrCreate(msg.SessionKey)
	} else {
		sess = ag.sessMgr.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)
	}
	msg.SessionKey = sess.SessionKey
	defer func() {
		if err := ag.sessMgr.Save(sess); err != nil {
			logs.CtxWarn(ctx, "[agent:%s] failed to persist session: %v", ag.id, err)
		}
	}()

	var resp *channel.Response
	models := append([]string{agCfg.Models.Primary}, agCfg.Models.Fallback...)
	ch, _ := channel.Get(msg.ChannelID)
	var modelErrors []string
	for _, spec := range models {
		ms, err := provider.ParseModelSpec(spec)
		if err != nil {
			logs.CtxWarn(ctx, "[agent:%s] invalid model spec %q: %v", ag.id, spec, err)
			continue
		}
		prov, err := provider.Get(ms.ProviderID)
		if err != nil {
			logs.CtxWarn(ctx, "[agent:%s] provider not found: %s", ag.id, ms.ProviderID)
			continue
		}
		if _, loaded := ag.toolsRegistered.LoadOrStore(ms.ProviderID, true); !loaded {
			prov.RegisterTools(ag.tools.ListToolInfos())
		}
		resp, err = ag.runLoop(ctx, prov, ms, sess, msg, agCfg.Config)
		if err != nil {
			logs.CtxWarn(ctx, "[agent:%s] model %s failed: %v", ag.id, ms, err)
			errMsg := fmt.Sprintf("[%s] error: %v", spec, err)
			modelErrors = append(modelErrors, errMsg)
			if ch != nil {
				_ = ch.SendMessage(ctx, msg.ChatID, errMsg)
			}
			continue
		}
		break
	}

	// fallback response — all models failed
	if resp == nil {
		fallbackContent := "All models failed:\n\n" + strings.Join(modelErrors, "\n")
		resp = &channel.Response{
			ID:      msg.ID,
			Content: fallbackContent,
		}
	}

	// Check if session has crossed the consolidation threshold.
	ag.maybeEnqueueFlush(ctx, sess)

	// Clear isolated cron sessions to prevent unbounded history growth.
	// Isolated sessions use key prefix "cron:" (vs "agent:" for main/heartbeat).
	if msg.ChannelType == channel.Type("cron") && strings.HasPrefix(msg.SessionKey, "cron:") {
		sess.Clear()
	}

	return resp, nil
}

// Close releases resources held by the agent (e.g. MCP server connections).
func (ag *Agent) Close() error {
	if ag.mcpMgr != nil {
		return ag.mcpMgr.Close()
	}
	return nil
}

// SetEnqueue gives the agent the ability to enqueue messages into the gateway
// pipeline. This is called during gateway initialization.
func (ag *Agent) SetEnqueue(fn EnqueueFunc) {
	ag.enqueue = fn
}

// ResetSession clears the current session for the given message's channel/chat.
// Before clearing, it archives a brief summary of user messages to today's
// daily memory file. No LLM call is made.
func (ag *Agent) ResetSession(ctx context.Context, msg *channel.Message) (string, error) {
	sess := ag.sessMgr.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)

	history := sess.History()
	msgCount := sess.MsgCount()

	// Archive user messages to today's daily memory file.
	if len(history) > 0 {
		if err := ag.archiveSessionToDailyMemory(history); err != nil {
			logs.CtxWarn(ctx, "[agent:%s] archive session to daily memory: %v", ag.id, err)
		}
	}

	sess.Clear()

	if err := ag.sessMgr.Save(sess); err != nil {
		return "", fmt.Errorf("save cleared session: %w", err)
	}

	if msgCount == 0 {
		return "Session is already empty. Ready for a fresh start!", nil
	}
	return fmt.Sprintf("Session cleared (%d messages archived). Starting fresh!", msgCount), nil
}

// archiveSessionToDailyMemory appends a brief text summary of user messages
// from the given history to today's daily memory file.
func (ag *Agent) archiveSessionToDailyMemory(history []*schema.Message) error {
	now := time.Now()
	dailyPath := filepath.Join(ag.workspace, consts.DailyMemoryFile(now))

	if err := os.MkdirAll(filepath.Dir(dailyPath), 0o755); err != nil {
		return fmt.Errorf("create daily dir: %w", err)
	}

	const maxArchiveMessages = 10
	const maxRuneLen = 200
	var lines []string
	for _, msg := range history {
		if msg.Role != schema.User || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		content := msg.Content
		if utf8.RuneCountInString(content) > maxRuneLen {
			runes := []rune(content)
			content = string(runes[:maxRuneLen]) + "..."
		}
		lines = append(lines, "- "+content)
		if len(lines) >= maxArchiveMessages {
			break
		}
	}
	if len(lines) == 0 {
		return nil
	}

	f, err := os.OpenFile(dailyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open daily file: %w", err)
	}
	defer f.Close()

	header := fmt.Sprintf("\n## Session Reset (%s)\n\n", now.Format("15:04"))
	if _, err := f.WriteString(header + strings.Join(lines, "\n") + "\n"); err != nil {
		return fmt.Errorf("write to daily file: %w", err)
	}
	return nil
}

// maybeEnqueueFlush checks if the session message count has crossed a
// consolidation threshold and, if so, enqueues a flush job to write context
// to memory files. Respects a cooldown to avoid excessive flushes.
func (ag *Agent) maybeEnqueueFlush(ctx context.Context, sess *session.Session) {
	if ag.enqueue == nil || ag.consolidateEvery <= 0 {
		logs.CtxDebug(ctx, "[agent:%s] flush skip: enqueue=%v consolidateEvery=%d", ag.id, ag.enqueue != nil, ag.consolidateEvery)
		return
	}

	count := sess.MsgCount()
	threshold := int64(ag.consolidateEvery)

	// Check if we've crossed a threshold boundary since the last flush.
	lastFlushCntStr := sess.GetMeta("flush_at_msg_cnt")
	lastFlushCnt := int64(0)
	if lastFlushCntStr != "" {
		lastFlushCnt, _ = strconv.ParseInt(lastFlushCntStr, 10, 64)
	}
	if count-lastFlushCnt < threshold {
		logs.CtxDebug(ctx, "[agent:%s] flush skip: count=%d lastFlushCnt=%d threshold=%d", ag.id, count, lastFlushCnt, threshold)
		return
	}

	// Check cooldown.
	if lastFlush := sess.GetMeta("last_flush_at"); lastFlush != "" {
		if t, err := time.Parse(time.RFC3339, lastFlush); err == nil {
			if time.Since(t) < ag.flushCooldown {
				logs.CtxDebug(ctx, "[agent:%s] flush skip: cooldown not elapsed, last=%s", ag.id, lastFlush)
				return
			}
		}
	}

	// Build flush prompt.
	now := time.Now()
	prompt, hasWork := cronjob.BuildFlushPrompt(ag.workspace, now)
	if !hasWork {
		logs.CtxDebug(ctx, "[agent:%s] flush skip: no session activity to flush", ag.id)
		return
	}

	// Record flush state before enqueuing.
	sess.SetMeta("last_flush_at", now.Format(time.RFC3339))
	sess.SetMeta("flush_at_msg_cnt", strconv.FormatInt(count, 10))

	sessionKey := fmt.Sprintf("cron:__consolidation_flush__:%s", ag.id)
	flushMsg := &channel.Message{
		ID:          fmt.Sprintf("flush-%s-%d", ag.id, now.UnixMilli()),
		ChannelType: channel.Type("cron"),
		Content:     prompt,
		SessionKey:  sessionKey,
		Metadata: map[string]string{
			"agent_id":      ag.id,
			"cron_job_name": "consolidation-flush",
			"cron_job_id":   "__consolidation_flush__:" + ag.id,
		},
	}

	// Detach from request context so the flush is not cancelled when the
	// HTTP handler returns.
	flushCtx := context.WithoutCancel(ctx)
	go func() {
		if err := ag.enqueue(flushCtx, flushMsg); err != nil {
			logs.CtxWarn(flushCtx, "[agent:%s] consolidation flush enqueue failed: %v", ag.id, err)
		} else {
			logs.CtxInfo(flushCtx, "[agent:%s] consolidation flush triggered at msg count %d", ag.id, count)
		}
	}()
}
