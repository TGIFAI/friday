package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	hzServer "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/tgifai/friday/internal/agent"
	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/channel/lark"
	"github.com/tgifai/friday/internal/channel/telegram"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/cronjob"
	"github.com/tgifai/friday/internal/pkg/logs"
	pkgutils "github.com/tgifai/friday/internal/pkg/utils"
	"github.com/tgifai/friday/internal/provider"
	"github.com/tgifai/friday/internal/provider/anthropic"
	"github.com/tgifai/friday/internal/provider/gemini"
	"github.com/tgifai/friday/internal/provider/ollama"
	"github.com/tgifai/friday/internal/provider/openai"
	"github.com/tgifai/friday/internal/provider/qwen"
)

const typingInterval = 3 * time.Second

type Gateway struct {
	agents     sync.Map
	commands   *CommandRouter
	security   *SecurityGuard
	msgQueue   *MessageQueue
	httpServer *hzServer.Hertz
	scheduler  *cronjob.Scheduler

	runCtx    context.Context
	runCancel context.CancelFunc

	mu       sync.Mutex
	stopOnce sync.Once
	stopErr  error
}

func NewGateway(cfg config.GatewayConfig) *Gateway {
	bind := cfg.Bind
	if bind == "" {
		bind = "0.0.0.0:8080"
	}

	timeout := time.Duration(cfg.RequestTimeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	hzSvr := hzServer.Default(
		hzServer.WithHostPorts(bind),
		hzServer.WithReadTimeout(timeout),
		hzServer.WithWriteTimeout(timeout),
		hzServer.WithExitWaitTime(5*time.Second),
	)

	commands := newCommandRouter()
	registerBuiltinCommands(commands)

	gw := &Gateway{
		httpServer: hzSvr,
		commands:   commands,
		security:   &SecurityGuard{},
		msgQueue: newMessageQueue(QueueOptions{
			LaneBuffer:    10,
			MaxConcurrent: cfg.MaxConcurrentSessions,
		}),
	}

	return gw
}

func (gw *Gateway) Start(ctx context.Context) error {
	gw.runCtx, gw.runCancel = context.WithCancel(ctx)

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	if err := gw.msgQueue.Init(gw.runCtx, gw.processMessage); err != nil {
		return fmt.Errorf("init msg queue: %w", err)
	}
	if err := gw.initHTTPServer(gw.runCtx, cfg.Gateway); err != nil {
		return fmt.Errorf("init http server: %w", err)
	}
	if err := gw.initProviders(gw.runCtx, cfg.Providers); err != nil {
		return fmt.Errorf("init providers: %w", err)
	}
	if err := gw.initAgents(gw.runCtx, cfg.Agents); err != nil {
		return fmt.Errorf("init agents: %w", err)
	}
	if err := gw.initChannels(gw.runCtx, cfg.Channels); err != nil {
		return fmt.Errorf("init channels: %w", err)
	}
	if err := gw.initCronjob(gw.runCtx, cfg); err != nil {
		return fmt.Errorf("init cronjob: %w", err)
	}

	go gw.httpServer.Spin()

	return nil
}

func (gw *Gateway) Stop(ctx context.Context) error {
	gw.stopOnce.Do(func() {
		if gw.scheduler != nil {
			gw.scheduler.Stop(ctx)
		}

		if gw.runCancel != nil {
			gw.runCancel()
		}

		for _, ch := range channel.List() {
			if err := ch.Stop(ctx); err != nil {
				logs.CtxWarn(ctx, "[gateway] stop channel %s error: %v", ch.ID(), err)
			}
		}

		if err := gw.httpServer.Shutdown(ctx); err != nil {
			logs.CtxWarn(ctx, "[gateway] shutdown http server error: %v", err)
		}

		logs.CtxInfo(ctx, "[gateway] all resources stopped")
	})
	return gw.stopErr
}

func (gw *Gateway) initProviders(ctx context.Context, providers map[string]config.ProviderConfig) error {
	for id, cfg := range providers {
		cfg.ID = id
		p, err := newProvider(ctx, cfg)
		if err != nil {
			logs.CtxError(ctx, "[%s] create provider #%s error: %v", strings.ToUpper(cfg.Type), cfg.ID, err)
			return fmt.Errorf("create provider %s: %w", cfg.ID, err)
		}

		if err = provider.Register(p); err != nil {
			logs.CtxError(ctx, "[%s] register provider #%s error: %v", strings.ToUpper(cfg.Type), cfg.ID, err)
			return fmt.Errorf("register provider %s: %w", cfg.ID, err)
		}

		logs.CtxInfo(ctx, "[%s] register provider #%s success", strings.ToUpper(cfg.Type), cfg.ID)
	}
	return nil
}

func newProvider(ctx context.Context, cfg config.ProviderConfig) (provider.Provider, error) {
	cfgMap := make(map[string]interface{}, len(cfg.Config))
	for k, v := range cfg.Config {
		cfgMap[k] = v
	}

	switch provider.Type(strings.ToLower(strings.TrimSpace(cfg.Type))) {
	case provider.OpenAI:
		return openai.NewProvider(ctx, cfg.ID, cfgMap)
	case provider.Anthropic:
		return anthropic.NewProvider(ctx, cfg.ID, cfgMap)
	case provider.Gemini:
		return gemini.NewProvider(ctx, cfg.ID, cfgMap)
	case provider.Ollama:
		return ollama.NewProvider(ctx, cfg.ID, cfgMap)
	case provider.Qwen:
		return qwen.NewProvider(ctx, cfg.ID, cfgMap)
	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Type)
	}
}

func (gw *Gateway) initAgents(ctx context.Context, agents map[string]config.AgentConfig) error {
	for id, cfg := range agents {
		cfg.ID = id

		ag, err := agent.NewAgent(ctx, cfg)
		if err != nil {
			logs.CtxError(ctx, "[gateway] create agent #%s error: %v", id, err)
			return fmt.Errorf("create agent %s: %w", id, err)
		}

		if err = ag.Init(ctx); err != nil {
			logs.CtxError(ctx, "[gateway] init agent #%s error: %v", id, err)
			return fmt.Errorf("init agent %s: %w", id, err)
		}

		gw.agents.Store(id, ag)
		logs.CtxInfo(ctx, "[gateway] register agent #%s success", id)
	}
	return nil
}

func (gw *Gateway) initChannels(ctx context.Context, channels map[string]config.ChannelConfig) error {
	for id, cfg := range channels {
		cfg.ID = id
		if !cfg.Enabled {
			logs.CtxInfo(ctx, "[gateway] channel #%s is disabled, skipping", id)
			continue
		}

		ch, err := newChannel(id, cfg, gw.httpServer)
		if err != nil {
			logs.CtxError(ctx, "[gateway] create channel #%s error: %v", id, err)
			return fmt.Errorf("create channel %s: %w", id, err)
		}

		if err = ch.RegisterMessageHandler(gw.enqueueMsg); err != nil {
			return fmt.Errorf("register handler for channel %s: %w", id, err)
		}

		if err = channel.Register(ch); err != nil {
			return fmt.Errorf("register channel %s: %w", id, err)
		}

		go func(id string, ch channel.Channel) {
			logs.CtxInfo(ctx, "[gateway] starting channel #%s (%s)", id, ch.Type())
			if err := ch.Start(ctx); err != nil {
				logs.CtxError(ctx, "[gateway] channel #%s stopped with error: %v", id, err)
			}
		}(id, ch)
	}
	return nil
}

func newChannel(id string, cfg config.ChannelConfig, httpServer *hzServer.Hertz) (channel.Channel, error) {
	switch channel.Type(strings.ToLower(strings.TrimSpace(cfg.Type))) {
	case channel.Telegram:
		return telegram.NewChannel(id, &cfg)
	case channel.Lark:
		return lark.NewChannel(id, &cfg, httpServer)
	default:
		return nil, fmt.Errorf("unsupported channel type: %s", cfg.Type)
	}
}

func (gw *Gateway) initHTTPServer(ctx context.Context, gateway config.GatewayConfig) error {

	gw.httpServer.GET("/health", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(consts.StatusOK, utils.H{"status": "ok"})
	})
	return nil

}

func (gw *Gateway) enqueueMsg(ctx context.Context, msg *channel.Message) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}

	if msg.SessionKey == "" {
		ag, err := gw.getAgentByChannel(msg.ChannelID)
		if err != nil {
			return err
		}
		msg.SessionKey = session.GenerateKey(ag.ID(), msg.ChannelType, msg.ChannelID, msg.ChatID)
	}
	return gw.msgQueue.Enqueue(ctx, msg)
}

func (gw *Gateway) processMessage(ctx context.Context, msg *channel.Message) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}

	// --- cron job messages ---
	if msg.ChannelType == channel.Type("cron") {
		return gw.processCronMessage(ctx, msg)
	}

	// --- normal channel messages ---
	logs.CtxDebug(ctx, "[msg] <- (%s/%s#%s) %s", msg.ChannelType, msg.ChannelID, msg.UserID, pkgutils.Truncate80(msg.Content))

	ch, err := channel.Get(msg.ChannelID)
	if err != nil {
		return fmt.Errorf("channel %s not found: %w", msg.ChannelID, err)
	}

	// 1. Security check (ACL + pairing).
	cfg, err := config.Get()
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	chCfg, chCfgOK := cfg.Channels[msg.ChannelID]
	if chCfgOK {
		allowed, reply := gw.security.Check(ctx, msg, chCfg)
		if reply != "" {
			_ = ch.SendMessage(ctx, msg.ChatID, reply)
		}
		if !allowed {
			return nil
		}
	}

	// 2. Command interception — bypass agent for built-in commands.
	if cmd, _, matched := gw.commands.Match(msg.Content); matched {
		reply, cmdErr := cmd.Handler(ctx, gw, msg)
		if cmdErr != nil {
			return fmt.Errorf("command %s failed: %w", cmd.Name, cmdErr)
		}
		if reply != "" {
			_ = ch.SendMessage(ctx, msg.ChatID, reply)
		}
		return nil
	}

	// 3. Normal agent processing.
	ag, err := gw.getAgentByChannel(msg.ChannelID)
	if err != nil {
		return err
	}

	stopTyping := gw.keepTyping(ctx, ch, msg.ChatID)
	resp, err := ag.ProcessMessage(ctx, msg)
	stopTyping()
	if err != nil {
		return fmt.Errorf("agent %s process message failed: %w", ag.ID(), err)
	}

	if resp == nil || resp.Content == "" {
		return nil
	}

	if err := ch.SendMessage(ctx, msg.ChatID, resp.Content); err != nil {
		return fmt.Errorf("send reply via channel %s failed: %w", msg.ChannelID, err)
	}
	logs.CtxDebug(ctx, "[msg] -> (%s/%s#%s) %s", msg.ChannelType, msg.ChannelID, msg.ChatID, pkgutils.Truncate80(resp.Content))
	return nil
}

func (gw *Gateway) processCronMessage(ctx context.Context, msg *channel.Message) error {
	agentID := msg.Metadata["agent_id"]
	if agentID == "" {
		return fmt.Errorf("cron message missing agent_id in metadata")
	}

	val, ok := gw.agents.Load(agentID)
	if !ok {
		return fmt.Errorf("agent %s not found for cron job", agentID)
	}
	ag := val.(*agent.Agent)

	logs.CtxDebug(ctx, "[cron] -> agent=%s job=%s", agentID, msg.Metadata["cron_job_name"])

	resp, err := ag.ProcessMessage(ctx, msg)
	if err != nil {
		return fmt.Errorf("agent %s cron job failed: %w", agentID, err)
	}
	if resp == nil || resp.Content == "" {
		return nil
	}

	// Heartbeat silent: if agent returns HEARTBEAT_OK, do not deliver.
	if strings.TrimSpace(resp.Content) == cronjob.HeartbeatOK {
		logs.CtxDebug(ctx, "[cron] heartbeat OK, nothing to deliver")
		return nil
	}

	// Deliver to configured channel (isolated jobs only).
	if msg.ChannelID != "" {
		ch, err := channel.Get(msg.ChannelID)
		if err != nil {
			logs.CtxWarn(ctx, "[cron] delivery channel %s not found: %v", msg.ChannelID, err)
			return nil
		}
		if err := ch.SendMessage(ctx, msg.ChatID, resp.Content); err != nil {
			return fmt.Errorf("cron job send reply via channel %s failed: %w", msg.ChannelID, err)
		}
	}

	return nil
}

func (gw *Gateway) getAgentByChannel(channelID string) (*agent.Agent, error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}

	agentID := ""
	for id, agCfg := range cfg.Agents {
		for _, chID := range agCfg.Channels {
			if chID == channelID {
				agentID = id
				break
			}
		}
		if agentID != "" {
			break
		}
	}
	if agentID == "" {
		return nil, fmt.Errorf("no agent bound to channel %s", channelID)
	}

	val, ok := gw.agents.Load(agentID)
	if !ok {
		return nil, fmt.Errorf("agent %s not found in registry", agentID)
	}
	return val.(*agent.Agent), nil
}

func (gw *Gateway) initCronjob(ctx context.Context, cfg *config.Config) error {
	if cfg.Cronjob.Enabled != nil && !*cfg.Cronjob.Enabled {
		logs.CtxInfo(ctx, "[gateway] cronjob disabled, skipping")
		return nil
	}

	gw.scheduler = cronjob.NewScheduler(cfg.Cronjob, gw.enqueueMsg)

	// Register a built-in heartbeat job for every agent.
	for id, agCfg := range cfg.Agents {
		hbJob := cronjob.NewHeartbeatJob(id, agCfg.Workspace, 0)
		if err := gw.scheduler.AddJob(hbJob, false); err != nil {
			logs.CtxWarn(ctx, "[gateway] register heartbeat for agent %s: %v", id, err)
		}
	}

	return gw.scheduler.Start(ctx)
}

func (gw *Gateway) keepTyping(ctx context.Context, ch channel.Channel, chatID string) (stop func()) {
	_ = ch.SendChatAction(ctx, chatID, channel.ChatActionTyping)

	ticker := time.NewTicker(typingInterval)
	done := make(chan struct{})

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = ch.SendChatAction(ctx, chatID, channel.ChatActionTyping)
			}
		}
	}()

	return func() { close(done) }
}
