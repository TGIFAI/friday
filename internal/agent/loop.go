package agent

import (
	"context"
	"fmt"

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

func (ag *Agent) runLoop(ctx context.Context, p provider.Provider, modelSpec *provider.ModelSpec, sess *session.Session, msg *channel.Message, cfg config.AgentRuntimeConfig) (*channel.Response, error) {
	// Inject session into context so CLI providers can access metadata.
	ctx = session.WithContext(ctx, sess)
	promptMsgs := ag.buildMessages(sess, msg)

	maxIterations := defaultMaxIterations
	if cfg.MaxIterations > 0 {
		maxIterations = cfg.MaxIterations
	}

	logs.CtxDebug(ctx, "[agent:%s] sending to provider %s:%s, messages count: %d, max_iterations: %d",
		ag.id, modelSpec.ProviderID, modelSpec.ModelName, len(promptMsgs), maxIterations)

	var finalMsg *schema.Message
	msgs := make([]*schema.Message, 0, 4)

	opts := []model.Option{
		model.WithTools(ag.tools.ListToolInfos()),
		model.WithToolChoice(schema.ToolChoiceAllowed),
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

		str, _ := sonic.MarshalString(llmResp)
		logs.CtxDebug(ctx, "[agent:%s:%d] llmResp: %+v", ag.id, iter, str)
		if len(llmResp.ToolCalls) > 0 {
			// TODO send msg to user when calling tools
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
		finalMsg = ag.runSummary(ctx, p, modelSpec, append(promptMsgs, msgs...))
	}

	// Commit msgs tool-call turns and the final response to session.
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

// runSummary makes one final LLM call without tools to summarize what has
// been accomplished and what remains when the iteration limit is exceeded.
func (ag *Agent) runSummary(ctx context.Context,
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
