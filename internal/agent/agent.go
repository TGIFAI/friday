package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/agent/skill"
	"github.com/tgifai/friday/internal/agent/tool"
	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/pkg/utils"
	"github.com/tgifai/friday/internal/provider"
)

type Agent struct {
	id        string
	name      string
	workspace string

	tools  *tool.Registry
	skills *skill.Registry
	sess   *session.Manager
}

func NewAgent(_ context.Context, cfg config.AgentConfig) (*Agent, error) {
	// session manager
	sessMgr, err := session.NewJSONLManager(cfg.ID, cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("init session manager: %w", err)
	}

	ag := &Agent{
		id:        cfg.ID,
		name:      cfg.Name,
		workspace: cfg.Workspace,
		sess:      sessMgr,
		tools:     tool.NewRegistry(),
		skills:    skill.NewRegistry(cfg.Workspace),
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

func (ag *Agent) Init(ctx context.Context) error { return nil }

func (ag *Agent) ProcessMessage(ctx context.Context, msg *channel.Message) (*channel.Response, error) {
	logs.CtxDebug(ctx, "[agent:%s] received message from channel %s, user %s: %s",
		ag.id, string(msg.ChannelType), msg.UserID, utils.Truncate80(msg.Content))
	if ag.sess == nil {
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
	sess := ag.sess.GetOrCreateFor(msg.ChannelType, msg.ChatID)
	msg.SessionKey = sess.SessionKey
	defer func() {
		if err := ag.sess.Save(sess); err != nil {
			logs.CtxWarn(ctx, "[agent:%s] failed to persist session: %v", ag.id, err)
		}
	}()

	// TODO there might be some medias
	// Append the user message exactly once for this turn.
	sess.Append(&schema.Message{Role: "user", Content: msg.Content})

	var resp *channel.Response
	models := append([]string{agCfg.Models.Primary}, agCfg.Models.Fallback...)
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
		resp, err = ag.runLoop(ctx, prov, ms, sess, msg)
		if err != nil {
			logs.CtxWarn(ctx, "[agent:%s] model %s failed: %v", ag.id, ms, err)
			continue
		}
		break
	}

	// fallback response
	if resp == nil {
		resp = &channel.Response{
			ID:      msg.ID,
			Content: "System might be unavailable, please try again later.",
		}
	}
	return resp, nil
}
