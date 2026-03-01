package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/provider"
)

const (
	defaultMaxIterations = 25

	loopNotifyDebounce = time.Second * 3
)

func (ag *Agent) runLoop(ctx context.Context, p provider.Provider, modelSpec *provider.ModelSpec, sess *session.Session, msg *channel.Message, cfg config.AgentRuntimeConfig) (*channel.Response, error) {
	// Inject session into context so CLI providers can access metadata.
	ctx = session.WithContext(ctx, sess)
	promptMsgs := ag.buildMessages(ctx, sess, msg)

	// Include user message in the prompt but defer session persistence
	// until the loop completes successfully, preventing orphaned user
	// messages when all models fail.
	userMsg := buildUserMessage(msg)
	promptMsgs = append(promptMsgs, userMsg)

	maxIterations := defaultMaxIterations
	if cfg.MaxIterations > 0 {
		maxIterations = cfg.MaxIterations
	}

	logs.CtxDebug(ctx, "[agent:%s] sending to provider %s:%s, messages count: %d, max_iterations: %d",
		ag.id, modelSpec.ProviderID, modelSpec.ModelName, len(promptMsgs), maxIterations)

	var finalMsg *schema.Message
	msgs := make([]*schema.Message, 0, 4)
	notifier := &loopNotifier{agent: ag, chatID: msg.ChatID}
	notifier.channel, _ = channel.Get(msg.ChannelID)

	var opts []model.Option
	if cfg.Temperature > 0 {
		opts = append(opts, model.WithTemperature(float32(cfg.Temperature)))
	}
	for iter := 0; iter < maxIterations; iter++ {
		llmResp, err := p.Generate(ctx, modelSpec.ModelName, append(promptMsgs, msgs...), opts...)
		if err != nil {
			logs.CtxWarn(ctx, "[agent:%s] LLM call to %s:%s failed: %v", ag.id, modelSpec.ProviderID, modelSpec.ModelName, err)
			return nil, err
		}
		if llmResp == nil {
			logs.CtxWarn(ctx, "[agent:%s] LLM call to %s:%s returned empty response", ag.id, modelSpec.ProviderID, modelSpec.ModelName)
			return nil, fmt.Errorf("LLM returned empty response from %s:%s", modelSpec.ProviderID, modelSpec.ModelName)
		}

		if logs.DefaultLogger().GetLevel() <= logs.DebugLevel {
			str, _ := sonic.MarshalString(llmResp)
			logs.CtxDebug(ctx, "[agent:%s:%d] llmResp: %+v", ag.id, iter, str)
		}
		if len(llmResp.ToolCalls) > 0 {
			notifier.send(ctx, llmResp.Content, llmResp.ReasoningContent)
			msgs = append(msgs, llmResp)
			for _, call := range llmResp.ToolCalls {
				logs.CtxDebug(ctx, "[agent:%s:%d] call: %+v", ag.id, iter, call)
				res, callErr := ag.tools.ExecuteToolCall(ctx, &call)
				callMsg := &schema.Message{
					Role:       schema.Tool,
					ToolName:   call.Function.Name,
					ToolCallID: call.ID,
				}
				if callErr != nil {
					logs.CtxWarn(ctx, "[agent:%s] tool %q (call_id=%s) failed: %v", ag.id, call.Function.Name, call.ID, callErr)
					callMsg.Content = "ERROR: " + callErr.Error()
				} else {
					jsonStr, marshalErr := sonic.MarshalString(res)
					if marshalErr != nil || jsonStr == "" {
						callMsg.Content = "{}"
					} else {
						callMsg.Content = jsonStr
					}
				}
				msgs = append(msgs, callMsg)
			}
			continue
		}

		finalMsg = llmResp
		break
	}

	if finalMsg == nil {
		// Iteration limit reached — ask LLM to summarize progress without tools.
		logs.CtxWarn(ctx, "[agent:%s] iteration limit (%d) reached, requesting summary", ag.id, maxIterations)
		finalMsg = ag.runLoopSummary(ctx, p, modelSpec, append(promptMsgs, msgs...))
	}

	// Commit user message, tool-call turns, and the final response to session.
	sess.Append(userMsg)
	for _, m := range msgs {
		sess.Append(m)
	}
	sess.Append(finalMsg)

	return &channel.Response{
		ID:       msg.ID,
		Content:  finalMsg.Content,
		Model:    modelSpec.ModelName,
		Provider: modelSpec.ProviderID,
	}, nil
}

// runLoopSummary makes one final LLM call without tools to summarize what has
// been accomplished and what remains when the iteration limit is exceeded.
func (ag *Agent) runLoopSummary(ctx context.Context,
	p provider.Provider,
	modelSpec *provider.ModelSpec,
	msgs []*schema.Message,
) *schema.Message {
	msgs = append(msgs, &schema.Message{
		Role:    schema.User,
		Content: "You have reached the maximum iteration limit. Please summarize what you have accomplished so far and what still remains to be done.",
	})

	resp, err := p.Generate(ctx, modelSpec.ModelName, msgs)
	if err != nil || resp == nil {
		logs.CtxWarn(ctx, "[agent:%s] summary generation failed: %v", ag.id, err)
		return &schema.Message{
			Role:    schema.Assistant,
			Content: "Task reached the maximum iteration limit. Partial work may have been applied. Please review and continue if needed.",
		}
	}
	return resp
}

type loopNotifier struct {
	agent    *Agent
	channel  channel.Channel
	chatID   string
	lastSend time.Time
}

// send delivers text to the user if the throttle window has elapsed.
// Empty text or missing channel is silently ignored.
func (n *loopNotifier) send(ctx context.Context, content, reasoning string) {
	if content == "" {
		content = reasoning
	}
	if n.channel == nil || content == "" {
		return
	}
	if now := time.Now(); now.Sub(n.lastSend) >= loopNotifyDebounce {
		if err := n.channel.SendMessage(ctx, n.chatID, content); err != nil {
			logs.CtxDebug(ctx, "[agent:%s] progress notify failed: %v", n.agent.id, err)
			return
		}
		n.lastSend = now
	}
}
