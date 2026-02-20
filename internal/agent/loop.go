package agent

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/provider"
)

const defaultMaxIterations = 25

func (ag *Agent) runLoop(ctx context.Context, p provider.Provider, ms *provider.ModelSpec, sess *session.Session, msg *channel.Message, cfg config.AgentRuntimeConfig) (*channel.Response, error) {
	maxIterations := defaultMaxIterations
	if cfg.MaxIterations > 0 {
		maxIterations = cfg.MaxIterations
	}

	// The current user message has already been appended to the session in ProcessMessage.
	msgs := ag.buildMessages(sess, msg, false)

	logs.CtxDebug(ctx, "[agent:%s] sending to provider %s:%s, messages count: %d, max_iterations: %d",
		ag.id, ms.ProviderID, ms.ModelName, len(msgs), maxIterations)

	var finalResp *schema.Message
	opts := []model.Option{
		model.WithTools(ag.tools.ListToolInfos()),
		model.WithToolChoice(schema.ToolChoiceAllowed),
	}
	for iter := 0; iter < maxIterations; iter++ {
		llmResp, err := p.Generate(ctx, ms.ModelName, msgs, opts...)
		if err != nil || llmResp == nil {
			logs.CtxWarn(ctx, "agent msg generation (nil: %v) failed: %s", llmResp == nil, err)
			return nil, err
		}

		str, _ := sonic.MarshalString(llmResp)
		logs.CtxDebug(ctx, "[agent:%s:%d] llmResp: %+v", ag.id, iter, str)
		if len(llmResp.ToolCalls) > 0 {
			msgs = append(msgs, llmResp)
			for _, call := range llmResp.ToolCalls {
				logs.CtxDebug(ctx, "[agent:%s:%d] call: %+v", ag.id, iter, call)
				res, callErr := ag.tools.ExecuteToolCall(ctx, &call)
				resMsg := &schema.Message{
					Role:       schema.Tool,
					ToolName:   call.Function.Name,
					ToolCallID: call.ID,
				}
				if callErr != nil {
					logs.CtxWarn(ctx, "agent tool call failed: %s", callErr)
					resMsg.Content = "ERROR: " + callErr.Error()
				} else {
					jsonStr, marshalErr := sonic.MarshalString(res)
					if marshalErr != nil || jsonStr == "" {
						resMsg.Content = "{}"
					} else {
						resMsg.Content = jsonStr
					}
				}
				msgs = append(msgs, resMsg)
			}
			continue
		}

		finalResp = llmResp
		break
	}

	if finalResp == nil {
		// Iteration limit reached — ask LLM to summarize progress without tools.
		logs.CtxWarn(ctx, "[agent:%s] iteration limit (%d) reached, requesting summary", ag.id, maxIterations)
		finalResp = ag.runSummary(ctx, p, ms, msgs)
	}

	sess.Append(finalResp)
	return &channel.Response{
		ID:       msg.ID,
		Content:  finalResp.Content,
		Model:    ms.ModelName,
		Provider: ms.ProviderID,
	}, nil
}

// runSummary makes one final LLM call without tools to summarize what has
// been accomplished and what remains when the iteration limit is exceeded.
func (ag *Agent) runSummary(ctx context.Context,
	p provider.Provider,
	ms *provider.ModelSpec,
	msgs []*schema.Message,
) *schema.Message {
	msgs = append(msgs, &schema.Message{
		Role:    schema.User,
		Content: "You have reached the maximum iteration limit. Please summarize what you have accomplished so far and what still remains to be done.",
	})

	resp, err := p.Generate(ctx, ms.ModelName, msgs)
	if err != nil || resp == nil {
		logs.CtxWarn(ctx, "[agent:%s] summary generation failed: %v", ag.id, err)
		return &schema.Message{
			Role:    schema.Assistant,
			Content: "Task reached the maximum iteration limit. Partial work may have been applied. Please review and continue if needed.",
		}
	}
	return resp
}
