package agent

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/agent/skill"
	"github.com/tgifai/friday/internal/agent/tool"
	"github.com/tgifai/friday/internal/agent/tool/agentx"
	"github.com/tgifai/friday/internal/agent/tool/cronx"
	"github.com/tgifai/friday/internal/agent/tool/filex"
	"github.com/tgifai/friday/internal/agent/tool/msgx"
	"github.com/tgifai/friday/internal/agent/tool/qmdx"
	"github.com/tgifai/friday/internal/agent/tool/shellx"
	"github.com/tgifai/friday/internal/agent/tool/webx"
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

func (ag *Agent) Init(_ context.Context) error {
	// skills
	_ = ag.skills.LoadAll()

	allowedPaths := []string{ag.workspace}
	// file related tools
	_ = ag.tools.Register(filex.NewFileTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewReadTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewWriteTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewListTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewDeleteTool(ag.workspace, allowedPaths))
	_ = ag.tools.Register(filex.NewEditTool(ag.workspace, allowedPaths))

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

	// cron tools
	_ = ag.tools.Register(cronx.NewCronTool())

	// agent delegation tools
	_ = ag.tools.Register(agentx.NewAgentTool(ag.workspace))

	return nil
}

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
	sess := ag.sess.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)
	msg.SessionKey = sess.SessionKey
	defer func() {
		if err := ag.sess.Save(sess); err != nil {
			logs.CtxWarn(ctx, "[agent:%s] failed to persist session: %v", ag.id, err)
		}
	}()

	// Append the user message exactly once for this turn.
	sess.Append(buildUserMessage(msg))

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
		resp, err = ag.runLoop(ctx, prov, ms, sess, msg, agCfg.Config)
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

// buildUserMessage constructs a schema.Message from a channel message.
// When attachments are present, it builds a multimodal message with base64-
// encoded inline data; otherwise it falls back to a plain text message.
func buildUserMessage(msg *channel.Message) *schema.Message {
	if len(msg.Attachments) == 0 {
		return &schema.Message{Role: schema.User, Content: msg.Content}
	}

	var parts []schema.MessageInputPart

	if msg.Content != "" {
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeText,
			Text: msg.Content,
		})
	}

	for _, att := range msg.Attachments {
		b64 := base64.StdEncoding.EncodeToString(att.Data)
		switch att.Type {
		case channel.AttachmentImage:
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						Base64Data: &b64,
						MIMEType:   att.MIMEType,
					},
					Detail: schema.ImageURLDetailAuto,
				},
			})
		case channel.AttachmentVoice:
			// Most LLM providers (Anthropic, Volcengine, etc.) do not support
			// audio_url content parts. Instead of sending an unsupported type
			// that would cause the entire request to fail, we add a text note
			// indicating an audio message was received.
			name := att.FileName
			if name == "" {
				name = "audio"
			}
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeText,
				Text: fmt.Sprintf("[Audio attachment received: %s (%s), but audio input is not supported by the current model]", name, att.MIMEType),
			})
		}
	}

	return &schema.Message{
		Role:                  schema.User,
		UserInputMultiContent: parts,
	}
}
